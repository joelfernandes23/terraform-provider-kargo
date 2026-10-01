package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

type projectErrorClient struct {
	client.KargoClient
	err error
}

func (c projectErrorClient) CreateProject(context.Context, string) (*client.Project, error) {
	return nil, c.err
}
func (c projectErrorClient) GetProject(context.Context, string) (*client.Project, error) {
	return nil, c.err
}
func (c projectErrorClient) DeleteProject(context.Context, string) error { return c.err }

func TestProjectLifecycleFailures(t *testing.T) {
	ctx := context.Background()
	for _, operation := range []string{"create", "read", "delete", "import", "update"} {
		for _, malformed := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "/api", true: "/invalid-state"}[malformed], func(t *testing.T) {
				r := &ProjectResource{client: projectErrorClient{err: errors.New("API unavailable")}}
				var schema resource.SchemaResponse
				r.Schema(ctx, resource.SchemaRequest{}, &schema)
				plan := tfsdk.Plan{Schema: schema.Schema}
				if d := plan.Set(ctx, &ProjectResourceModel{Name: types.StringValue("test"), ID: types.StringValue("test")}); d.HasError() {
					t.Fatal(d)
				}
				if malformed {
					plan.Raw = tftypes.NewValue(tftypes.String, "invalid")
				}
				state := tfsdk.State{Schema: schema.Schema, Raw: plan.Raw}
				var diagnostics diag.Diagnostics
				switch operation {
				case "create":
					resp := resource.CreateResponse{State: state}
					r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
					diagnostics = resp.Diagnostics
				case "read":
					resp := resource.ReadResponse{State: state}
					r.Read(ctx, resource.ReadRequest{State: state}, &resp)
					diagnostics = resp.Diagnostics
				case "delete":
					resp := resource.DeleteResponse{State: state}
					r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
					diagnostics = resp.Diagnostics
				case "import":
					resp := resource.ImportStateResponse{State: state}
					r.ImportState(ctx, resource.ImportStateRequest{ID: "test"}, &resp)
					diagnostics = resp.Diagnostics
				case "update":
					resp := resource.UpdateResponse{State: state}
					r.Update(ctx, resource.UpdateRequest{Plan: plan}, &resp)
					diagnostics = resp.Diagnostics
				}
				if !diagnostics.HasError() {
					t.Fatal("expected actionable error diagnostics")
				}
			})
		}
	}
}

func TestProjectImportMissing(t *testing.T) {
	r := &ProjectResource{client: projectErrorClient{}}
	var resp resource.ImportStateResponse
	r.ImportState(context.Background(), resource.ImportStateRequest{ID: "missing"}, &resp)
	if !resp.Diagnostics.HasError() || resp.Diagnostics[0].Summary() != "Project not found" {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
}
