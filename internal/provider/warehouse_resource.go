package provider

import (
	"context"
	"fmt"
	"strings"

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
	_ resource.Resource                = &WarehouseResource{}
	_ resource.ResourceWithImportState = &WarehouseResource{}
)

type WarehouseResource struct {
	client client.KargoClient
}

type WarehouseResourceModel struct {
	Project      types.String                 `tfsdk:"project"`
	Name         types.String                 `tfsdk:"name"`
	ID           types.String                 `tfsdk:"id"`
	Subscription []WarehouseSubscriptionModel `tfsdk:"subscription"`
}

type WarehouseSubscriptionModel struct {
	Image *WarehouseImageSubscriptionModel `tfsdk:"image"`
	Git   *WarehouseGitSubscriptionModel   `tfsdk:"git"`
	Chart *WarehouseChartSubscriptionModel `tfsdk:"chart"`
}

type WarehouseImageSubscriptionModel struct {
	RepoURL              types.String `tfsdk:"repo_url"`
	SemverConstraint     types.String `tfsdk:"semver_constraint"`
	TagSelectionStrategy types.String `tfsdk:"tag_selection_strategy"`
	Platform             types.String `tfsdk:"platform"`
}

type WarehouseGitSubscriptionModel struct {
	RepoURL          types.String `tfsdk:"repo_url"`
	Branch           types.String `tfsdk:"branch"`
	SemverConstraint types.String `tfsdk:"semver_constraint"`
}

type WarehouseChartSubscriptionModel struct {
	RepoURL          types.String `tfsdk:"repo_url"`
	Name             types.String `tfsdk:"name"`
	SemverConstraint types.String `tfsdk:"semver_constraint"`
}

func NewWarehouseResource() resource.Resource {
	return &WarehouseResource{}
}

func (r *WarehouseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_warehouse"
}

