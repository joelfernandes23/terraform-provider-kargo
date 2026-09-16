package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
	Project                 types.String                 `tfsdk:"project"`
	Name                    types.String                 `tfsdk:"name"`
	ID                      types.String                 `tfsdk:"id"`
	Shard                   types.String                 `tfsdk:"shard"`
	Interval                durationValue                `tfsdk:"interval"`
	FreightCreationPolicy   types.String                 `tfsdk:"freight_creation_policy"`
	FreightCreationCriteria types.String                 `tfsdk:"freight_creation_criteria"`
	Subscription            []WarehouseSubscriptionModel `tfsdk:"subscription"`
}

type WarehouseSubscriptionModel struct {
	Name    types.String                       `tfsdk:"name"`
	Image   *WarehouseImageSubscriptionModel   `tfsdk:"image"`
	Git     *WarehouseGitSubscriptionModel     `tfsdk:"git"`
	Chart   *WarehouseChartSubscriptionModel   `tfsdk:"chart"`
	Generic *WarehouseGenericSubscriptionModel `tfsdk:"generic"`
}

type WarehouseImageSubscriptionModel struct {
	RepoURL               types.String `tfsdk:"repo_url"`
	SemverConstraint      types.String `tfsdk:"semver_constraint"`
	TagSelectionStrategy  types.String `tfsdk:"tag_selection_strategy"`
	Platform              types.String `tfsdk:"platform"`
	AllowTags             types.String `tfsdk:"allow_tags"`
	AllowTagsRegexes      types.List   `tfsdk:"allow_tags_regexes"`
	IgnoreTagsRegexes     types.List   `tfsdk:"ignore_tags_regexes"`
	IgnoreTags            types.List   `tfsdk:"ignore_tags"`
	CacheByTag            types.Bool   `tfsdk:"cache_by_tag"`
	DiscoveryLimit        types.Int64  `tfsdk:"discovery_limit"`
	InsecureSkipTLSVerify types.Bool   `tfsdk:"insecure_skip_tls_verify"`
	StrictSemvers         types.Bool   `tfsdk:"strict_semvers"`
}

type WarehouseGitSubscriptionModel struct {
	RepoURL                 types.String `tfsdk:"repo_url"`
	Branch                  types.String `tfsdk:"branch"`
	SemverConstraint        types.String `tfsdk:"semver_constraint"`
	CommitSelectionStrategy types.String `tfsdk:"commit_selection_strategy"`
	AllowTags               types.String `tfsdk:"allow_tags"`
	AllowTagsRegexes        types.List   `tfsdk:"allow_tags_regexes"`
	IgnoreTagsRegexes       types.List   `tfsdk:"ignore_tags_regexes"`
	IgnoreTags              types.List   `tfsdk:"ignore_tags"`
	IncludePaths            types.List   `tfsdk:"include_paths"`
	ExcludePaths            types.List   `tfsdk:"exclude_paths"`
	ExpressionFilter        types.String `tfsdk:"expression_filter"`
	Since                   types.String `tfsdk:"since"`
	Blobless                types.Bool   `tfsdk:"blobless"`
	DiscoveryLimit          types.Int64  `tfsdk:"discovery_limit"`
	InsecureSkipTLSVerify   types.Bool   `tfsdk:"insecure_skip_tls_verify"`
	StrictSemvers           types.Bool   `tfsdk:"strict_semvers"`
}

type WarehouseChartSubscriptionModel struct {
	RepoURL               types.String `tfsdk:"repo_url"`
	Name                  types.String `tfsdk:"name"`
	SemverConstraint      types.String `tfsdk:"semver_constraint"`
	DiscoveryLimit        types.Int64  `tfsdk:"discovery_limit"`
	InsecureSkipTLSVerify types.Bool   `tfsdk:"insecure_skip_tls_verify"`
}

type WarehouseGenericSubscriptionModel struct {
	Type           types.String `tfsdk:"type"`
	Config         types.String `tfsdk:"config"`
	DiscoveryLimit types.Int64  `tfsdk:"discovery_limit"`
}

func NewWarehouseResource() resource.Resource {
	return &WarehouseResource{}
}

