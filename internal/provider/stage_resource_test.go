package provider

import (
	"context"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestStageResourceMetadata(t *testing.T) {
	r := &StageResource{}
	resp := &fwresource.MetadataResponse{}
	r.Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "kargo"}, resp)

	if resp.TypeName != "kargo_stage" {
		t.Errorf("expected type name %q, got %q", "kargo_stage", resp.TypeName)
	}
}

func TestStageResourceSchema(t *testing.T) {
	r := &StageResource{}
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %s", resp.Diagnostics)
	}

	projectAttr, ok := resp.Schema.Attributes["project"]
	if !ok {
		t.Fatal("expected 'project' attribute")
	}
	if !projectAttr.IsRequired() {
		t.Error("project should be required")
	}

	nameAttr, ok := resp.Schema.Attributes["name"]
	if !ok {
		t.Fatal("expected 'name' attribute")
	}
	if !nameAttr.IsRequired() {
		t.Error("name should be required")
	}

	idAttr, ok := resp.Schema.Attributes["id"]
	if !ok {
		t.Fatal("expected 'id' attribute")
	}
	if !idAttr.IsComputed() {
		t.Error("id should be computed")
	}

	shardAttr, ok := resp.Schema.Attributes["shard"]
	if !ok {
		t.Fatal("expected 'shard' attribute")
	}
	if !shardAttr.IsOptional() {
		t.Error("shard should be optional")
	}

	if _, ok := resp.Schema.Blocks["requested_freight"]; !ok {
		t.Fatal("expected 'requested_freight' block")
	}
	if _, ok := resp.Schema.Blocks["promotion_template"]; !ok {
		t.Fatal("expected 'promotion_template' block")
	}
}

func TestStageResourceSchemaValid(t *testing.T) {
	r := &StageResource{}
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	diags := resp.Schema.ValidateImplementation(context.Background())
	if diags.HasError() {
		t.Fatalf("schema implementation invalid: %s", diags)
	}
}

func TestStageResourceConfigureWrongType(t *testing.T) {
	r := &StageResource{}
	resp := &fwresource.ConfigureResponse{}
	r.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: "wrong"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

func TestStageResourceConfigureNil(t *testing.T) {
	r := &StageResource{}
	resp := &fwresource.ConfigureResponse{}
	r.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: nil}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error: %s", resp.Diagnostics)
	}
}

func TestNewStageResource(t *testing.T) {
	r := NewStageResource()
	if r == nil {
		t.Fatal("expected non-nil resource")
	}
	if _, ok := r.(*StageResource); !ok {
		t.Error("expected *StageResource")
	}
}
