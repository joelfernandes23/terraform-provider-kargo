package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

var (
	_ datasource.DataSource              = &StageDataSource{}
	_ datasource.DataSourceWithConfigure = &StageDataSource{}
)

type StageDataSource struct {
	client client.KargoClient
}

type StageDataSourceModel struct {
	Project           types.String                  `tfsdk:"project"`
	Name              types.String                  `tfsdk:"name"`
	ID                types.String                  `tfsdk:"id"`
	Shard             types.String                  `tfsdk:"shard"`
	RequestedFreight  []StageRequestedFreightModel  `tfsdk:"requested_freight"`
	PromotionTemplate *StagePromotionTemplateModel  `tfsdk:"promotion_template"`
	Status            *StageDataSourceStatusModel   `tfsdk:"status"`
	Freight           []StageDataSourceFreightModel `tfsdk:"freight"`
}

type StageDataSourceStatusModel struct {
	Health               *StageDataSourceHealthModel        `tfsdk:"health"`
	Conditions           []StageDataSourceConditionModel    `tfsdk:"conditions"`
	FreightSummary       types.String                       `tfsdk:"freight_summary"`
	ObservedGeneration   types.Int64                        `tfsdk:"observed_generation"`
	AutoPromotionEnabled types.Bool                         `tfsdk:"auto_promotion_enabled"`
	LastHandledRefresh   types.String                       `tfsdk:"last_handled_refresh"`
	LastPromotion        *StageDataSourceLastPromotionModel `tfsdk:"last_promotion"`
}

type StageDataSourceHealthModel struct {
	Status types.String `tfsdk:"status"`
	Issues types.List   `tfsdk:"issues"`
}

type StageDataSourceConditionModel struct {
	Type               types.String `tfsdk:"type"`
	Status             types.String `tfsdk:"status"`
	Reason             types.String `tfsdk:"reason"`
	Message            types.String `tfsdk:"message"`
	LastTransitionTime types.String `tfsdk:"last_transition_time"`
}

type StageDataSourceLastPromotionModel struct {
	Name       types.String `tfsdk:"name"`
	Phase      types.String `tfsdk:"phase"`
	FinishedAt types.String `tfsdk:"finished_at"`
}

type StageDataSourceFreightModel struct {
	Name       types.String                              `tfsdk:"name"`
	OriginKind types.String                              `tfsdk:"origin_kind"`
	OriginName types.String                              `tfsdk:"origin_name"`
	Commits    []WarehouseDataSourceFreightCommitModel   `tfsdk:"commits"`
	Images     []WarehouseDataSourceFreightImageModel    `tfsdk:"images"`
	Charts     []WarehouseDataSourceFreightChartModel    `tfsdk:"charts"`
	Artifacts  []WarehouseDataSourceFreightArtifactModel `tfsdk:"artifacts"`
}

func NewStageDataSource() datasource.DataSource {
	return &StageDataSource{}
}

func (d *StageDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_stage"
}

func (d *StageDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Kargo stage by project and name.",
		Attributes: map[string]schema.Attribute{
			"project": schema.StringAttribute{
				Required:    true,
				Description: "The Kargo project that contains the stage.",
				Validators:  rfc1123NameValidators(),
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the Kargo stage to look up.",
				Validators:  rfc1123NameValidators(),
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique identifier of the stage in project/name format.",
			},
			"shard": schema.StringAttribute{
				Computed:    true,
				Description: "Shard that the stage belongs to. Null when the stage is not sharded.",
			},
			"requested_freight":  stageDataSourceRequestedFreightAttribute(),
			"promotion_template": stageDataSourcePromotionTemplateAttribute(),
			"status":             stageDataSourceStatusAttribute(),
			"freight":            stageDataSourceFreightAttribute(),
		},
	}
}

func (d *StageDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	c, ok := req.ProviderData.(client.KargoClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected client.KargoClient, got: %T", req.ProviderData),
		)
		return
	}
	d.client = c
}

