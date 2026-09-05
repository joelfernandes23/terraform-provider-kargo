package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

var (
	_ datasource.DataSource              = &WarehouseDataSource{}
	_ datasource.DataSourceWithConfigure = &WarehouseDataSource{}
)

type WarehouseDataSource struct {
	client client.KargoClient
}

type WarehouseDataSourceModel struct {
	Project      types.String                      `tfsdk:"project"`
	Name         types.String                      `tfsdk:"name"`
	ID           types.String                      `tfsdk:"id"`
	Subscription []WarehouseSubscriptionModel      `tfsdk:"subscription"`
	Status       *WarehouseDataSourceStatusModel   `tfsdk:"status"`
	Freight      []WarehouseDataSourceFreightModel `tfsdk:"freight"`
}

type WarehouseDataSourceStatusModel struct {
	Conditions         []WarehouseDataSourceConditionModel `tfsdk:"conditions"`
	LastHandledRefresh types.String                        `tfsdk:"last_handled_refresh"`
	ObservedGeneration types.Int64                         `tfsdk:"observed_generation"`
	LastFreightID      types.String                        `tfsdk:"last_freight_id"`
}

type WarehouseDataSourceConditionModel struct {
	Type               types.String `tfsdk:"type"`
	Status             types.String `tfsdk:"status"`
	Reason             types.String `tfsdk:"reason"`
	Message            types.String `tfsdk:"message"`
	LastTransitionTime types.String `tfsdk:"last_transition_time"`
}

type WarehouseDataSourceFreightModel struct {
	Name        types.String                              `tfsdk:"name"`
	Alias       types.String                              `tfsdk:"alias"`
	OriginKind  types.String                              `tfsdk:"origin_kind"`
	OriginName  types.String                              `tfsdk:"origin_name"`
	Commits     []WarehouseDataSourceFreightCommitModel   `tfsdk:"commits"`
	Images      []WarehouseDataSourceFreightImageModel    `tfsdk:"images"`
	Charts      []WarehouseDataSourceFreightChartModel    `tfsdk:"charts"`
	Artifacts   []WarehouseDataSourceFreightArtifactModel `tfsdk:"artifacts"`
	CurrentlyIn []WarehouseDataSourceCurrentStageModel    `tfsdk:"currently_in"`
	VerifiedIn  []WarehouseDataSourceVerifiedStageModel   `tfsdk:"verified_in"`
	ApprovedFor []WarehouseDataSourceApprovedStageModel   `tfsdk:"approved_for"`
}

type WarehouseDataSourceFreightCommitModel struct {
	RepoURL   types.String `tfsdk:"repo_url"`
	ID        types.String `tfsdk:"id"`
	Branch    types.String `tfsdk:"branch"`
	Tag       types.String `tfsdk:"tag"`
	Message   types.String `tfsdk:"message"`
	Author    types.String `tfsdk:"author"`
	Committer types.String `tfsdk:"committer"`
}

type WarehouseDataSourceFreightImageModel struct {
	RepoURL types.String `tfsdk:"repo_url"`
	Tag     types.String `tfsdk:"tag"`
	Digest  types.String `tfsdk:"digest"`
}

type WarehouseDataSourceFreightChartModel struct {
	RepoURL types.String `tfsdk:"repo_url"`
	Name    types.String `tfsdk:"name"`
	Version types.String `tfsdk:"version"`
}

type WarehouseDataSourceFreightArtifactModel struct {
	ArtifactType     types.String `tfsdk:"artifact_type"`
	SubscriptionName types.String `tfsdk:"subscription_name"`
	Version          types.String `tfsdk:"version"`
}

type WarehouseDataSourceCurrentStageModel struct {
	Stage types.String `tfsdk:"stage"`
	Since types.String `tfsdk:"since"`
}

type WarehouseDataSourceVerifiedStageModel struct {
	Stage       types.String `tfsdk:"stage"`
	VerifiedAt  types.String `tfsdk:"verified_at"`
	LongestSoak types.String `tfsdk:"longest_soak"`
}

type WarehouseDataSourceApprovedStageModel struct {
	Stage      types.String `tfsdk:"stage"`
	ApprovedAt types.String `tfsdk:"approved_at"`
}

