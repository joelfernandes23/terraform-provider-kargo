package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

type warehouseErrorClient struct {
	client.KargoClient
	err error
}

func (c warehouseErrorClient) CreateWarehouse(context.Context, string, string, client.WarehouseSpec) (*client.Warehouse, error) {
	return nil, c.err
}
func (c warehouseErrorClient) UpdateWarehouse(context.Context, string, string, client.WarehouseSpec) (*client.Warehouse, error) {
	return nil, c.err
}
func (c warehouseErrorClient) GetWarehouse(context.Context, string, string) (*client.Warehouse, error) {
	return nil, c.err
}
func (c warehouseErrorClient) DeleteWarehouse(context.Context, string, string) error { return c.err }

func TestWarehouseLifecycleErrors(t *testing.T) {
	ctx := context.Background()
	for _, operation := range []string{"create", "read", "update", "delete", "import"} {
		for _, failure := range []string{"api", "not-found", "empty", "invalid-subscription", "invalid-import"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				var apiErr error
				switch failure {
				case "api":
					apiErr = errors.New("request failed")
				case "not-found":
					apiErr = &client.APIError{Code: "not_found"}
				}
				r := &WarehouseResource{client: warehouseErrorClient{err: apiErr}}
				var schemaResponse resource.SchemaResponse
				r.Schema(ctx, resource.SchemaRequest{}, &schemaResponse)
				data := flattenWarehouse("test", &client.Warehouse{Metadata: client.WarehouseMetadata{Name: "source"}, Spec: client.WarehouseSpec{Subscriptions: []client.WarehouseSubscription{{Git: &client.GitSubscription{RepoURL: "https://example.com/repo"}}}}}, nil)
				if failure == "invalid-subscription" {
					data.Subscription = nil
				}
				plan := tfsdk.Plan{Schema: schemaResponse.Schema}
				if d := plan.Set(ctx, &data); d.HasError() {
					t.Fatal(d)
				}
				state := tfsdk.State{Schema: schemaResponse.Schema, Raw: plan.Raw}
				var diagnostics diag.Diagnostics
				wantError := false
				switch operation {
				case "create":
					resp := resource.CreateResponse{State: state}
					r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
					diagnostics, wantError = resp.Diagnostics, true
				case "update":
					resp := resource.UpdateResponse{State: state}
					r.Update(ctx, resource.UpdateRequest{Plan: plan}, &resp)
					diagnostics, wantError = resp.Diagnostics, true
				case "read":
					resp := resource.ReadResponse{State: state}
					r.Read(ctx, resource.ReadRequest{State: state}, &resp)
					diagnostics, wantError = resp.Diagnostics, failure == "api"
					if !wantError && !resp.State.Raw.IsNull() {
						t.Error("missing Warehouse must be removed from state")
					}
				case "delete":
					resp := resource.DeleteResponse{State: state}
					r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
					diagnostics, wantError = resp.Diagnostics, failure == "api"
				case "import":
					id := "test/source"
					if failure == "invalid-import" {
						id = "invalid"
					}
					resp := resource.ImportStateResponse{State: state}
					r.ImportState(ctx, resource.ImportStateRequest{ID: id}, &resp)
					diagnostics, wantError = resp.Diagnostics, true
				}
				if diagnostics.HasError() != wantError {
					t.Fatalf("diagnostics=%v, want error=%t", diagnostics, wantError)
				}
			})
		}
	}
}
