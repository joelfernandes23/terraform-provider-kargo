package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

var (
	_ resource.Resource                = &StageResource{}
	_ resource.ResourceWithImportState = &StageResource{}
)

type StageResource struct {
	client client.KargoClient
}

type StageResourceModel struct {
	Project           types.String                 `tfsdk:"project"`
	Name              types.String                 `tfsdk:"name"`
	ID                types.String                 `tfsdk:"id"`
	Shard             types.String                 `tfsdk:"shard"`
	RequestedFreight  []StageRequestedFreightModel `tfsdk:"requested_freight"`
	PromotionTemplate *StagePromotionTemplateModel `tfsdk:"promotion_template"`
}

type StageRequestedFreightModel struct {
	Origin  *StageFreightOriginModel  `tfsdk:"origin"`
	Sources *StageFreightSourcesModel `tfsdk:"sources"`
}

type StageFreightOriginModel struct {
	Kind types.String `tfsdk:"kind"`
	Name types.String `tfsdk:"name"`
}

type StageFreightSourcesModel struct {
	Direct types.Bool `tfsdk:"direct"`
	Stages types.List `tfsdk:"stages"`
}

type StagePromotionTemplateModel struct {
	Step []StageStepModel `tfsdk:"step"`
}

type StageStepModel struct {
	Uses   types.String         `tfsdk:"uses"`
	As     types.String         `tfsdk:"as"`
	Config jsontypes.Normalized `tfsdk:"config"`
}

func NewStageResource() resource.Resource {
	return &StageResource{}
}

func (r *StageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stage"
}

func (r *StageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Kargo stage. Stages request freight from warehouses or upstream stages and promote it via promotion template steps.",
		Attributes: map[string]schema.Attribute{
			"project": schema.StringAttribute{
				Required:    true,
				Description: "The Kargo project that contains the stage.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: rfc1123NameValidators(),
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the Kargo stage.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: rfc1123NameValidators(),
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique identifier of the stage in project/name format.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"shard": schema.StringAttribute{
				Optional:    true,
				Description: "Shard that the stage belongs to. Kargo syncs this to the kargo.akuity.io/shard label.",
			},
		},
		Blocks: map[string]schema.Block{
			"requested_freight": schema.ListNestedBlock{
				Description: "Freight requested by this stage. At least one block is required.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedBlockObject{
					Blocks: map[string]schema.Block{
						"origin": schema.SingleNestedBlock{
							Description: "The warehouse this freight originates from.",
							Attributes: map[string]schema.Attribute{
								"kind": schema.StringAttribute{
									Optional:    true,
									Description: "Origin kind. Only \"Warehouse\" is supported; defaults to \"Warehouse\".",
									Validators: []validator.String{
										stringvalidator.OneOf("Warehouse"),
									},
								},
								"name": schema.StringAttribute{
									Optional:    true,
									Description: "Name of the origin warehouse. Required.",
								},
							},
						},
						"sources": schema.SingleNestedBlock{
							Description: "Where the stage may obtain the requested freight.",
							Attributes: map[string]schema.Attribute{
								"direct": schema.BoolAttribute{
									Optional:    true,
									Description: "Request freight directly from the origin warehouse.",
								},
								"stages": schema.ListAttribute{
									ElementType: types.StringType,
									Optional:    true,
									Description: "Names of upstream stages to request freight from.",
								},
							},
						},
					},
				},
			},
			"promotion_template": schema.SingleNestedBlock{
				Description: "Template for promotions into this stage. Omit for control-flow stages.",
				Blocks: map[string]schema.Block{
					"step": schema.ListNestedBlock{
						Description: "Ordered promotion steps executed during a promotion. At least one step is required.",
						Validators: []validator.List{
							listvalidator.SizeAtLeast(1),
						},
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"uses": schema.StringAttribute{
									Optional:    true,
									Description: "Name of the promotion step runner (for example git-clone). Required.",
								},
								"as": schema.StringAttribute{
									Optional:    true,
									Description: "Alias for referencing this step's output.",
								},
								"config": schema.StringAttribute{
									Optional:    true,
									CustomType:  jsontypes.NormalizedType{},
									Description: "Step configuration as a JSON object string (use jsonencode()). Compared with JSON semantic equality. Do not inline secret values; reference Kargo secrets by name/expression instead.",
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *StageResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(client.KargoClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected client.KargoClient, got: %T", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *StageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data StageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	spec, err := expandStageSpec(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Invalid stage configuration", err.Error())
		return
	}

	stage, err := r.client.CreateStage(ctx, data.Project.ValueString(), data.Name.ValueString(), spec)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create stage", err.Error())
		return
	}
	if stage == nil {
		resp.Diagnostics.AddError("Failed to create stage", "Kargo returned no stage after creation.")
		return
	}

	newData := flattenStage(ctx, data.Project.ValueString(), stage, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newData)...)
}

func (r *StageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data StageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	stage, err := r.client.GetStage(ctx, data.Project.ValueString(), data.Name.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read stage", err.Error())
		return
	}

	if stage == nil {
		// The stage is terminating (deletionTimestamp set) — treat as deleted.
		resp.State.RemoveResource(ctx)
		return
	}

	newData := flattenStage(ctx, data.Project.ValueString(), stage, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newData)...)
}

func (r *StageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data StageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	spec, err := expandStageSpec(ctx, &data)
	if err != nil {
		resp.Diagnostics.AddError("Invalid stage configuration", err.Error())
		return
	}

	stage, err := r.client.UpdateStage(ctx, data.Project.ValueString(), data.Name.ValueString(), spec)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update stage", err.Error())
		return
	}
	if stage == nil {
		resp.Diagnostics.AddError("Failed to update stage", "Kargo returned no stage after update.")
		return
	}

	newData := flattenStage(ctx, data.Project.ValueString(), stage, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newData)...)
}

