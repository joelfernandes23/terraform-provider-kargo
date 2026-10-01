package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

type warehouseReadErrorClient struct {
	client.KargoClient
	warehouse          *client.Warehouse
	getErr, freightErr error
}

func (c warehouseReadErrorClient) GetWarehouse(context.Context, string, string) (*client.Warehouse, error) {
	return c.warehouse, c.getErr
}
func (c warehouseReadErrorClient) ListWarehouseFreight(context.Context, string, string) ([]client.Freight, error) {
	return nil, c.freightErr
}

func TestWarehouseDataSourceReadFailures(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, summary string
		client        warehouseReadErrorClient
	}{
		{"API failure", "Failed to read warehouse", warehouseReadErrorClient{getErr: errors.New("API unavailable")}},
		{"missing", "Warehouse not found", warehouseReadErrorClient{getErr: &client.APIError{Code: "not_found"}}},
		{"deleting", "Warehouse not found", warehouseReadErrorClient{}},
		{"freight failure", "Failed to read warehouse freight", warehouseReadErrorClient{warehouse: &client.Warehouse{}, freightErr: errors.New("freight unavailable")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &WarehouseDataSource{client: tc.client}
			var schema datasource.SchemaResponse
			d.Schema(ctx, datasource.SchemaRequest{}, &schema)
			plan := tfsdk.Plan{Schema: schema.Schema}
			if diagnostics := plan.Set(ctx, &WarehouseDataSourceModel{Project: types.StringValue("test"), Name: types.StringValue("source")}); diagnostics.HasError() {
				t.Fatal(diagnostics)
			}
			resp := datasource.ReadResponse{State: tfsdk.State{Schema: schema.Schema}}
			d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schema.Schema, Raw: plan.Raw}}, &resp)
			if !resp.Diagnostics.HasError() || resp.Diagnostics[0].Summary() != tc.summary {
				t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
			}
		})
	}
	t.Run("malformed configuration", func(t *testing.T) {
		d := &WarehouseDataSource{}
		var schema datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schema)
		resp := datasource.ReadResponse{State: tfsdk.State{Schema: schema.Schema}}
		d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: schema.Schema, Raw: tftypes.NewValue(tftypes.String, "invalid")}}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("invalid config must fail before API access")
		}
	})
}
