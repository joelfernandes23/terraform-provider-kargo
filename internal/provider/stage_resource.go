package provider

import (
	"context"
	"fmt"

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

func (r *StageResource) Create(_ context.Context, _ resource.CreateRequest, resp *resource.CreateResponse) {
	resp.Diagnostics.AddError("Not implemented", "kargo_stage Create is not implemented yet.")
}

func (r *StageResource) Read(_ context.Context, _ resource.ReadRequest, resp *resource.ReadResponse) {
	resp.Diagnostics.AddError("Not implemented", "kargo_stage Read is not implemented yet.")
}

func (r *StageResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Not implemented", "kargo_stage Update is not implemented yet.")
}

func (r *StageResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddError("Not implemented", "kargo_stage Delete is not implemented yet.")
}

func (r *StageResource) ImportState(_ context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.AddError("Not implemented", "kargo_stage import is not implemented yet.")
}

func parseStageID(id string) (project, name string, err error) {
	return id, "", fmt.Errorf("parseStageID not implemented")
}

func expandStageSpec(_ context.Context, data *StageResourceModel) (client.StageSpec, error) {
	return client.StageSpec{Shard: valueString(data.Shard)}, fmt.Errorf("expandStageSpec not implemented for stage %q", valueString(data.Name))
}

func flattenStage(_ context.Context, project string, _ *client.Stage, _ *StageResourceModel) StageResourceModel {
	return StageResourceModel{Project: types.StringValue(project)}
}