func (r *WarehouseResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_warehouse"
}

func (r *WarehouseResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provides a Kargo Warehouse resource.",
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
				Description: "Warehouse identifier in `project/name` format.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"shard":                     optionalWarehouseString("Shard that the warehouse belongs to."),
			"interval":                  schema.StringAttribute{Optional: true, Computed: true, CustomType: durationType{}, Description: "How often Kargo discovers artifacts, such as 5m."},
			"freight_creation_policy":   schema.StringAttribute{Optional: true, Computed: true, Description: "Whether Kargo creates Freight automatically or manually.", Validators: []validator.String{stringvalidator.OneOf("Automatic", "Manual")}},
			"freight_creation_criteria": optionalWarehouseString("Expression that must evaluate to true before automatic Freight creation."),
		},
		Blocks: map[string]schema.Block{
			"subscription": schema.ListNestedBlock{
				Description: "Ordered artifact subscriptions for the warehouse.",
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": optionalWarehouseString("Unique subscription name. Required for generic subscriptions; not supported for image, Git, or chart subscriptions."),
					},
					Blocks: map[string]schema.Block{
						"image": schema.SingleNestedBlock{
							Description: "Container image repository subscription.",
							Attributes:  warehouseImageAttributes(),
						},
						"git": schema.SingleNestedBlock{
							Description: "Git repository subscription.",
							Attributes:  warehouseGitAttributes(),
						},
						"chart": schema.SingleNestedBlock{
							Description: "Helm chart repository subscription.",
							Attributes:  warehouseChartAttributes(),
						},
						"generic": schema.SingleNestedBlock{Description: "Subscription implemented by a Kargo extension.", Attributes: map[string]schema.Attribute{
							"type":            schema.StringAttribute{Optional: true, Description: "Kargo subscription type. Required when generic is set."},
							"config":          schema.StringAttribute{Optional: true, Description: "JSON object understood by the selected Kargo subscription type."},
							"discovery_limit": optionalWarehouseInt("Maximum number of artifacts to discover. Supported values are 1 through 100. Defaults to 20."),
						}},
					},
				},
			},
		},
	}
}

func optionalWarehouseString(description string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Computed: true, Description: description}
}

func optionalWarehouseBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{Optional: true, Computed: true, Description: description}
}

func optionalWarehouseInt(description string) schema.Int64Attribute {
	return schema.Int64Attribute{Optional: true, Computed: true, Description: description, Validators: []validator.Int64{int64validator.Between(1, 100)}}
}

func optionalWarehouseStrings(description string) schema.ListAttribute {
	return schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType, Description: description}
}

func warehouseImageAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"repo_url":                 optionalWarehouseString("Image repository URL without a tag. Required when image is set."),
		"semver_constraint":        optionalWarehouseString("Selection-strategy constraint for image tags."),
		"tag_selection_strategy":   schema.StringAttribute{Optional: true, Computed: true, Description: "Image tag selection strategy.", Validators: []validator.String{stringvalidator.OneOf("Digest", "Lexical", "NewestBuild", "SemVer")}},
		"platform":                 optionalWarehouseString("Target image platform, such as linux/amd64."),
		"allow_tags":               optionalWarehouseString("Deprecated regular expression for image tags to include."),
		"allow_tags_regexes":       optionalWarehouseStrings("Regular expressions for tags to include."),
		"ignore_tags_regexes":      optionalWarehouseStrings("Regular expressions for tags to exclude."),
		"ignore_tags":              optionalWarehouseStrings("Deprecated exact image tags to exclude."),
		"cache_by_tag":             optionalWarehouseBool("Whether image metadata is cached by tag."),
		"discovery_limit":          optionalWarehouseInt("Maximum number of image references to discover."),
		"insecure_skip_tls_verify": optionalWarehouseBool("Whether to skip TLS certificate verification."),
		"strict_semvers":           optionalWarehouseBool("Whether SemVer selection accepts only strict versions."),
	}
}

func warehouseGitAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"repo_url":                  optionalWarehouseString("Git repository URL. Required when git is set."),
		"branch":                    optionalWarehouseString("Branch to watch."),
		"semver_constraint":         optionalWarehouseString("Constraint for SemVer commit selection."),
		"commit_selection_strategy": schema.StringAttribute{Optional: true, Computed: true, Description: "Strategy for selecting commits.", Validators: []validator.String{stringvalidator.OneOf("Lexical", "NewestFromBranch", "NewestTag", "SemVer")}},
		"allow_tags":                optionalWarehouseString("Deprecated regular expression for Git tags to include."),
		"allow_tags_regexes":        optionalWarehouseStrings("Regular expressions for Git tags to include."),
		"ignore_tags_regexes":       optionalWarehouseStrings("Regular expressions for Git tags to exclude."),
		"ignore_tags":               optionalWarehouseStrings("Deprecated exact Git tags to exclude."),
		"include_paths":             optionalWarehouseStrings("Paths that trigger Freight creation."),
		"exclude_paths":             optionalWarehouseStrings("Paths that do not trigger Freight creation."),
		"expression_filter":         optionalWarehouseString("Expression used to filter candidate commits or tags."),
		"since":                     optionalWarehouseString("RFC 3339 cutoff for commit discovery."),
		"blobless":                  optionalWarehouseBool("Whether to use blobless Git clones."),
		"discovery_limit":           optionalWarehouseInt("Maximum number of commits to discover."),
		"insecure_skip_tls_verify":  optionalWarehouseBool("Whether to skip TLS certificate verification."),
		"strict_semvers":            optionalWarehouseBool("Whether SemVer selection accepts only strict versions."),
	}
}

func warehouseChartAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"repo_url":                 optionalWarehouseString("Helm chart repository URL. Required when chart is set."),
		"name":                     optionalWarehouseString("Chart name for a classic chart repository."),
		"semver_constraint":        optionalWarehouseString("Constraint for chart versions."),
		"discovery_limit":          optionalWarehouseInt("Maximum number of chart versions to discover."),
		"insecure_skip_tls_verify": optionalWarehouseBool("Whether to skip TLS certificate verification."),
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

	spec, err := expandWarehouseResource(&data)
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

	spec, err := expandWarehouseResource(&data)
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
	return expandWarehouseResource(&WarehouseResourceModel{Subscription: subs})
}