func NewWarehouseDataSource() datasource.DataSource {
	return &WarehouseDataSource{}
}

func (d *WarehouseDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_warehouse"
}

func (d *WarehouseDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Kargo warehouse by project and name.",
		Attributes: map[string]schema.Attribute{
			"project": schema.StringAttribute{
				Required:    true,
				Description: "The Kargo project that contains the warehouse.",
				Validators:  rfc1123NameValidators(),
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the Kargo warehouse to look up.",
				Validators:  rfc1123NameValidators(),
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique identifier of the warehouse in project/name format.",
			},
			"subscription": warehouseDataSourceSubscriptionAttribute(),
			"status":       warehouseDataSourceStatusAttribute(),
			"freight":      warehouseDataSourceFreightAttribute(),
		},
	}
}

func (d *WarehouseDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *WarehouseDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data WarehouseDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	project := data.Project.ValueString()
	name := data.Name.ValueString()
	warehouse, err := d.client.GetWarehouse(ctx, project, name)
	if err != nil {
		if client.IsNotFound(err) {
			resp.Diagnostics.AddError("Warehouse not found", fmt.Sprintf("Warehouse %q does not exist or is being deleted.", warehouseID(project, name)))
			return
		}
		resp.Diagnostics.AddError("Failed to read warehouse", fmt.Sprintf("Unable to read warehouse %q: %s", warehouseID(project, name), err.Error()))
		return
	}
	if warehouse == nil {
		resp.Diagnostics.AddError("Warehouse not found", fmt.Sprintf("Warehouse %q does not exist or is being deleted.", warehouseID(project, name)))
		return
	}

	freight, err := d.client.ListWarehouseFreight(ctx, project, name)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read warehouse freight", fmt.Sprintf("Unable to read freight for warehouse %q: %s", warehouseID(project, name), err.Error()))
		return
	}

	newData := flattenWarehouseDataSource(project, warehouse, freight)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newData)...)
}

func warehouseDataSourceSubscriptionAttribute() schema.Attribute {
	return schema.ListNestedAttribute{
		Computed:    true,
		Description: "Artifact subscriptions for the warehouse.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"image": schema.SingleNestedAttribute{
					Computed:    true,
					Description: "Container image repository subscription.",
					Attributes: map[string]schema.Attribute{
						"repo_url":               schema.StringAttribute{Computed: true, Description: "The image repository URL without a tag."},
						"semver_constraint":      schema.StringAttribute{Computed: true, Description: "SemVer constraint for acceptable image tags."},
						"tag_selection_strategy": schema.StringAttribute{Computed: true, Description: "Image tag selection strategy."},
						"platform":               schema.StringAttribute{Computed: true, Description: "Target image platform, such as linux/amd64."},
					},
				},
				"git": schema.SingleNestedAttribute{
					Computed:    true,
					Description: "Git repository subscription.",
					Attributes: map[string]schema.Attribute{
						"repo_url":          schema.StringAttribute{Computed: true, Description: "The Git repository URL."},
						"branch":            schema.StringAttribute{Computed: true, Description: "Branch to watch."},
						"semver_constraint": schema.StringAttribute{Computed: true, Description: "SemVer constraint for acceptable Git tags."},
					},
				},
				"chart": schema.SingleNestedAttribute{
					Computed:    true,
					Description: "Helm chart repository subscription.",
					Attributes: map[string]schema.Attribute{
						"repo_url":          schema.StringAttribute{Computed: true, Description: "The Helm chart repository URL."},
						"name":              schema.StringAttribute{Computed: true, Description: "The chart name for classic chart repositories."},
						"semver_constraint": schema.StringAttribute{Computed: true, Description: "SemVer constraint for acceptable chart versions."},
					},
				},
			},
		},
	}
}