func (d *StageDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data StageDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	project := data.Project.ValueString()
	name := data.Name.ValueString()
	stage, err := d.client.GetStage(ctx, project, name)
	if err != nil {
		if client.IsNotFound(err) {
			resp.Diagnostics.AddError("Stage not found", fmt.Sprintf("Stage %q does not exist or is being deleted.", stageID(project, name)))
			return
		}
		resp.Diagnostics.AddError("Failed to read stage", fmt.Sprintf("Unable to read stage %q: %s", stageID(project, name), err.Error()))
		return
	}
	if stage == nil {
		// The stage is terminating (deletionTimestamp set).
		resp.Diagnostics.AddError("Stage not found", fmt.Sprintf("Stage %q does not exist or is being deleted.", stageID(project, name)))
		return
	}

	newData := flattenStageDataSource(ctx, project, stage)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newData)...)
}

func stageDataSourceRequestedFreightAttribute() schema.Attribute {
	return schema.ListNestedAttribute{
		Computed:    true,
		Description: "Freight requested by this stage.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"origin": schema.SingleNestedAttribute{
					Computed:    true,
					Description: "The warehouse this freight originates from.",
					Attributes: map[string]schema.Attribute{
						"kind": schema.StringAttribute{Computed: true, Description: "Origin kind."},
						"name": schema.StringAttribute{Computed: true, Description: "Name of the origin warehouse."},
					},
				},
				"sources": schema.SingleNestedAttribute{
					Computed:    true,
					Description: "Where the stage may obtain the requested freight.",
					Attributes: map[string]schema.Attribute{
						"direct": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether freight is requested directly from the origin warehouse. False when absent on the wire; Kargo omits false values.",
						},
						"stages": schema.ListAttribute{
							ElementType: types.StringType,
							Computed:    true,
							Description: "Names of upstream stages the freight may be obtained from.",
						},
					},
				},
			},
		},
	}
}

func stageDataSourcePromotionTemplateAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Computed:    true,
		Description: "Template for promotions into this stage. Null for control-flow stages.",
		Attributes: map[string]schema.Attribute{
			"step": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Ordered promotion steps executed during a promotion.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"uses": schema.StringAttribute{Computed: true, Description: "Name of the promotion step runner (for example git-clone)."},
						"as":   schema.StringAttribute{Computed: true, Description: "Alias for referencing this step's output."},
						"config": schema.StringAttribute{
							Computed:    true,
							CustomType:  jsontypes.NormalizedType{},
							Description: "Step configuration as a JSON object string.",
						},
					},
				},
			},
		},
	}
}

func stageDataSourceStatusAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Computed:    true,
		Description: "The current status of the stage.",
		Attributes: map[string]schema.Attribute{
			"health": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The health of the stage. Null until the stage has promotion activity.",
				Attributes: map[string]schema.Attribute{
					"status": schema.StringAttribute{Computed: true, Description: "Healthy | Unhealthy | Progressing | Unknown | NotApplicable."},
					"issues": schema.ListAttribute{
						ElementType: types.StringType,
						Computed:    true,
						Description: "Issues explaining the health status.",
					},
				},
			},
			"conditions": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The conditions of the stage, sorted by type.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type":                 schema.StringAttribute{Computed: true, Description: "The type of the condition."},
						"status":               schema.StringAttribute{Computed: true, Description: "The status of the condition."},
						"reason":               schema.StringAttribute{Computed: true, Description: "The reason for the condition."},
						"message":              schema.StringAttribute{Computed: true, Description: "A human-readable message for the condition."},
						"last_transition_time": schema.StringAttribute{Computed: true, Description: "The last time the condition transitioned."},
					},
				},
			},
			"freight_summary": schema.StringAttribute{
				Computed:    true,
				Description: "Human-readable freight fulfillment summary (for example 1/1 Fulfilled).",
			},
			"observed_generation": schema.Int64Attribute{
				Computed:    true,
				Description: "The Stage generation most recently observed by the controller. Null when the controller has not reported it.",
			},
			"auto_promotion_enabled": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether auto-promotion is enabled. False when absent on the wire.",
			},
			"last_handled_refresh": schema.StringAttribute{
				Computed:    true,
				Description: "The most recent refresh annotation value handled by the Stage controller.",
			},
			"last_promotion": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The last completed promotion. Null when the stage has never been promoted.",
				Attributes: map[string]schema.Attribute{
					"name":        schema.StringAttribute{Computed: true, Description: "The promotion name."},
					"phase":       schema.StringAttribute{Computed: true, Description: "The promotion phase, such as Succeeded."},
					"finished_at": schema.StringAttribute{Computed: true, Description: "When the promotion finished."},
				},
			},
		},
	}
}