func expandWarehouseResource(data *WarehouseResourceModel) (client.WarehouseSpec, error) {
	subs := data.Subscription
	if len(subs) == 0 {
		return client.WarehouseSpec{}, fmt.Errorf("warehouse must have at least one subscription")
	}

	spec := client.WarehouseSpec{
		Shard:                 valueString(data.Shard),
		Interval:              valueString(data.Interval.StringValue),
		FreightCreationPolicy: valueString(data.FreightCreationPolicy),
		Subscriptions:         make([]client.WarehouseSubscription, 0, len(subs)),
	}
	if expression := valueString(data.FreightCreationCriteria); expression != "" {
		spec.FreightCreationCriteria = &client.FreightCreationCriteria{Expression: expression}
	}

	names := make(map[string]bool)
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
		if sub.Generic != nil {
			kinds++
		}
		if kinds != 1 {
			return client.WarehouseSpec{}, fmt.Errorf("subscription %d must set exactly one of image, git, chart, or generic", i)
		}

		expanded := client.WarehouseSubscription{Name: valueString(sub.Name)}
		if sub.Generic == nil && expanded.Name != "" {
			return client.WarehouseSpec{}, fmt.Errorf("subscription %d name is only supported for generic subscriptions", i)
		}
		if sub.Image != nil {
			repoURL, err := requiredWarehouseString(sub.Image.RepoURL, i, "image.repo_url")
			if err != nil {
				return client.WarehouseSpec{}, err
			}
			expanded.Image = &client.ImageSubscription{
				RepoURL:                repoURL,
				AllowTags:              valueString(sub.Image.AllowTags),
				Constraint:             valueString(sub.Image.SemverConstraint),
				ImageSelectionStrategy: valueString(sub.Image.TagSelectionStrategy),
				Platform:               valueString(sub.Image.Platform),
				AllowTagsRegexes:       stringListValue(sub.Image.AllowTagsRegexes),
				IgnoreTagsRegexes:      stringListValue(sub.Image.IgnoreTagsRegexes),
				IgnoreTags:             stringListValue(sub.Image.IgnoreTags),
				CacheByTag:             boolPointer(sub.Image.CacheByTag),
				DiscoveryLimit:         int64Pointer(sub.Image.DiscoveryLimit),
				InsecureSkipTLSVerify:  boolPointer(sub.Image.InsecureSkipTLSVerify),
				StrictSemvers:          boolPointer(sub.Image.StrictSemvers),
			}
		}
		if sub.Git != nil {
			repoURL, err := requiredWarehouseString(sub.Git.RepoURL, i, "git.repo_url")
			if err != nil {
				return client.WarehouseSpec{}, err
			}
			git := &client.GitSubscription{
				RepoURL:                 repoURL,
				AllowTags:               valueString(sub.Git.AllowTags),
				Branch:                  valueString(sub.Git.Branch),
				SemverConstraint:        valueString(sub.Git.SemverConstraint),
				CommitSelectionStrategy: valueString(sub.Git.CommitSelectionStrategy),
				AllowTagsRegexes:        stringListValue(sub.Git.AllowTagsRegexes),
				IgnoreTagsRegexes:       stringListValue(sub.Git.IgnoreTagsRegexes),
				IgnoreTags:              stringListValue(sub.Git.IgnoreTags),
				IncludePaths:            stringListValue(sub.Git.IncludePaths),
				ExcludePaths:            stringListValue(sub.Git.ExcludePaths),
				ExpressionFilter:        valueString(sub.Git.ExpressionFilter),
				Since:                   valueString(sub.Git.Since),
				Blobless:                boolPointer(sub.Git.Blobless),
				DiscoveryLimit:          int64Pointer(sub.Git.DiscoveryLimit),
				InsecureSkipTLSVerify:   boolPointer(sub.Git.InsecureSkipTLSVerify),
				StrictSemvers:           boolPointer(sub.Git.StrictSemvers),
			}
			expanded.Git = git
		}
		if sub.Chart != nil {
			repoURL, err := requiredWarehouseString(sub.Chart.RepoURL, i, "chart.repo_url")
			if err != nil {
				return client.WarehouseSpec{}, err
			}
			expanded.Chart = &client.ChartSubscription{
				RepoURL:               repoURL,
				Name:                  valueString(sub.Chart.Name),
				SemverConstraint:      valueString(sub.Chart.SemverConstraint),
				DiscoveryLimit:        int64Pointer(sub.Chart.DiscoveryLimit),
				InsecureSkipTLSVerify: boolPointer(sub.Chart.InsecureSkipTLSVerify),
			}
		}
		if sub.Generic != nil {
			kind, err := requiredWarehouseString(sub.Generic.Type, i, "generic.type")
			if err != nil {
				return client.WarehouseSpec{}, err
			}
			if kind == "git" || kind == "image" || kind == "chart" {
				return client.WarehouseSpec{}, fmt.Errorf("subscription %d generic.type %q requires its dedicated subscription block", i, kind)
			}
			config, err := requiredWarehouseJSONObject(sub.Generic.Config, i, "generic.config")
			if err != nil {
				return client.WarehouseSpec{}, err
			}
			if expanded.Name == "" || names[expanded.Name] {
				return client.WarehouseSpec{}, fmt.Errorf("subscription %d generic subscription requires a non-empty unique name", i)
			}
			names[expanded.Name] = true
			expanded.Generic = &client.GenericSubscription{Type: kind, Config: config, DiscoveryLimit: int64Pointer(sub.Generic.DiscoveryLimit)}
		}
		spec.Subscriptions = append(spec.Subscriptions, expanded)
	}

	return spec, nil
}

func requiredWarehouseString(value types.String, index int, field string) (string, error) {
	if value.IsNull() || value.IsUnknown() || value.ValueString() == "" {
		return "", fmt.Errorf("subscription %d %s must be set", index, field)
	}
	return value.ValueString(), nil
}