func warehouseDataSourceStatusAttribute() schema.Attribute {
	return schema.SingleNestedAttribute{
		Computed:    true,
		Description: "The current status of the warehouse.",
		Attributes: map[string]schema.Attribute{
			"conditions": schema.ListNestedAttribute{
				Computed:    true,
				Description: "The conditions of the warehouse, sorted by type.",
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
			"last_handled_refresh": schema.StringAttribute{
				Computed:    true,
				Description: "The most recent refresh annotation value handled by the Warehouse controller.",
			},
			"observed_generation": schema.Int64Attribute{
				Computed:    true,
				Description: "The Warehouse generation most recently observed by the controller.",
			},
			"last_freight_id": schema.StringAttribute{
				Computed:    true,
				Description: "The name of the most recent Freight produced by the Warehouse.",
			},
		},
	}
}

func warehouseDataSourceFreightAttribute() schema.Attribute {
	return schema.ListNestedAttribute{
		Computed:    true,
		Description: "The current Freight item matching status.last_freight_id. Historical Freight is intentionally not exposed.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"name":        schema.StringAttribute{Computed: true, Description: "The Freight name."},
				"alias":       schema.StringAttribute{Computed: true, Description: "The Freight alias."},
				"origin_kind": schema.StringAttribute{Computed: true, Description: "The kind of resource that produced the Freight."},
				"origin_name": schema.StringAttribute{Computed: true, Description: "The name of the resource that produced the Freight."},
				"commits":     freightCommitAttribute(),
				"images":      freightImageAttribute(),
				"charts":      freightChartAttribute(),
				"artifacts":   freightArtifactAttribute(),
				"currently_in": schema.ListNestedAttribute{
					Computed:    true,
					Description: "Stages where this Freight is currently in use, sorted by stage name.",
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"stage": schema.StringAttribute{Computed: true, Description: "The stage name."},
							"since": schema.StringAttribute{Computed: true, Description: "When the Stage started using the Freight."},
						},
					},
				},
				"verified_in": schema.ListNestedAttribute{
					Computed:    true,
					Description: "Stages where this Freight has been verified, sorted by stage name.",
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"stage":        schema.StringAttribute{Computed: true, Description: "The stage name."},
							"verified_at":  schema.StringAttribute{Computed: true, Description: "When the Freight was verified in the Stage."},
							"longest_soak": schema.StringAttribute{Computed: true, Description: "The longest completed soak duration for the Stage."},
						},
					},
				},
				"approved_for": schema.ListNestedAttribute{
					Computed:    true,
					Description: "Stages this Freight is approved for, sorted by stage name.",
					NestedObject: schema.NestedAttributeObject{
						Attributes: map[string]schema.Attribute{
							"stage":       schema.StringAttribute{Computed: true, Description: "The stage name."},
							"approved_at": schema.StringAttribute{Computed: true, Description: "When the Freight was approved for the Stage."},
						},
					},
				},
			},
		},
	}
}

func freightCommitAttribute() schema.Attribute {
	return schema.ListNestedAttribute{
		Computed:    true,
		Description: "Git commits included in the Freight.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"repo_url":  schema.StringAttribute{Computed: true, Description: "The Git repository URL."},
				"id":        schema.StringAttribute{Computed: true, Description: "The commit ID."},
				"branch":    schema.StringAttribute{Computed: true, Description: "The branch where the commit was found."},
				"tag":       schema.StringAttribute{Computed: true, Description: "The tag that resolved to the commit."},
				"message":   schema.StringAttribute{Computed: true, Description: "The commit message subject."},
				"author":    schema.StringAttribute{Computed: true, Description: "The commit author."},
				"committer": schema.StringAttribute{Computed: true, Description: "The commit committer."},
			},
		},
	}
}

func freightImageAttribute() schema.Attribute {
	return schema.ListNestedAttribute{
		Computed:    true,
		Description: "Container images included in the Freight.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"repo_url": schema.StringAttribute{Computed: true, Description: "The image repository URL."},
				"tag":      schema.StringAttribute{Computed: true, Description: "The image tag."},
				"digest":   schema.StringAttribute{Computed: true, Description: "The image digest."},
			},
		},
	}
}

func freightChartAttribute() schema.Attribute {
	return schema.ListNestedAttribute{
		Computed:    true,
		Description: "Helm charts included in the Freight.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"repo_url": schema.StringAttribute{Computed: true, Description: "The chart repository URL."},
				"name":     schema.StringAttribute{Computed: true, Description: "The chart name."},
				"version":  schema.StringAttribute{Computed: true, Description: "The chart version."},
			},
		},
	}
}

