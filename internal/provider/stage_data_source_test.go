package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	fwdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

func TestStageDataSourceMetadata(t *testing.T) {
	d := &StageDataSource{}
	resp := &fwdatasource.MetadataResponse{}
	d.Metadata(context.Background(), fwdatasource.MetadataRequest{ProviderTypeName: "kargo"}, resp)

	if resp.TypeName != "kargo_stage" {
		t.Errorf("expected type name %q, got %q", "kargo_stage", resp.TypeName)
	}
}

func TestStageDataSourceSchema(t *testing.T) {
	d := &StageDataSource{}
	resp := &fwdatasource.SchemaResponse{}
	d.Schema(context.Background(), fwdatasource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %s", resp.Diagnostics)
	}

	for _, name := range []string{"project", "name"} {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Fatalf("expected %q attribute", name)
		}
		if !attr.IsRequired() {
			t.Errorf("%s should be required", name)
		}
	}

	for _, name := range []string{"id", "shard", "requested_freight", "promotion_template", "status", "freight"} {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Fatalf("expected %q attribute", name)
		}
		if !attr.IsComputed() {
			t.Errorf("%s should be computed", name)
		}
	}
}

func TestStageDataSourceSchemaValid(t *testing.T) {
	d := &StageDataSource{}
	resp := &fwdatasource.SchemaResponse{}
	d.Schema(context.Background(), fwdatasource.SchemaRequest{}, resp)

	diags := resp.Schema.ValidateImplementation(context.Background())
	if diags.HasError() {
		t.Fatalf("schema implementation invalid: %s", diags)
	}
}