func stageDataSourceFreightAttribute() schema.Attribute {
	return schema.ListNestedAttribute{
		Computed:    true,
		Description: "The current freight deployed to the stage (head of status freightHistory), one entry per origin, sorted by origin. Historical freight is intentionally not exposed.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"name":        schema.StringAttribute{Computed: true, Description: "The Freight name."},
				"origin_kind": schema.StringAttribute{Computed: true, Description: "The kind of resource that produced the Freight."},
				"origin_name": schema.StringAttribute{Computed: true, Description: "The name of the resource that produced the Freight."},
				"commits":     freightCommitAttribute(),
				"images":      freightImageAttribute(),
				"charts":      freightChartAttribute(),
				"artifacts":   freightArtifactAttribute(),
			},
		},
	}
}

func flattenStageDataSource(ctx context.Context, project string, stage *client.Stage) StageDataSourceModel {
	resolvedProject := stage.Metadata.Namespace
	if resolvedProject == "" {
		resolvedProject = project
	}

	data := StageDataSourceModel{
		Project:          types.StringValue(resolvedProject),
		Name:             types.StringValue(stage.Metadata.Name),
		ID:               types.StringValue(stageID(resolvedProject, stage.Metadata.Name)),
		Shard:            computedStringValue(stage.Spec.Shard),
		RequestedFreight: make([]StageRequestedFreightModel, 0, len(stage.Spec.RequestedFreight)),
		Status:           flattenStageDataSourceStatus(ctx, stage.Status),
		Freight:          flattenStageDataSourceFreight(stage.Status.FreightHistory),
	}

	for _, freight := range stage.Spec.RequestedFreight {
		stages := types.ListNull(types.StringType)
		if len(freight.Sources.Stages) > 0 {
			// []string into a string list cannot produce diagnostics.
			list, diags := types.ListValueFrom(ctx, types.StringType, freight.Sources.Stages)
			if !diags.HasError() {
				stages = list
			}
		}
		data.RequestedFreight = append(data.RequestedFreight, StageRequestedFreightModel{
			Origin: &StageFreightOriginModel{
				Kind: computedStringValue(freight.Origin.Kind),
				Name: types.StringValue(freight.Origin.Name),
			},
			Sources: &StageFreightSourcesModel{
				Direct: types.BoolValue(freight.Sources.Direct),
				Stages: stages,
			},
		})
	}

	if stage.Spec.PromotionTemplate != nil {
		serverSteps := stage.Spec.PromotionTemplate.Spec.Steps
		steps := make([]StageStepModel, 0, len(serverSteps))
		for _, step := range serverSteps {
			steps = append(steps, StageStepModel{
				Uses:   types.StringValue(step.Uses),
				As:     computedStringValue(step.As),
				Config: flattenStepConfig(step.Config),
			})
		}
		data.PromotionTemplate = &StagePromotionTemplateModel{Step: steps}
	}

	return data
}

func flattenStageDataSourceStatus(ctx context.Context, status client.StageStatus) *StageDataSourceStatusModel {
	conditions := append([]client.StageCondition(nil), status.Conditions...)
	sort.SliceStable(conditions, func(i, j int) bool {
		return conditions[i].Type < conditions[j].Type
	})

	conditionModels := make([]StageDataSourceConditionModel, 0, len(conditions))
	for _, condition := range conditions {
		conditionModels = append(conditionModels, StageDataSourceConditionModel{
			Type:               types.StringValue(condition.Type),
			Status:             types.StringValue(condition.Status),
			Reason:             computedStringValue(condition.Reason),
			Message:            computedStringValue(condition.Message),
			LastTransitionTime: computedStringValue(condition.LastTransitionTime.String()),
		})
	}

	// The controller leaves top-level observedGeneration unset even on
	// reconciled stages (omitempty drops 0), so 0 flattens to null.
	observedGeneration := types.Int64Null()
	if status.ObservedGeneration != 0 {
		observedGeneration = types.Int64Value(status.ObservedGeneration)
	}

	var lastPromotion *StageDataSourceLastPromotionModel
	if status.LastPromotion != nil {
		phase := types.StringNull()
		if status.LastPromotion.Status != nil {
			phase = computedStringValue(status.LastPromotion.Status.Phase)
		}
		lastPromotion = &StageDataSourceLastPromotionModel{
			Name:       types.StringValue(status.LastPromotion.Name),
			Phase:      phase,
			FinishedAt: computedStringValue(status.LastPromotion.FinishedAt),
		}
	}

	return &StageDataSourceStatusModel{
		Health:         flattenStageDataSourceHealth(ctx, status.Health),
		Conditions:     conditionModels,
		FreightSummary: computedStringValue(status.FreightSummary),
		// false is indistinguishable from absent on the wire (omitempty), so
		// always render a concrete bool.
		AutoPromotionEnabled: types.BoolValue(status.AutoPromotionEnabled),
		ObservedGeneration:   observedGeneration,
		LastHandledRefresh:   computedStringValue(status.LastHandledRefresh),
		LastPromotion:        lastPromotion,
	}
}