func freightArtifactAttribute() schema.Attribute {
	return schema.ListNestedAttribute{
		Computed:    true,
		Description: "Generic artifacts included in the Freight.",
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"artifact_type":     schema.StringAttribute{Computed: true, Description: "The artifact type."},
				"subscription_name": schema.StringAttribute{Computed: true, Description: "The subscription that discovered the artifact."},
				"version":           schema.StringAttribute{Computed: true, Description: "The artifact version."},
			},
		},
	}
}

func flattenWarehouseDataSource(project string, warehouse *client.Warehouse, freight []client.Freight) WarehouseDataSourceModel {
	resolvedProject := warehouse.Metadata.Namespace
	if resolvedProject == "" {
		resolvedProject = project
	}

	data := WarehouseDataSourceModel{
		Project:      types.StringValue(resolvedProject),
		Name:         types.StringValue(warehouse.Metadata.Name),
		ID:           types.StringValue(warehouseID(resolvedProject, warehouse.Metadata.Name)),
		Subscription: flattenWarehouseDataSourceSubscriptions(warehouse.Spec.Subscriptions),
		Status:       flattenWarehouseDataSourceStatus(warehouse.Status),
		Freight:      flattenWarehouseDataSourceFreight(warehouse.Status.LastFreightID, freight),
	}
	return data
}

func flattenWarehouseDataSourceSubscriptions(subs []client.WarehouseSubscription) []WarehouseSubscriptionModel {
	result := make([]WarehouseSubscriptionModel, 0, len(subs))
	for _, sub := range subs {
		flattened := WarehouseSubscriptionModel{}
		if sub.Image != nil {
			flattened.Image = &WarehouseImageSubscriptionModel{
				RepoURL:              types.StringValue(sub.Image.RepoURL),
				SemverConstraint:     computedStringValue(sub.Image.Constraint),
				TagSelectionStrategy: computedStringValue(sub.Image.ImageSelectionStrategy),
				Platform:             computedStringValue(sub.Image.Platform),
			}
		}
		if sub.Git != nil {
			flattened.Git = &WarehouseGitSubscriptionModel{
				RepoURL:          types.StringValue(sub.Git.RepoURL),
				Branch:           computedStringValue(sub.Git.Branch),
				SemverConstraint: computedStringValue(sub.Git.SemverConstraint),
			}
		}
		if sub.Chart != nil {
			flattened.Chart = &WarehouseChartSubscriptionModel{
				RepoURL:          types.StringValue(sub.Chart.RepoURL),
				Name:             computedStringValue(sub.Chart.Name),
				SemverConstraint: computedStringValue(sub.Chart.SemverConstraint),
			}
		}
		result = append(result, flattened)
	}
	return result
}

func flattenWarehouseDataSourceStatus(status client.WarehouseStatus) *WarehouseDataSourceStatusModel {
	conditions := append([]client.WarehouseCondition(nil), status.Conditions...)
	sort.SliceStable(conditions, func(i, j int) bool {
		return conditions[i].Type < conditions[j].Type
	})

	conditionModels := make([]WarehouseDataSourceConditionModel, 0, len(conditions))
	for _, condition := range conditions {
		conditionModels = append(conditionModels, WarehouseDataSourceConditionModel{
			Type:               types.StringValue(condition.Type),
			Status:             types.StringValue(condition.Status),
			Reason:             computedStringValue(condition.Reason),
			Message:            computedStringValue(condition.Message),
			LastTransitionTime: computedStringValue(condition.LastTransitionTime.String()),
		})
	}

	observedGeneration := types.Int64Null()
	if status.ObservedGeneration.Set() {
		observedGeneration = types.Int64Value(status.ObservedGeneration.Value())
	}

	return &WarehouseDataSourceStatusModel{
		Conditions:         conditionModels,
		LastHandledRefresh: computedStringValue(status.LastHandledRefresh),
		ObservedGeneration: observedGeneration,
		LastFreightID:      computedStringValue(status.LastFreightID),
	}
}