func (r *StageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data StageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteStage(ctx, data.Project.ValueString(), data.Name.ValueString()); err != nil {
		// Kargo's DeleteStage propagates the k8s 404; ignore stages already gone.
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete stage", err.Error())
	}
}

func (r *StageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	project, name, err := parseStageID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid stage import ID", err.Error())
		return
	}

	stage, err := r.client.GetStage(ctx, project, name)
	if err != nil {
		if client.IsNotFound(err) {
			resp.Diagnostics.AddError("Stage not found", fmt.Sprintf("Stage %q does not exist.", req.ID))
			return
		}
		resp.Diagnostics.AddError("Failed to read stage", err.Error())
		return
	}
	if stage == nil {
		resp.Diagnostics.AddError("Stage not found", fmt.Sprintf("Stage %q does not exist.", req.ID))
		return
	}

	data := flattenStage(ctx, project, stage, nil)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func stageID(project, name string) string {
	return project + "/" + name
}

func parseStageID(id string) (project, name string, err error) {
	project, name, ok := strings.Cut(id, "/")
	if !ok || project == "" || name == "" || strings.Contains(name, "/") {
		return "", "", fmt.Errorf("expected import ID in project/name format")
	}
	return project, name, nil
}

func expandStageSpec(ctx context.Context, data *StageResourceModel) (client.StageSpec, error) {
	if len(data.RequestedFreight) == 0 {
		return client.StageSpec{}, fmt.Errorf("stage must have at least one requested_freight")
	}

	spec := client.StageSpec{
		Shard:            valueString(data.Shard),
		RequestedFreight: make([]client.FreightRequest, 0, len(data.RequestedFreight)),
	}

	seenOrigins := map[string]int{}
	for i, freight := range data.RequestedFreight {
		if freight.Origin == nil || valueString(freight.Origin.Name) == "" {
			return client.StageSpec{}, fmt.Errorf("requested_freight %d origin.name must be set", i)
		}
		name := freight.Origin.Name.ValueString()

		kind := valueString(freight.Origin.Kind)
		if kind == "" {
			kind = "Warehouse"
		}

		originKey := kind + "/" + name
		if firstIdx, seen := seenOrigins[originKey]; seen {
			return client.StageSpec{}, fmt.Errorf("requested_freight %d duplicates origin of requested_freight %d", i, firstIdx)
		}
		seenOrigins[originKey] = i

		var direct bool
		var stages []string
		if freight.Sources != nil {
			direct = freight.Sources.Direct.ValueBool()
			if !freight.Sources.Stages.IsNull() && !freight.Sources.Stages.IsUnknown() {
				if diags := freight.Sources.Stages.ElementsAs(ctx, &stages, false); diags.HasError() {
					return client.StageSpec{}, fmt.Errorf("requested_freight %d has invalid sources.stages: %s", i, diags)
				}
			}
		}
		if !direct && len(stages) == 0 {
			return client.StageSpec{}, fmt.Errorf("requested_freight %d sources must set direct = true or at least one stage", i)
		}

		spec.RequestedFreight = append(spec.RequestedFreight, client.FreightRequest{
			Origin:  client.FreightOrigin{Kind: kind, Name: name},
			Sources: client.FreightSources{Direct: direct, Stages: stages},
		})
	}

	if data.PromotionTemplate != nil {
		if len(data.PromotionTemplate.Step) == 0 {
			// Defense in depth: the schema validator already enforces SizeAtLeast(1).
			return client.StageSpec{}, fmt.Errorf("promotion_template must have at least one step")
		}

		steps := make([]client.PromotionStep, 0, len(data.PromotionTemplate.Step))
		for j, step := range data.PromotionTemplate.Step {
			if valueString(step.Uses) == "" {
				return client.StageSpec{}, fmt.Errorf("promotion_template step %d uses must be set", j)
			}
			expanded := client.PromotionStep{
				Uses: step.Uses.ValueString(),
				As:   valueString(step.As),
			}
			if !step.Config.IsNull() && !step.Config.IsUnknown() {
				expanded.Config = json.RawMessage(step.Config.ValueString())
			}
			steps = append(steps, expanded)
		}
		spec.PromotionTemplate = &client.PromotionTemplate{
			Spec: client.PromotionTemplateSpec{Steps: steps},
		}
	}

	return spec, nil
}