func requiredWarehouseJSONObject(value types.String, index int, field string) (json.RawMessage, error) {
	text, err := requiredWarehouseString(value, index, field)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(text), &object); err != nil {
		return nil, fmt.Errorf("subscription %d %s must be a JSON object: %w", index, field, err)
	}
	if object == nil {
		return nil, fmt.Errorf("subscription %d %s must be a JSON object, not null", index, field)
	}
	return json.RawMessage(text), nil
}

func stringListValue(value types.List) []string {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	var values []string
	if value.ElementsAs(context.Background(), &values, false).HasError() {
		return nil
	}
	return values
}

func boolPointer(value types.Bool) *bool {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	result := value.ValueBool()
	return &result
}
func int64Pointer(value types.Int64) *int64 {
	if value.IsNull() || value.IsUnknown() {
		return nil
	}
	result := value.ValueInt64()
	return &result
}
func optionalBool(value *bool) types.Bool {
	if value == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*value)
}
func optionalInt64(value *int64) types.Int64 {
	if value == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*value)
}
func stringList(values []string) types.List {
	if len(values) == 0 {
		return types.ListNull(types.StringType)
	}
	result, diags := types.ListValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		return types.ListNull(types.StringType)
	}
	return result
}

func priorWarehouseString(model *WarehouseResourceModel, field string) types.String {
	if model == nil {
		return types.StringValue("__import__")
	}
	switch field {
	case "shard":
		return model.Shard
	case "interval":
		return model.Interval.StringValue
	case "freight_creation_policy":
		return model.FreightCreationPolicy
	case "freight_creation_criteria":
		return model.FreightCreationCriteria
	}
	return types.StringNull()
}
func priorSubscriptionString(model *WarehouseSubscriptionModel, field string) types.String {
	if model == nil {
		return types.StringValue("__import__")
	}
	if field == "name" {
		return model.Name
	}
	return types.StringNull()
}