func flattenWarehouseDataSourceFreight(lastFreightID string, freight []client.Freight) []WarehouseDataSourceFreightModel {
	if lastFreightID == "" {
		return []WarehouseDataSourceFreightModel{}
	}

	sortedFreight := append([]client.Freight(nil), freight...)
	sort.SliceStable(sortedFreight, func(i, j int) bool {
		return sortedFreight[i].Metadata.Name < sortedFreight[j].Metadata.Name
	})

	for i := range sortedFreight {
		item := &sortedFreight[i]
		if item.Metadata.Name != lastFreightID {
			continue
		}
		return []WarehouseDataSourceFreightModel{flattenWarehouseFreight(*item)}
	}
	return []WarehouseDataSourceFreightModel{}
}

func flattenWarehouseFreight(freight client.Freight) WarehouseDataSourceFreightModel {
	model := WarehouseDataSourceFreightModel{
		Name:        types.StringValue(freight.Metadata.Name),
		Alias:       computedStringValue(freight.Alias),
		OriginKind:  types.StringNull(),
		OriginName:  types.StringNull(),
		Commits:     make([]WarehouseDataSourceFreightCommitModel, 0, len(freight.Commits)),
		Images:      make([]WarehouseDataSourceFreightImageModel, 0, len(freight.Images)),
		Charts:      make([]WarehouseDataSourceFreightChartModel, 0, len(freight.Charts)),
		Artifacts:   make([]WarehouseDataSourceFreightArtifactModel, 0, len(freight.Artifacts)),
		CurrentlyIn: flattenCurrentlyIn(freight.Status.CurrentlyIn),
		VerifiedIn:  flattenVerifiedIn(freight.Status.VerifiedIn),
		ApprovedFor: flattenApprovedFor(freight.Status.ApprovedFor),
	}
	if freight.Origin != nil {
		model.OriginKind = computedStringValue(freight.Origin.Kind)
		model.OriginName = computedStringValue(freight.Origin.Name)
	}

	for _, commit := range freight.Commits {
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
	for _, image := range freight.Images {
		model.Images = append(model.Images, WarehouseDataSourceFreightImageModel{
			RepoURL: types.StringValue(image.RepoURL),
			Tag:     computedStringValue(image.Tag),
			Digest:  computedStringValue(image.Digest),
		})
	}
	for _, chart := range freight.Charts {
		model.Charts = append(model.Charts, WarehouseDataSourceFreightChartModel{
			RepoURL: types.StringValue(chart.RepoURL),
			Name:    computedStringValue(chart.Name),
			Version: computedStringValue(chart.Version),
		})
	}
	for _, artifact := range freight.Artifacts {
		model.Artifacts = append(model.Artifacts, WarehouseDataSourceFreightArtifactModel{
			ArtifactType:     types.StringValue(artifact.ArtifactType),
			SubscriptionName: types.StringValue(artifact.SubscriptionName),
			Version:          types.StringValue(artifact.Version),
		})
	}
	return model
}

func flattenCurrentlyIn(stages map[string]client.CurrentStage) []WarehouseDataSourceCurrentStageModel {
	keys := sortedMapKeys(stages)
	result := make([]WarehouseDataSourceCurrentStageModel, 0, len(keys))
	for _, stage := range keys {
		result = append(result, WarehouseDataSourceCurrentStageModel{
			Stage: types.StringValue(stage),
			Since: computedStringValue(stages[stage].Since.String()),
		})
	}
	return result
}

func flattenVerifiedIn(stages map[string]client.VerifiedStage) []WarehouseDataSourceVerifiedStageModel {
	keys := sortedMapKeys(stages)
	result := make([]WarehouseDataSourceVerifiedStageModel, 0, len(keys))
	for _, stage := range keys {
		result = append(result, WarehouseDataSourceVerifiedStageModel{
			Stage:       types.StringValue(stage),
			VerifiedAt:  computedStringValue(stages[stage].VerifiedAt.String()),
			LongestSoak: computedStringValue(stages[stage].LongestSoak.String()),
		})
	}
	return result
}

func flattenApprovedFor(stages map[string]client.ApprovedStage) []WarehouseDataSourceApprovedStageModel {
	keys := sortedMapKeys(stages)
	result := make([]WarehouseDataSourceApprovedStageModel, 0, len(keys))
	for _, stage := range keys {
		result = append(result, WarehouseDataSourceApprovedStageModel{
			Stage:      types.StringValue(stage),
			ApprovedAt: computedStringValue(stages[stage].ApprovedAt.String()),
		})
	}
	return result
}

func sortedMapKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func computedStringValue(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}
