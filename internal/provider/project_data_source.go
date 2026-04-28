package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

var (
	_ datasource.DataSource              = &ProjectDataSource{}
	_ datasource.DataSourceWithConfigure = &ProjectDataSource{}
)

type ProjectDataSource struct {
	client client.KargoClient
}

type ProjectDataSourceModel struct {
	Name   types.String                  `tfsdk:"name"`
	ID     types.String                  `tfsdk:"id"`
	Status *ProjectDataSourceStatusModel `tfsdk:"status"`
}

type ProjectDataSourceStatusModel struct {
	Phase      types.String                      `tfsdk:"phase"`
	Conditions []ProjectDataSourceConditionModel `tfsdk:"conditions"`
}

type ProjectDataSourceConditionModel struct {
	Type               types.String `tfsdk:"type"`
	Status             types.String `tfsdk:"status"`
	Reason             types.String `tfsdk:"reason"`
	Message            types.String `tfsdk:"message"`
	LastTransitionTime types.String `tfsdk:"last_transition_time"`
}

func NewProjectDataSource() datasource.DataSource {
	return &ProjectDataSource{}
}

func (d *ProjectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (d *ProjectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Kargo project by name.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the Kargo project to look up.",
				Validators:  rfc1123NameValidators(),
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique identifier of the project (same as name).",
			},
			"status": schema.SingleNestedAttribute{
				Computed:    true,
				Description: "The current status of the project.",
				Attributes: map[string]schema.Attribute{
					"phase": schema.StringAttribute{
						Computed:    true,
						Description: "The current phase of the project.",
					},
					"conditions": schema.ListNestedAttribute{
						Computed:    true,
						Description: "The conditions of the project.",
						NestedObject: schema.NestedAttributeObject{
							Attributes: map[string]schema.Attribute{
								"type": schema.StringAttribute{
									Computed:    true,
									Description: "The type of the condition.",
								},
								"status": schema.StringAttribute{
									Computed:    true,
									Description: "The status of the condition.",
								},
								"reason": schema.StringAttribute{
									Computed:    true,
									Description: "The reason for the condition.",
								},
								"message": schema.StringAttribute{
									Computed:    true,
									Description: "A human-readable message for the condition.",
								},
								"last_transition_time": schema.StringAttribute{
									Computed:    true,
									Description: "The last time the condition transitioned.",
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *ProjectDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *ProjectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data ProjectDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	project, err := d.client.GetProject(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to read project",
			fmt.Sprintf("Unable to read project %q: %s", data.Name.ValueString(), err.Error()),
		)
		return
	}

	if project == nil {
		resp.Diagnostics.AddError(
			"Project not found",
			fmt.Sprintf("Project %q does not exist or is being deleted.", data.Name.ValueString()),
		)
		return
	}

	data.ID = types.StringValue(project.Metadata.Name)

	conditions := make([]ProjectDataSourceConditionModel, len(project.Status.Conditions))
	for i, condition := range project.Status.Conditions {
		conditions[i] = ProjectDataSourceConditionModel{
			Type:               types.StringValue(condition.Type),
			Status:             types.StringValue(condition.Status),
			Reason:             types.StringValue(condition.Reason),
			Message:            types.StringValue(condition.Message),
			LastTransitionTime: types.StringValue(condition.LastTransitionTime),
		}
	}

	data.Status = &ProjectDataSourceStatusModel{
		Phase:      types.StringValue(project.Status.Phase),
		Conditions: conditions,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