func flattenWarehouse(project string, warehouse *client.Warehouse, prior *WarehouseResourceModel) WarehouseResourceModel {
	resolvedProject := warehouse.Metadata.Namespace
	if resolvedProject == "" {
		resolvedProject = project
	}

	data := WarehouseResourceModel{
		Project:               types.StringValue(resolvedProject),
		Name:                  types.StringValue(warehouse.Metadata.Name),
		ID:                    types.StringValue(warehouseID(resolvedProject, warehouse.Metadata.Name)),
		Shard:                 warehouseComputedStringValue(warehouse.Spec.Shard, priorWarehouseString(prior, "shard")),
		Interval:              durationValue{StringValue: warehouseComputedStringValue(warehouse.Spec.Interval, priorWarehouseString(prior, "interval"))},
		FreightCreationPolicy: warehouseComputedStringValue(warehouse.Spec.FreightCreationPolicy, priorWarehouseString(prior, "freight_creation_policy")),
		Subscription:          make([]WarehouseSubscriptionModel, 0, len(warehouse.Spec.Subscriptions)),
	}
	if warehouse.Spec.FreightCreationCriteria != nil {
		data.FreightCreationCriteria = warehouseComputedStringValue(warehouse.Spec.FreightCreationCriteria.Expression, priorWarehouseString(prior, "freight_creation_criteria"))
	} else {
		data.FreightCreationCriteria = types.StringNull()
	}

	for i, sub := range warehouse.Spec.Subscriptions {
		var priorSub *WarehouseSubscriptionModel
		if prior != nil && i < len(prior.Subscription) {
			priorSub = &prior.Subscription[i]
		}

		flattened := WarehouseSubscriptionModel{Name: warehouseComputedStringValue(sub.Name, priorSubscriptionString(priorSub, "name"))}
		if sub.Image != nil {
			var priorImage *WarehouseImageSubscriptionModel
			if priorSub != nil {
				priorImage = priorSub.Image
			}
			flattened.Image = &WarehouseImageSubscriptionModel{
				RepoURL:              types.StringValue(sub.Image.RepoURL),
				SemverConstraint:     warehouseComputedStringValue(sub.Image.Constraint, priorImageString(priorImage, "semver_constraint")),
				TagSelectionStrategy: warehouseComputedStringValue(sub.Image.ImageSelectionStrategy, priorImageString(priorImage, "tag_selection_strategy")),
				Platform:             warehouseComputedStringValue(sub.Image.Platform, priorImageString(priorImage, "platform")),
				AllowTags:            warehouseComputedStringValue(sub.Image.AllowTags, types.StringValue("__import__")),
				AllowTagsRegexes:     stringList(sub.Image.AllowTagsRegexes), IgnoreTagsRegexes: stringList(sub.Image.IgnoreTagsRegexes), IgnoreTags: stringList(sub.Image.IgnoreTags),
				CacheByTag: optionalBool(sub.Image.CacheByTag), DiscoveryLimit: optionalInt64(sub.Image.DiscoveryLimit),
				InsecureSkipTLSVerify: optionalBool(sub.Image.InsecureSkipTLSVerify), StrictSemvers: optionalBool(sub.Image.StrictSemvers),
			}
		}
		if sub.Git != nil {
			var priorGit *WarehouseGitSubscriptionModel
			if priorSub != nil {
				priorGit = priorSub.Git
			}
			flattened.Git = &WarehouseGitSubscriptionModel{
				RepoURL:                 types.StringValue(sub.Git.RepoURL),
				Branch:                  warehouseComputedStringValue(sub.Git.Branch, priorGitString(priorGit, "branch")),
				SemverConstraint:        warehouseComputedStringValue(sub.Git.SemverConstraint, priorGitString(priorGit, "semver_constraint")),
				CommitSelectionStrategy: warehouseComputedStringValue(sub.Git.CommitSelectionStrategy, priorGitString(priorGit, "commit_selection_strategy")),
				AllowTags:               warehouseComputedStringValue(sub.Git.AllowTags, types.StringValue("__import__")),
				AllowTagsRegexes:        stringList(sub.Git.AllowTagsRegexes), IgnoreTagsRegexes: stringList(sub.Git.IgnoreTagsRegexes), IgnoreTags: stringList(sub.Git.IgnoreTags), IncludePaths: stringList(sub.Git.IncludePaths), ExcludePaths: stringList(sub.Git.ExcludePaths),
				ExpressionFilter: warehouseComputedStringValue(sub.Git.ExpressionFilter, priorGitString(priorGit, "expression_filter")), Since: warehouseComputedStringValue(sub.Git.Since, priorGitString(priorGit, "since")),
				Blobless: optionalBool(sub.Git.Blobless), DiscoveryLimit: optionalInt64(sub.Git.DiscoveryLimit), InsecureSkipTLSVerify: optionalBool(sub.Git.InsecureSkipTLSVerify), StrictSemvers: optionalBool(sub.Git.StrictSemvers),
			}
		}
		if sub.Chart != nil {
			var priorChart *WarehouseChartSubscriptionModel
			if priorSub != nil {
				priorChart = priorSub.Chart
			}
			flattened.Chart = &WarehouseChartSubscriptionModel{
				RepoURL:          types.StringValue(sub.Chart.RepoURL),
				Name:             warehouseComputedStringValue(sub.Chart.Name, priorChartString(priorChart, "name")),
				SemverConstraint: warehouseComputedStringValue(sub.Chart.SemverConstraint, priorChartString(priorChart, "semver_constraint")),
				DiscoveryLimit:   optionalInt64(sub.Chart.DiscoveryLimit), InsecureSkipTLSVerify: optionalBool(sub.Chart.InsecureSkipTLSVerify),
			}
		}
		if sub.Generic != nil {
			flattened.Generic = &WarehouseGenericSubscriptionModel{Type: types.StringValue(sub.Generic.Type), Config: types.StringValue(string(sub.Generic.Config)), DiscoveryLimit: optionalInt64(sub.Generic.DiscoveryLimit)}
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

func warehouseComputedStringValue(value string, _ types.String) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
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
	case "commit_selection_strategy":
		return model.CommitSelectionStrategy
	case "expression_filter":
		return model.ExpressionFilter
	case "since":
		return model.Since
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