func flattenStage(ctx context.Context, project string, stage *client.Stage, prior *StageResourceModel) StageResourceModel {
	resolvedProject := stage.Metadata.Namespace
	if resolvedProject == "" {
		resolvedProject = project
	}

	priorShard := types.StringValue("__import__")
	if prior != nil {
		priorShard = prior.Shard
	}

	data := StageResourceModel{
		Project:          types.StringValue(resolvedProject),
		Name:             types.StringValue(stage.Metadata.Name),
		ID:               types.StringValue(stageID(resolvedProject, stage.Metadata.Name)),
		Shard:            optionalStringValue(stage.Spec.Shard, priorShard),
		RequestedFreight: make([]StageRequestedFreightModel, 0, len(stage.Spec.RequestedFreight)),
	}

	for i, freight := range stage.Spec.RequestedFreight {
		var priorFreight *StageRequestedFreightModel
		if prior != nil && i < len(prior.RequestedFreight) {
			priorFreight = &prior.RequestedFreight[i]
		}

		priorKind := types.StringValue("__import__")
		if priorFreight != nil {
			priorKind = types.StringNull()
			if priorFreight.Origin != nil {
				priorKind = priorFreight.Origin.Kind
			}
		}

		var priorSources *StageFreightSourcesModel
		if priorFreight != nil {
			priorSources = priorFreight.Sources
		}

		sources := &StageFreightSourcesModel{}
		if priorSources != nil {
			// Echo the planned sources unchanged: expand already validated them and
			// the server round-trips direct/stages as sent.
			sources.Direct = priorSources.Direct
			sources.Stages = priorSources.Stages
		} else {
			sources.Direct = types.BoolNull()
			if freight.Sources.Direct {
				sources.Direct = types.BoolValue(true)
			}
			sources.Stages = types.ListNull(types.StringType)
			if len(freight.Sources.Stages) > 0 {
				// []string into a string list cannot produce diagnostics.
				list, diags := types.ListValueFrom(ctx, types.StringType, freight.Sources.Stages)
				if !diags.HasError() {
					sources.Stages = list
				}
			}
		}

		data.RequestedFreight = append(data.RequestedFreight, StageRequestedFreightModel{
			Origin: &StageFreightOriginModel{
				Kind: optionalStringValue(freight.Origin.Kind, priorKind),
				Name: types.StringValue(freight.Origin.Name),
			},
			Sources: sources,
		})
	}

	if stage.Spec.PromotionTemplate != nil {
		serverSteps := stage.Spec.PromotionTemplate.Spec.Steps
		steps := make([]StageStepModel, 0, len(serverSteps))
		for j, step := range serverSteps {
			var priorStep *StageStepModel
			if prior != nil && prior.PromotionTemplate != nil && j < len(prior.PromotionTemplate.Step) {
				priorStep = &prior.PromotionTemplate.Step[j]
			}

			priorAs := types.StringValue("__import__")
			if priorStep != nil {
				priorAs = priorStep.As
			}

			steps = append(steps, StageStepModel{
				Uses:   types.StringValue(step.Uses),
				As:     optionalStringValue(step.As, priorAs),
				Config: flattenStepConfig(step.Config, priorStep),
			})
		}
		data.PromotionTemplate = &StagePromotionTemplateModel{Step: steps}
	}

	return data
}

// flattenStepConfig echoes the planned config bytes back to state so apply
// results match the plan exactly; on import (prior == nil) it takes the server
// value, and jsontypes.Normalized semantic equality keeps refreshes stable even
// though the server re-serializes config with sorted keys.
func flattenStepConfig(server json.RawMessage, prior *StageStepModel) jsontypes.Normalized {
	if prior != nil {
		return prior.Config
	}
	if len(server) == 0 {
		return jsontypes.NewNormalizedNull()
	}
	return jsontypes.NewNormalizedValue(string(server))
}
