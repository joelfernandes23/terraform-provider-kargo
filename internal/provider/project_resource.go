package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

var (
	_ resource.Resource                = &ProjectResource{}
	_ resource.ResourceWithImportState = &ProjectResource{}
)

type ProjectResource struct {
	client client.KargoClient
}

type ProjectResourceModel struct {
	Name types.String `tfsdk:"name"`
	ID   types.String `tfsdk:"id"`
}

func NewProjectResource() resource.Resource {
	return &ProjectResource{}
}

func (r *ProjectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *ProjectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Kargo project. A project is a namespace-scoped grouping that creates a dedicated Kubernetes namespace.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the Kargo project.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The unique identifier of the project (same as name).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *ProjectResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	project, err := r.client.CreateProject(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to create project", err.Error())
		return
	}

	data.ID = types.StringValue(project.Metadata.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	project, err := r.client.GetProject(ctx, data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to read project", err.Error())
		return
	}

	// deleted out-of-band
	if project == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	data.Name = types.StringValue(project.Metadata.Name)
	data.ID = types.StringValue(project.Metadata.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ProjectResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	// name is the only field and it requires replace — update is never called
	resp.Diagnostics.AddError("Update not supported", "Project name is immutable; changes require replacement.")
}

func (r *ProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteProject(ctx, data.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to delete project", err.Error())
	}
}

func (r *ProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	project, err := r.client.GetProject(ctx, req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Failed to import project", err.Error())
		return
	}
	if project == nil {
		resp.Diagnostics.AddError("Project not found", fmt.Sprintf("Project %q does not exist.", req.ID))
		return
	}

	var data ProjectResourceModel
	data.Name = types.StringValue(project.Metadata.Name)
	data.ID = types.StringValue(project.Metadata.Name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