func (r *WarehouseResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Kargo warehouse. A warehouse subscribes to artifact sources and creates Freight.",
		Attributes: map[string]schema.Attribute{
			"project": schema.StringAttribute{
				Required:    true,
				Description: "The Kargo project that contains the warehouse.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: rfc1123NameValidators(),
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the Kargo warehouse.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: rfc1123NameValidators(),
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique identifier of the warehouse in project/name format.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"subscription": schema.ListNestedBlock{
				Description: "Ordered artifact subscriptions for the warehouse. Subscription order is user-significant.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedBlockObject{
					Blocks: map[string]schema.Block{
						"image": schema.SingleNestedBlock{
							Description: "Container image repository subscription.",
							Attributes: map[string]schema.Attribute{
								"repo_url": schema.StringAttribute{
									Required:    true,
									Description: "The image repository URL without a tag.",
								},
								"semver_constraint": schema.StringAttribute{
									Optional:    true,
									Description: "SemVer constraint for acceptable image tags.",
								},
								"tag_selection_strategy": schema.StringAttribute{
									Optional:    true,
									Description: "Image tag selection strategy.",
									Validators: []validator.String{
										stringvalidator.OneOf("Digest", "Lexical", "NewestBuild", "SemVer"),
									},
								},
								"platform": schema.StringAttribute{
									Optional:    true,
									Description: "Target image platform, such as linux/amd64.",
								},
							},
						},
						"git": schema.SingleNestedBlock{
							Description: "Git repository subscription.",
							Attributes: map[string]schema.Attribute{
								"repo_url": schema.StringAttribute{
									Required:    true,
									Description: "The Git repository URL.",
								},
								"branch": schema.StringAttribute{
									Optional:    true,
									Description: "Branch to watch.",
								},
								"semver_constraint": schema.StringAttribute{
									Optional:    true,
									Description: "SemVer constraint for acceptable Git tags.",
								},
							},
						},
						"chart": schema.SingleNestedBlock{
							Description: "Helm chart repository subscription.",
							Attributes: map[string]schema.Attribute{
								"repo_url": schema.StringAttribute{
									Required:    true,
									Description: "The Helm chart repository URL.",
								},
								"name": schema.StringAttribute{
									Optional:    true,
									Description: "The chart name for classic chart repositories.",
								},
								"semver_constraint": schema.StringAttribute{
									Optional:    true,
									Description: "SemVer constraint for acceptable chart versions.",
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *WarehouseResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *WarehouseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data WarehouseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	spec, err := expandWarehouseSpec(data.Subscription)
	if err != nil {
		resp.Diagnostics.AddError("Invalid warehouse subscription", err.Error())
		return
	}

	warehouse, err := r.client.CreateWarehouse(ctx, data.Project.ValueString(), data.Name.ValueString(), spec)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create warehouse", err.Error())
		return
	}
	if warehouse == nil {
		resp.Diagnostics.AddError("Failed to create warehouse", "Kargo returned no warehouse after creation.")
		return
	}

	newData := flattenWarehouse(data.Project.ValueString(), warehouse, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newData)...)
}

func (r *WarehouseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data WarehouseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	warehouse, err := r.client.GetWarehouse(ctx, data.Project.ValueString(), data.Name.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read warehouse", err.Error())
		return
	}

	if warehouse == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	newData := flattenWarehouse(data.Project.ValueString(), warehouse, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newData)...)
}

func (r *WarehouseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data WarehouseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	spec, err := expandWarehouseSpec(data.Subscription)
	if err != nil {
		resp.Diagnostics.AddError("Invalid warehouse subscription", err.Error())
		return
	}

	warehouse, err := r.client.UpdateWarehouse(ctx, data.Project.ValueString(), data.Name.ValueString(), spec)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update warehouse", err.Error())
		return
	}
	if warehouse == nil {
		resp.Diagnostics.AddError("Failed to update warehouse", "Kargo returned no warehouse after update.")
		return
	}

	newData := flattenWarehouse(data.Project.ValueString(), warehouse, &data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &newData)...)
}

func (r *WarehouseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data WarehouseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteWarehouse(ctx, data.Project.ValueString(), data.Name.ValueString()); err != nil {
		if client.IsNotFound(err) {
			return
		}
		resp.Diagnostics.AddError("Failed to delete warehouse", err.Error())
	}
}

func (r *WarehouseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	project, name, err := parseWarehouseID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid warehouse import ID", err.Error())
		return
	}

	warehouse, err := r.client.GetWarehouse(ctx, project, name)
	if err != nil {
		if client.IsNotFound(err) {
			resp.Diagnostics.AddError("Warehouse not found", fmt.Sprintf("Warehouse %q does not exist.", req.ID))
			return
		}
		resp.Diagnostics.AddError("Failed to read warehouse", err.Error())
		return
	}
	if warehouse == nil {
		resp.Diagnostics.AddError("Warehouse not found", fmt.Sprintf("Warehouse %q does not exist.", req.ID))
		return
	}

	data := flattenWarehouse(project, warehouse, nil)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func warehouseID(project, name string) string {
	return project + "/" + name
}

func parseWarehouseID(id string) (project, name string, err error) {
	project, name, ok := strings.Cut(id, "/")
	if !ok || project == "" || name == "" || strings.Contains(name, "/") {
		return "", "", fmt.Errorf("expected import ID in project/name format")
	}
	return project, name, nil
}

func expandWarehouseSpec(subs []WarehouseSubscriptionModel) (client.WarehouseSpec, error) {
	if len(subs) == 0 {
		return client.WarehouseSpec{}, fmt.Errorf("warehouse must have at least one subscription")
	}

	spec := client.WarehouseSpec{
		Subscriptions: make([]client.WarehouseSubscription, 0, len(subs)),
	}

	for i, sub := range subs {
		kinds := 0
		if sub.Image != nil {
			kinds++
		}
		if sub.Git != nil {
			kinds++
		}
		if sub.Chart != nil {
			kinds++
		}
		if kinds != 1 {
			return client.WarehouseSpec{}, fmt.Errorf("subscription %d must set exactly one of image, git, or chart", i)
		}

		expanded := client.WarehouseSubscription{}
		if sub.Image != nil {
			expanded.Image = &client.ImageSubscription{
				RepoURL:                sub.Image.RepoURL.ValueString(),
				Constraint:             valueString(sub.Image.SemverConstraint),
				ImageSelectionStrategy: valueString(sub.Image.TagSelectionStrategy),
				Platform:               valueString(sub.Image.Platform),
			}
		}
		if sub.Git != nil {
			git := &client.GitSubscription{
				RepoURL:          sub.Git.RepoURL.ValueString(),
				Branch:           valueString(sub.Git.Branch),
				SemverConstraint: valueString(sub.Git.SemverConstraint),
			}
			if git.SemverConstraint != "" {
				git.CommitSelectionStrategy = "SemVer"
			}
			expanded.Git = git
		}
		if sub.Chart != nil {
			expanded.Chart = &client.ChartSubscription{
				RepoURL:          sub.Chart.RepoURL.ValueString(),
				Name:             valueString(sub.Chart.Name),
				SemverConstraint: valueString(sub.Chart.SemverConstraint),
			}
		}
		spec.Subscriptions = append(spec.Subscriptions, expanded)
	}

	return spec, nil
}

func flattenWarehouse(project string, warehouse *client.Warehouse, prior *WarehouseResourceModel) WarehouseResourceModel {
	resolvedProject := warehouse.Metadata.Namespace
	if resolvedProject == "" {
		resolvedProject = project
	}

	data := WarehouseResourceModel{
		Project:      types.StringValue(resolvedProject),
		Name:         types.StringValue(warehouse.Metadata.Name),
		ID:           types.StringValue(warehouseID(resolvedProject, warehouse.Metadata.Name)),
		Subscription: make([]WarehouseSubscriptionModel, 0, len(warehouse.Spec.Subscriptions)),
	}

	for i, sub := range warehouse.Spec.Subscriptions {
		var priorSub *WarehouseSubscriptionModel
		if prior != nil && i < len(prior.Subscription) {
			priorSub = &prior.Subscription[i]
		}

		flattened := WarehouseSubscriptionModel{}
		if sub.Image != nil {
			var priorImage *WarehouseImageSubscriptionModel
			if priorSub != nil {
				priorImage = priorSub.Image
			}
			flattened.Image = &WarehouseImageSubscriptionModel{
				RepoURL:              types.StringValue(sub.Image.RepoURL),
				SemverConstraint:     optionalStringValue(sub.Image.Constraint, priorImageString(priorImage, "semver_constraint")),
				TagSelectionStrategy: optionalStringValue(sub.Image.ImageSelectionStrategy, priorImageString(priorImage, "tag_selection_strategy")),
				Platform:             optionalStringValue(sub.Image.Platform, priorImageString(priorImage, "platform")),
			}
		}
		if sub.Git != nil {
			var priorGit *WarehouseGitSubscriptionModel
			if priorSub != nil {
				priorGit = priorSub.Git
			}
			flattened.Git = &WarehouseGitSubscriptionModel{
				RepoURL:          types.StringValue(sub.Git.RepoURL),
				Branch:           optionalStringValue(sub.Git.Branch, priorGitString(priorGit, "branch")),
				SemverConstraint: optionalStringValue(sub.Git.SemverConstraint, priorGitString(priorGit, "semver_constraint")),
			}
		}
		if sub.Chart != nil {
			var priorChart *WarehouseChartSubscriptionModel
			if priorSub != nil {
				priorChart = priorSub.Chart
			}
			flattened.Chart = &WarehouseChartSubscriptionModel{
				RepoURL:          types.StringValue(sub.Chart.RepoURL),
				Name:             optionalStringValue(sub.Chart.Name, priorChartString(priorChart, "name")),
				SemverConstraint: optionalStringValue(sub.Chart.SemverConstraint, priorChartString(priorChart, "semver_constraint")),
			}
		}
		data.Subscription = append(data.Subscription, flattened)
	}

	return data
}

func valueString(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return value.ValueString()
}

func optionalStringValue(value string, prior types.String) types.String {
	if prior.IsNull() || prior.IsUnknown() {
		return types.StringNull()
	}
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func priorImageString(model *WarehouseImageSubscriptionModel, field string) types.String {
	if model == nil {
		return types.StringValue("__import__")
	}
	switch field {
	case "semver_constraint":
		return model.SemverConstraint
	case "tag_selection_strategy":
		return model.TagSelectionStrategy
	case "platform":
		return model.Platform
	default:
		return types.StringNull()
	}
}

func priorGitString(model *WarehouseGitSubscriptionModel, field string) types.String {
	if model == nil {
		return types.StringValue("__import__")
	}
	switch field {
	case "branch":
		return model.Branch
	case "semver_constraint":
		return model.SemverConstraint
	default:
		return types.StringNull()
	}
}

func priorChartString(model *WarehouseChartSubscriptionModel, field string) types.String {
	if model == nil {
		return types.StringValue("__import__")
	}
	switch field {
	case "name":
		return model.Name
	case "semver_constraint":
		return model.SemverConstraint
	default:
		return types.StringNull()
	}
}