func TestStageDataSourceConfigureWrongType(t *testing.T) {
	d := &StageDataSource{}
	resp := &fwdatasource.ConfigureResponse{}
	d.Configure(context.Background(), fwdatasource.ConfigureRequest{ProviderData: "wrong"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

func TestStageDataSourceConfigureNil(t *testing.T) {
	d := &StageDataSource{}
	resp := &fwdatasource.ConfigureResponse{}
	d.Configure(context.Background(), fwdatasource.ConfigureRequest{ProviderData: nil}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error: %s", resp.Diagnostics)
	}
}

func TestStageDataSourceConfigureCorrectType(t *testing.T) {
	d := &StageDataSource{}
	resp := &fwdatasource.ConfigureResponse{}
	c := &client.Client{}
	d.Configure(context.Background(), fwdatasource.ConfigureRequest{ProviderData: c}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error: %s", resp.Diagnostics)
	}
}

func TestNewStageDataSource(t *testing.T) {
	d := NewStageDataSource()
	if d == nil {
		t.Fatal("expected non-nil data source")
	}
	if _, ok := d.(*StageDataSource); !ok {
		t.Error("expected *StageDataSource")
	}
}

func TestFlattenStageDataSource(t *testing.T) {
	var stage client.Stage
	assertNoError(t, json.Unmarshal([]byte(`{
		"metadata": {"name": "staging", "namespace": "demo"},
		"spec": {
			"shard": "eu-west",
			"requestedFreight": [
				{
					"origin": {"kind": "Warehouse", "name": "app"},
					"sources": {"stages": ["test"]}
				}
			],
			"promotionTemplate": {
				"spec": {
					"steps": [
						{
							"uses": "git-clone",
							"as": "clone",
							"config": {"repoURL": "https://github.com/example/repo.git"}
						}
					]
				}
			}
		},
		"status": {
			"conditions": [
				{"type": "Verified", "status": "True"},
				{"type": "Healthy", "status": "True"},
				{"type": "Ready", "status": "True", "reason": "Healthy", "message": "ready", "lastTransitionTime": "2026-07-02T09:30:00Z"}
			],
			"freightSummary": "1/1 Fulfilled",
			"health": {"status": "Healthy", "issues": ["example issue"]},
			"lastHandledRefresh": "refresh-1",
			"autoPromotionEnabled": true,
			"freightHistory": [
				{
					"id": "abc123def456",
					"items": {
						"Warehouse/app": {
							"name": "freight-1",
							"origin": {"kind": "Warehouse", "name": "app"},
							"commits": [{"repoURL": "https://github.com/example/repo.git", "id": "deadbeef"}],
							"images": [{"repoURL": "ghcr.io/example/app", "tag": "1.2.3", "digest": "sha256:abc"}],
							"charts": [{"repoURL": "https://charts.example.com", "name": "app", "version": "1.2.3"}],
							"artifacts": [{"artifactType": "application/example", "subscriptionName": "bundle", "version": "v1"}]
						}
					}
				}
			],
			"lastPromotion": {
				"name": "staging.01hxyz.abcdef",
				"finishedAt": "2026-07-02T09:30:00Z",
				"status": {"phase": "Succeeded"}
			}
		}
	}`), &stage))

	model := flattenStageDataSource(context.Background(), "demo", &stage)
	assertEqual(t, "demo", model.Project.ValueString())
	assertEqual(t, "staging", model.Name.ValueString())
	assertEqual(t, "demo/staging", model.ID.ValueString())
	assertEqual(t, "eu-west", model.Shard.ValueString())

	if len(model.RequestedFreight) != 1 {
		t.Fatalf("expected 1 requested freight, got %d", len(model.RequestedFreight))
	}
	assertEqual(t, "Warehouse", model.RequestedFreight[0].Origin.Kind.ValueString())
	assertEqual(t, "app", model.RequestedFreight[0].Origin.Name.ValueString())
	if model.RequestedFreight[0].Sources.Direct.ValueBool() {
		t.Error("expected sources.direct to be false")
	}
	var stages []string
	if diags := model.RequestedFreight[0].Sources.Stages.ElementsAs(context.Background(), &stages, false); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %s", diags)
	}
	if len(stages) != 1 || stages[0] != "test" {
		t.Fatalf("expected sources.stages [test], got %#v", stages)
	}

	if model.PromotionTemplate == nil || len(model.PromotionTemplate.Step) != 1 {
		t.Fatalf("expected promotion template with 1 step, got %#v", model.PromotionTemplate)
	}
	assertEqual(t, "git-clone", model.PromotionTemplate.Step[0].Uses.ValueString())
	assertEqual(t, "clone", model.PromotionTemplate.Step[0].As.ValueString())
	if model.PromotionTemplate.Step[0].Config.IsNull() {
		t.Error("expected step config to be set")
	}

	if model.Status == nil {
		t.Fatal("expected status")
	}
	if model.Status.Health == nil {
		t.Fatal("expected health")
	}
	assertEqual(t, "Healthy", model.Status.Health.Status.ValueString())
	var issues []string
	if diags := model.Status.Health.Issues.ElementsAs(context.Background(), &issues, false); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %s", diags)
	}
	if len(issues) != 1 || issues[0] != "example issue" {
		t.Fatalf("expected health issues [example issue], got %#v", issues)
	}

	if len(model.Status.Conditions) != 3 {
		t.Fatalf("expected 3 conditions, got %d", len(model.Status.Conditions))
	}
	assertEqual(t, "Healthy", model.Status.Conditions[0].Type.ValueString())
	assertEqual(t, "Ready", model.Status.Conditions[1].Type.ValueString())
	assertEqual(t, "Verified", model.Status.Conditions[2].Type.ValueString())
	if !model.Status.Conditions[0].Reason.IsNull() {
		t.Fatalf("expected empty condition reason to flatten to null, got %q", model.Status.Conditions[0].Reason.ValueString())
	}
	assertEqual(t, "Healthy", model.Status.Conditions[1].Reason.ValueString())

	assertEqual(t, "1/1 Fulfilled", model.Status.FreightSummary.ValueString())
	if !model.Status.ObservedGeneration.IsNull() {
		t.Fatalf("expected absent observed generation to flatten to null, got %d", model.Status.ObservedGeneration.ValueInt64())
	}
	if !model.Status.AutoPromotionEnabled.ValueBool() {
		t.Error("expected auto_promotion_enabled to be true")
	}
	assertEqual(t, "refresh-1", model.Status.LastHandledRefresh.ValueString())

	if model.Status.LastPromotion == nil {
		t.Fatal("expected last promotion")
	}
	assertEqual(t, "staging.01hxyz.abcdef", model.Status.LastPromotion.Name.ValueString())
	assertEqual(t, "Succeeded", model.Status.LastPromotion.Phase.ValueString())
	assertEqual(t, "2026-07-02T09:30:00Z", model.Status.LastPromotion.FinishedAt.ValueString())

	if len(model.Freight) != 1 {
		t.Fatalf("expected 1 freight item, got %d", len(model.Freight))
	}
	current := model.Freight[0]
	assertEqual(t, "freight-1", current.Name.ValueString())
	assertEqual(t, "Warehouse", current.OriginKind.ValueString())
	assertEqual(t, "app", current.OriginName.ValueString())
	assertEqual(t, "deadbeef", current.Commits[0].ID.ValueString())
	assertEqual(t, "1.2.3", current.Images[0].Tag.ValueString())
	assertEqual(t, "1.2.3", current.Charts[0].Version.ValueString())
	assertEqual(t, "v1", current.Artifacts[0].Version.ValueString())
}

func TestFlattenStageDataSourceSparse(t *testing.T) {
	stage := &client.Stage{Metadata: client.StageMetadata{Name: "fresh", Namespace: "demo"}}

	model := flattenStageDataSource(context.Background(), "demo", stage)
	assertEqual(t, "demo", model.Project.ValueString())
	assertEqual(t, "demo/fresh", model.ID.ValueString())

	if model.Status == nil {
		t.Fatal("expected non-nil status for zero-value stage status")
	}
	if model.Status.Health != nil {
		t.Fatalf("expected nil health, got %#v", model.Status.Health)
	}
	if model.Status.LastPromotion != nil {
		t.Fatalf("expected nil last promotion, got %#v", model.Status.LastPromotion)
	}
	if !model.Status.ObservedGeneration.IsNull() {
		t.Fatalf("expected null observed generation, got %d", model.Status.ObservedGeneration.ValueInt64())
	}
	if model.Status.AutoPromotionEnabled.ValueBool() {
		t.Error("expected auto_promotion_enabled to be false")
	}
	if !model.Status.FreightSummary.IsNull() {
		t.Fatalf("expected null freight summary, got %q", model.Status.FreightSummary.ValueString())
	}
	if len(model.Freight) != 0 {
		t.Fatalf("expected empty freight, got %d items", len(model.Freight))
	}
	if model.PromotionTemplate != nil {
		t.Fatalf("expected nil promotion template, got %#v", model.PromotionTemplate)
	}
	if !model.Shard.IsNull() {
		t.Fatalf("expected null shard, got %q", model.Shard.ValueString())
	}
}

func TestFlattenStageDataSourceFreightSortsOrigins(t *testing.T) {
	history := []client.StageFreightCollection{{
		ID: "abc123def456",
		Items: map[string]client.StageFreightReference{
			"Warehouse/zeta": {Name: "freight-zeta", Origin: client.FreightOrigin{Name: "zeta"}},
			"Warehouse/app":  {Name: "freight-app", Origin: client.FreightOrigin{Name: "app"}},
			"Warehouse/mid":  {Name: "freight-mid", Origin: client.FreightOrigin{Name: "mid"}},
		},
	}}

	models := flattenStageDataSourceFreight(history)
	if len(models) != 3 {
		t.Fatalf("expected 3 freight items, got %d", len(models))
	}
	for i, expected := range []string{"app", "mid", "zeta"} {
		if got := models[i].OriginName.ValueString(); got != expected {
			t.Errorf("expected freight[%d] origin name %q, got %q", i, expected, got)
		}
	}

	if got := flattenStageDataSourceFreight(nil); len(got) != 0 {
		t.Fatalf("expected no freight for nil history, got %d", len(got))
	}
	if got := flattenStageDataSourceFreight([]client.StageFreightCollection{}); len(got) != 0 {
		t.Fatalf("expected no freight for empty history, got %d", len(got))
	}
}

func TestFlattenStageDataSourceUsesNamespaceWhenPresent(t *testing.T) {
	stage := &client.Stage{Metadata: client.StageMetadata{Name: "app", Namespace: "real-project"}}

	model := flattenStageDataSource(context.Background(), "fallback", stage)
	assertEqual(t, "real-project", model.Project.ValueString())
	assertEqual(t, "real-project/app", model.ID.ValueString())
}

type stageDataSourceServerOptions struct {
	sparse      bool
	notFound    bool
	terminating bool
}

// testStageDataSourceServer emulates the wire contract observed against a real
// Kargo v1.10.8 (phase 10 live check): GetStage must be requested with format
// RAW_FORMAT_JSON and responds {"raw": base64(standard k8s JSON manifest)}
// including the full status block. The handler fails the test on any
// structured (non-raw) read because that path drops current-freight content
// entirely — the suite must never green-light it.
func testStageDataSourceServer(t *testing.T, opts stageDataSourceServerOptions) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case endsWith(r.URL.Path, "/AdminLogin"):
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{"idToken": "test-jwt"})) //nolint:gosec // test
		case endsWith(r.URL.Path, "/GetStage"):
			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			if body["format"] != "RAW_FORMAT_JSON" {
				t.Errorf("GetStage must request RAW_FORMAT_JSON, got %q", body["format"])
			}
			if opts.notFound {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"code":"not_found","message":"stage %q not found"}`, body["name"])
				return
			}
			manifestJSON, err := json.Marshal(testStageDataSourceManifest(opts))
			assertNoError(t, err)
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{
				"raw": base64.StdEncoding.EncodeToString(manifestJSON),
			}))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func testStageDataSourceManifest(opts stageDataSourceServerOptions) map[string]any {
	metadata := map[string]any{
		"name":            "staging",
		"namespace":       "demo",
		"uid":             "uid-staging",
		"resourceVersion": "1",
	}
	if opts.terminating {
		metadata["deletionTimestamp"] = "2026-07-05T00:00:00Z"
	}

	manifest := map[string]any{
		"apiVersion": "kargo.akuity.io/v1alpha1",
		"kind":       "Stage",
		"metadata":   metadata,
		"spec": map[string]any{
			"shard": "eu-west",
			"requestedFreight": []map[string]any{{
				"origin":  map[string]string{"kind": "Warehouse", "name": "app"},
				"sources": map[string]any{"stages": []string{"test"}},
			}},
			"promotionTemplate": map[string]any{
				"spec": map[string]any{
					"steps": []map[string]any{{
						"uses":   "git-clone",
						"as":     "clone",
						"config": map[string]string{"repoURL": "https://github.com/example/repo.git"},
					}},
				},
			},
		},
	}

	if opts.sparse {
		// A fresh never-promoted stage carries only conditions + freightSummary
		// (observed live in the phase 09 wire check).
		manifest["status"] = map[string]any{
			"conditions": []map[string]string{{
				"type":               "Ready",
				"status":             "False",
				"reason":             "NoFreight",
				"lastTransitionTime": "2026-07-05T13:23:57Z",
			}},
			"freightSummary": "0/1 Fulfilled",
		}
		return manifest
	}

	// Populated status matching the live-observed Kargo v1.10.8 shape, with TWO
	// origins so sorted multi-origin freight output is exercised end-to-end.
	manifest["status"] = map[string]any{
		"autoPromotionEnabled": true,
		"conditions": []map[string]string{
			{"type": "Verified", "status": "True", "reason": "Verified", "lastTransitionTime": "2026-07-02T09:31:00Z"},
			{"type": "Healthy", "status": "True", "reason": "Healthy", "lastTransitionTime": "2026-07-02T09:30:00Z"},
		},
		"freightHistory": []map[string]any{{
			"id": "abc123def456",
			"items": map[string]any{
				"Warehouse/zeta": map[string]any{
					"name":   "freight-2",
					"origin": map[string]string{"kind": "Warehouse", "name": "zeta"},
					"images": []map[string]string{{"repoURL": "ghcr.io/example/zeta", "tag": "2.0.0", "digest": "sha256:zzz"}},
				},
				"Warehouse/app": map[string]any{
					"name":   "freight-1",
					"origin": map[string]string{"kind": "Warehouse", "name": "app"},
					"commits": []map[string]string{{
						"repoURL": "https://github.com/example/repo.git",
						"id":      "deadbeef",
						"branch":  "main",
					}},
					"images": []map[string]string{{"repoURL": "ghcr.io/example/app", "tag": "1.2.3", "digest": "sha256:abc"}},
					"charts": []map[string]string{{"repoURL": "https://charts.example.com", "name": "app", "version": "1.2.3"}},
					"artifacts": []map[string]string{{
						"artifactType":     "application/example",
						"subscriptionName": "bundle",
						"version":          "v1",
					}},
				},
			},
		}},
		"freightSummary":     "2/2 Fulfilled",
		"health":             map[string]any{"status": "Healthy", "issues": []string{"example issue"}},
		"lastHandledRefresh": "refresh-1",
		"lastPromotion": map[string]any{
			"name":       "staging.01hxyz.abcdef",
			"finishedAt": "2026-07-02T09:30:00Z",
			"status":     map[string]string{"phase": "Succeeded"},
		},
	}
	return manifest
}

func TestAccStageDataSource_basic(t *testing.T) {
	srv := testStageDataSourceServer(t, stageDataSourceServerOptions{})
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testStageDataSourceConfig(srv.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.kargo_stage.test", "project", "demo"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "name", "staging"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "id", "demo/staging"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "shard", "eu-west"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "requested_freight.#", "1"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "requested_freight.0.origin.name", "app"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "requested_freight.0.sources.stages.0", "test"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "promotion_template.step.#", "1"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "promotion_template.step.0.uses", "git-clone"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "promotion_template.step.0.as", "clone"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.health.status", "Healthy"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.health.issues.0", "example issue"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.conditions.#", "2"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.conditions.0.type", "Healthy"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.conditions.1.type", "Verified"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.freight_summary", "2/2 Fulfilled"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.auto_promotion_enabled", "true"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.last_handled_refresh", "refresh-1"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.last_promotion.name", "staging.01hxyz.abcdef"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.last_promotion.phase", "Succeeded"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.last_promotion.finished_at", "2026-07-02T09:30:00Z"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "freight.#", "2"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "freight.0.origin_name", "app"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "freight.0.name", "freight-1"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "freight.0.commits.0.id", "deadbeef"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "freight.0.images.0.tag", "1.2.3"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "freight.0.charts.0.version", "1.2.3"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "freight.0.artifacts.0.version", "v1"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "freight.1.origin_name", "zeta"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "freight.1.images.0.tag", "2.0.0"),
					resource.TestCheckNoResourceAttr("data.kargo_stage.test", "status.observed_generation"),
				),
			},
		},
	})
}

func TestAccStageDataSource_sparseStatus(t *testing.T) {
	srv := testStageDataSourceServer(t, stageDataSourceServerOptions{sparse: true})
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testStageDataSourceConfig(srv.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.kargo_stage.test", "freight.#", "0"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.freight_summary", "0/1 Fulfilled"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.conditions.0.reason", "NoFreight"),
					resource.TestCheckResourceAttr("data.kargo_stage.test", "status.auto_promotion_enabled", "false"),
					resource.TestCheckNoResourceAttr("data.kargo_stage.test", "status.health.status"),
					resource.TestCheckNoResourceAttr("data.kargo_stage.test", "status.last_promotion.name"),
				),
			},
		},
	})
}

func TestAccStageDataSource_notFound(t *testing.T) {
	srv := testStageDataSourceServer(t, stageDataSourceServerOptions{notFound: true})
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      testStageDataSourceConfig(srv.URL),
				ExpectError: regexp.MustCompile(`(?i)(stage not found|not_found)`),
			},
		},
	})
}

func TestAccStageDataSource_terminating(t *testing.T) {
	srv := testStageDataSourceServer(t, stageDataSourceServerOptions{terminating: true})
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      testStageDataSourceConfig(srv.URL),
				ExpectError: regexp.MustCompile(`(?i)stage not found`),
			},
		},
	})
}

func testStageDataSourceConfig(url string) string {
	return fmt.Sprintf(`
provider "kargo" {
  api_url        = %q
  admin_password = "admin"
}

data "kargo_stage" "test" {
  project = "demo"
  name    = "staging"
}
`, url)
}