func flattenStageDataSourceHealth(ctx context.Context, health *client.StageHealth) *StageDataSourceHealthModel {
	if health == nil {
		// Fresh stage without promotion activity: whole health object is null.
		return nil
	}

	issues := types.ListNull(types.StringType)
	if len(health.Issues) > 0 {
		// []string into a string list cannot produce diagnostics.
		list, diags := types.ListValueFrom(ctx, types.StringType, health.Issues)
		if !diags.HasError() {
			issues = list
		}
	}

	return &StageDataSourceHealthModel{
		Status: computedStringValue(health.Status),
		Issues: issues,
	}
}

func flattenStageDataSourceFreight(history []client.StageFreightCollection) []StageDataSourceFreightModel {
	if len(history) == 0 {
		return []StageDataSourceFreightModel{}
	}

	// The head of freightHistory is the collection currently deployed to the
	// stage; older collections are intentionally not exposed.
	current := history[0]
	result := make([]StageDataSourceFreightModel, 0, len(current.Items))
	for _, key := range sortedMapKeys(current.Items) {
		ref := current.Items[key]
		model := StageDataSourceFreightModel{
			Name:       computedStringValue(ref.Name),
			OriginKind: computedStringValue(ref.Origin.Kind),
			OriginName: computedStringValue(ref.Origin.Name),
			Commits:    make([]WarehouseDataSourceFreightCommitModel, 0, len(ref.Commits)),
			Images:     make([]WarehouseDataSourceFreightImageModel, 0, len(ref.Images)),
			Charts:     make([]WarehouseDataSourceFreightChartModel, 0, len(ref.Charts)),
			Artifacts:  make([]WarehouseDataSourceFreightArtifactModel, 0, len(ref.Artifacts)),
		}
		for _, commit := range ref.Commits {
			model.Commits = append(model.Commits, WarehouseDataSourceFreightCommitModel{
				RepoURL:   types.StringValue(commit.RepoURL),
				ID:        types.StringValue(commit.ID),
				Branch:    computedStringValue(commit.Branch),
				Tag:       computedStringValue(commit.Tag),
				Message:   computedStringValue(commit.Message),
				Author:    computedStringValue(commit.Author),
				Committer: computedStringValue(commit.Committer),
			})
		}
		for _, image := range ref.Images {
			model.Images = append(model.Images, WarehouseDataSourceFreightImageModel{
				RepoURL: types.StringValue(image.RepoURL),
				Tag:     computedStringValue(image.Tag),
				Digest:  computedStringValue(image.Digest),
			})
		}
		for _, chart := range ref.Charts {
			model.Charts = append(model.Charts, WarehouseDataSourceFreightChartModel{
				RepoURL: types.StringValue(chart.RepoURL),
				Name:    computedStringValue(chart.Name),
				Version: computedStringValue(chart.Version),
			})
		}
		for _, artifact := range ref.Artifacts {
			model.Artifacts = append(model.Artifacts, WarehouseDataSourceFreightArtifactModel{
				ArtifactType:     types.StringValue(artifact.ArtifactType),
				SubscriptionName: types.StringValue(artifact.SubscriptionName),
				Version:          types.StringValue(artifact.Version),
			})
		}
		result = append(result, model)
	}
	return result
}
