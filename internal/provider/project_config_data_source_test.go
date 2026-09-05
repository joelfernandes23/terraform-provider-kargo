package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

func TestProjectConfigDataSourceSchema(t *testing.T) {
	d := &ProjectConfigDataSource{}
	resp := &datasource.SchemaResponse{}
	d.Schema(context.Background(), datasource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %s", resp.Diagnostics)
	}
	if !resp.Schema.Attributes["project"].IsRequired() {
		t.Error("project must be required")
	}
	if !resp.Schema.Attributes["promotion_policies"].IsComputed() {
		t.Error("promotion_policies must be computed")
	}
	if !resp.Schema.Attributes["webhook_endpoint"].IsSensitive() {
		t.Error("webhook_endpoint must be sensitive")
	}
	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("invalid schema implementation: %s", diags)
	}
}

func TestNewProjectConfigDataSource(t *testing.T) {
	if _, ok := NewProjectConfigDataSource().(*ProjectConfigDataSource); !ok {
		t.Fatal("expected *ProjectConfigDataSource")
	}
}

func TestProjectConfigDataSourceConfigure(t *testing.T) {
	tests := []struct {
		name      string
		data      any
		wantError bool
	}{
		{name: "nil", data: nil},
		{name: "client", data: &client.Client{}},
		{name: "wrong type", data: "wrong", wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &ProjectConfigDataSource{}
			resp := &datasource.ConfigureResponse{}
			d.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: tc.data}, resp)
			if resp.Diagnostics.HasError() != tc.wantError {
				t.Fatalf("unexpected diagnostics: %s", resp.Diagnostics)
			}
		})
	}
}

func TestProjectConfigDataSourceMetadata(t *testing.T) {
	d := &ProjectConfigDataSource{}
	resp := &datasource.MetadataResponse{}
	d.Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "kargo"}, resp)
	if resp.TypeName != "kargo_project_config" {
		t.Fatalf("unexpected type name %q", resp.TypeName)
	}
}
