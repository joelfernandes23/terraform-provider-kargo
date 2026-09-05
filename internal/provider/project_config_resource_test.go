package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

func TestProjectConfigResourceSchema(t *testing.T) {
	r := &ProjectConfigResource{}
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", resp.Diagnostics)
	}
	if !resp.Schema.Attributes["project"].IsRequired() {
		t.Error("project must be required")
	}
	if !resp.Schema.Attributes["webhook_endpoint"].IsSensitive() {
		t.Error("webhook_endpoint must be sensitive")
	}
	if _, ok := resp.Schema.Blocks["promotion_policy"]; !ok {
		t.Error("promotion_policy block missing")
	}
	if _, ok := resp.Schema.Blocks["webhook_receiver"]; !ok {
		t.Error("webhook_receiver block missing")
	}
	if diags := resp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatalf("invalid schema implementation: %s", diags)
	}
}

func TestExpandProjectConfig(t *testing.T) {
	ctx := context.Background()
	labels, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"team": "platform"})
	values, _ := types.ListValueFrom(ctx, types.StringType, []string{"backend"})
	model := ProjectConfigResourceModel{
		Project: types.StringValue("demo"),
		PromotionPolicies: []ProjectConfigPolicyModel{{
			StageSelector: &ProjectConfigSelectorModel{
				Name:        types.StringValue("glob:dev-*"),
				MatchLabels: labels,
				MatchExpressions: []ProjectConfigMatchExpressionModel{{
					Key: types.StringValue("tier"), Operator: types.StringValue("In"), Values: values,
				}},
			},
			AutoPromotionEnabled: types.BoolValue(true),
		}},
		WebhookReceivers: []ProjectConfigWebhookModel{{
			Name: types.StringValue("github"), Type: types.StringValue("github"), SecretName: types.StringValue("hook-secret"),
			VirtualRepoName: types.StringNull(), GenericActions: jsontypes.NewNormalizedNull(),
		}},
	}
	spec, err := expandProjectConfig(ctx, &model)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.PromotionPolicies[0].AutoPromotionEnabled || spec.PromotionPolicies[0].StageSelector.MatchLabels["team"] != "platform" {
		t.Fatalf("unexpected policy: %#v", spec.PromotionPolicies[0])
	}
	if spec.WebhookReceivers[0].GitHub.SecretRef.Name != "hook-secret" {
		t.Fatalf("unexpected receiver: %#v", spec.WebhookReceivers[0])
	}
}

func TestExpandProjectConfigValidation(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name  string
		model ProjectConfigResourceModel
	}{
		{name: "missing selector", model: ProjectConfigResourceModel{PromotionPolicies: []ProjectConfigPolicyModel{{}}}},
		{name: "empty selector", model: ProjectConfigResourceModel{PromotionPolicies: []ProjectConfigPolicyModel{{StageSelector: &ProjectConfigSelectorModel{Name: types.StringNull(), MatchLabels: types.MapNull(types.StringType)}}}}},
		{name: "invalid regex", model: ProjectConfigResourceModel{PromotionPolicies: []ProjectConfigPolicyModel{{StageSelector: &ProjectConfigSelectorModel{Name: types.StringValue("regex:[")}}}}},
		{name: "duplicate receiver", model: ProjectConfigResourceModel{WebhookReceivers: []ProjectConfigWebhookModel{
			{Name: types.StringValue("same"), Type: types.StringValue("github"), SecretName: types.StringValue("secret"), GenericActions: jsontypes.NewNormalizedNull()},
			{Name: types.StringValue("same"), Type: types.StringValue("gitlab"), SecretName: types.StringValue("secret"), GenericActions: jsontypes.NewNormalizedNull()},
		}}},
		{name: "generic missing actions", model: ProjectConfigResourceModel{WebhookReceivers: []ProjectConfigWebhookModel{{Name: types.StringValue("generic"), Type: types.StringValue("generic"), SecretName: types.StringValue("secret"), GenericActions: jsontypes.NewNormalizedNull()}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := expandProjectConfig(ctx, &tc.model); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestGenericWebhookValidation(t *testing.T) {
	valid := ProjectConfigWebhookModel{
		Name: types.StringValue("generic"), Type: types.StringValue("generic"), SecretName: types.StringValue("secret"),
		GenericActions: jsontypes.NewNormalizedValue(`[{"action":"Refresh","targetSelectionCriteria":[{"kind":"Warehouse","name":"app"}]}]`),
	}
	got, err := expandWebhookReceiver(valid)
	if err != nil {
		t.Fatal(err)
	}
	if got.Generic == nil || got.Generic.Actions[0].TargetSelectionCriteria[0].Name != "app" {
		t.Fatalf("unexpected generic receiver: %#v", got)
	}

	invalid := valid
	invalid.GenericActions = jsontypes.NewNormalizedValue(`[{"action":"Refresh","targetSelectionCriteria":[{"kind":"Promotion","name":"run"}]}]`)
	if _, err := expandWebhookReceiver(invalid); err == nil {
		t.Fatal("expected v1.11 Promotion target to be rejected")
	}
}

func TestWebhookReceiverTypesRoundTrip(t *testing.T) {
	typesToTest := []string{"azure", "bitbucket", "dockerhub", "gitea", "github", "gitlab", "harbor", "quay"}
	for _, typ := range typesToTest {
		t.Run(typ, func(t *testing.T) {
			model := ProjectConfigWebhookModel{
				Name: types.StringValue(typ), Type: types.StringValue(typ), SecretName: types.StringValue("secret"),
				VirtualRepoName: types.StringNull(), GenericActions: jsontypes.NewNormalizedNull(),
			}
			expanded, err := expandWebhookReceiver(model)
			if err != nil {
				t.Fatal(err)
			}
			flattened := flattenWebhookReceiver(expanded)
			if flattened.Type.ValueString() != typ || flattened.SecretName.ValueString() != "secret" {
				t.Fatalf("unexpected round trip: %#v", flattened)
			}
		})
	}

	artifactory := ProjectConfigWebhookModel{
		Name: types.StringValue("artifactory"), Type: types.StringValue("artifactory"), SecretName: types.StringValue("secret"),
		VirtualRepoName: types.StringValue("virtual"), GenericActions: jsontypes.NewNormalizedNull(),
	}
	expanded, err := expandWebhookReceiver(artifactory)
	if err != nil {
		t.Fatal(err)
	}
	if flattenWebhookReceiver(expanded).VirtualRepoName.ValueString() != "virtual" {
		t.Fatal("Artifactory virtual repository was not preserved")
	}
}

func TestWebhookReceiverFieldValidation(t *testing.T) {
	base := ProjectConfigWebhookModel{
		Name: types.StringValue("github"), Type: types.StringValue("github"), SecretName: types.StringValue("secret"),
		VirtualRepoName: types.StringNull(), GenericActions: jsontypes.NewNormalizedNull(),
	}

	withVirtual := base
	withVirtual.VirtualRepoName = types.StringValue("invalid")
	if _, err := expandWebhookReceiver(withVirtual); err == nil {
		t.Fatal("expected non-Artifactory virtual_repo_name error")
	}

	withActions := base
	withActions.GenericActions = jsontypes.NewNormalizedValue(`[]`)
	if _, err := expandWebhookReceiver(withActions); err == nil {
		t.Fatal("expected non-generic generic_actions error")
	}

	unsupported := base
	unsupported.Type = types.StringValue("unsupported")
	if _, err := expandWebhookReceiver(unsupported); err == nil {
		t.Fatal("expected unsupported type error")
	}
}

func TestFlattenProjectConfig(t *testing.T) {
	config := &client.ProjectConfig{
		Metadata: client.ProjectConfigMetadata{Name: "demo", Namespace: "demo"},
		Spec: client.ProjectConfigSpec{
			PromotionPolicies: []client.PromotionPolicy{{StageSelector: client.PromotionPolicySelector{Name: "dev"}, AutoPromotionEnabled: true}},
			WebhookReceivers:  []client.WebhookReceiverConfig{{Name: "github", GitHub: &client.WebhookSecretRefConfig{SecretRef: client.LocalObjectReference{Name: "secret"}}}},
		},
		Status: client.ProjectConfigStatus{WebhookReceivers: []client.WebhookReceiverDetails{{Name: "github", Path: "/hook", URL: "https://example.test/hook"}}},
	}
	data := flattenProjectConfig(context.Background(), "demo", config, nil)
	if data.ID.ValueString() != "demo" || data.WebhookReceivers[0].Type.ValueString() != "github" {
		t.Fatalf("unexpected flattened data: %#v", data)
	}
	var endpoints []ProjectConfigEndpointModel
	if diags := data.WebhookEndpoints.ElementsAs(context.Background(), &endpoints, false); diags.HasError() {
		t.Fatalf("decoding endpoints: %s", diags)
	}
	if endpoints[0].URL.ValueString() != "https://example.test/hook" {
		t.Fatal("webhook status was not flattened")
	}
}

func testProjectConfigServer(t *testing.T) *httptest.Server {
	t.Helper()
	var stored map[string]any
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case endsWith(r.URL.Path, "/AdminLogin"):
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{"idToken": "test-jwt"}))
		case endsWith(r.URL.Path, "/CreateResource"), endsWith(r.URL.Path, "/UpdateResource"):
			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			raw, err := base64.StdEncoding.DecodeString(body["manifest"])
			assertNoError(t, err)
			assertNoError(t, json.Unmarshal(raw, &stored))
			field := "createdResourceManifest"
			if endsWith(r.URL.Path, "/UpdateResource") {
				field = "updatedResourceManifest"
			}
			assertNoError(t, json.NewEncoder(w).Encode(map[string]any{"results": []map[string]string{{field: "dGVzdA=="}}}))
		case endsWith(r.URL.Path, "/GetProjectConfig"):
			if stored == nil {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"code":"not_found","message":"missing"}`))
				return
			}
			stored["status"] = map[string]any{"webhookReceivers": []map[string]string{{
				"name": "github", "path": "/hook", "url": "https://example.test/hook",
			}}}
			raw, err := json.Marshal(stored)
			assertNoError(t, err)
			var decoded client.ProjectConfig
			assertNoError(t, json.Unmarshal(raw, &decoded))
			if len(decoded.Spec.PromotionPolicies) == 0 || decoded.Spec.PromotionPolicies[0].StageSelector.Name == "" {
				t.Fatalf("server stored invalid promotion policy: %s", raw)
			}
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{"raw": base64.StdEncoding.EncodeToString(raw)}))
		case endsWith(r.URL.Path, "/DeleteProjectConfig"):
			stored = nil
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAccProjectConfigResource_basic(t *testing.T) {
	srv := testProjectConfigServer(t)
	defer srv.Close()
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testProjectConfigResourceConfig(srv.URL, "dev", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_project_config.test", "id", "demo"),
					resource.TestCheckResourceAttr("kargo_project_config.test", "promotion_policy.0.stage_selector.name", "dev"),
					resource.TestCheckResourceAttr("kargo_project_config.test", "promotion_policy.0.auto_promotion_enabled", "true"),
					resource.TestCheckResourceAttr("kargo_project_config.test", "webhook_receiver.0.type", "github"),
					resource.TestCheckResourceAttr("kargo_project_config.test", "webhook_endpoint.0.name", "github"),
					resource.TestCheckResourceAttr("data.kargo_project_config.test", "id", "demo"),
					resource.TestCheckResourceAttr("data.kargo_project_config.test", "webhook_endpoint.0.name", "github"),
				),
			},
			{
				Config: testProjectConfigResourceConfig(srv.URL, "glob:dev-*", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_project_config.test", "promotion_policy.0.stage_selector.name", "glob:dev-*"),
					resource.TestCheckResourceAttr("kargo_project_config.test", "promotion_policy.0.auto_promotion_enabled", "false"),
				),
			},
			{ResourceName: "kargo_project_config.test", ImportState: true, ImportStateVerify: true},
		},
	})
}

func TestAccProjectConfigResource_live(t *testing.T) {
	if os.Getenv("KARGO_LIVE_TEST") == "" {
		t.Skip("set KARGO_LIVE_TEST=1 to run against a real Kargo API")
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testProjectConfigLiveConfig("dev", true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_project.live", "id", "tf-project-config-live"),
					resource.TestCheckResourceAttr("kargo_project_config.live", "id", "tf-project-config-live"),
					resource.TestCheckResourceAttr("kargo_project_config.live", "promotion_policy.#", "2"),
					resource.TestCheckResourceAttr("kargo_project_config.live", "promotion_policy.0.stage_selector.name", "dev"),
					resource.TestCheckResourceAttr("kargo_project_config.live", "promotion_policy.0.auto_promotion_enabled", "true"),
					resource.TestCheckResourceAttr("kargo_project_config.live", "promotion_policy.1.stage_selector.match_labels.team", "platform"),
					resource.TestCheckResourceAttr("kargo_project_config.live", "webhook_receiver.#", "3"),
					resource.TestCheckResourceAttr("kargo_project_config.live", "webhook_receiver.0.type", "github"),
					resource.TestCheckResourceAttr("kargo_project_config.live", "webhook_receiver.1.virtual_repo_name", "libs"),
					resource.TestCheckResourceAttr("kargo_project_config.live", "webhook_receiver.2.type", "generic"),
					resource.TestCheckResourceAttr("data.kargo_project_config.live", "id", "tf-project-config-live"),
				),
			},
			{
				Config: testProjectConfigLiveConfig("glob:dev-*", false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_project_config.live", "promotion_policy.0.stage_selector.name", "glob:dev-*"),
					resource.TestCheckResourceAttr("kargo_project_config.live", "promotion_policy.0.auto_promotion_enabled", "false"),
				),
			},
			{ResourceName: "kargo_project_config.live", ImportState: true, ImportStateVerify: true},
		},
	})
}

func testProjectConfigLiveConfig(selector string, enabled bool) string {
	return fmt.Sprintf(`
provider "kargo" {}

resource "kargo_project" "live" {
  name = "tf-project-config-live"
}

resource "kargo_project_config" "live" {
  project = kargo_project.live.name

  promotion_policy {
    stage_selector {
      name = %q
    }
    auto_promotion_enabled = %t
  }

  promotion_policy {
    stage_selector {
      match_labels = {
        team = "platform"
      }
      match_expression {
        key      = "tier"
        operator = "In"
        values   = ["backend", "worker"]
	  }
    }
    auto_promotion_enabled = false
  }

  webhook_receiver {
    name        = "github"
    type        = "github"
    secret_name = "github-webhook-secret"
  }

  webhook_receiver {
    name              = "artifactory"
    type              = "artifactory"
    secret_name       = "artifactory-webhook-secret"
    virtual_repo_name = "libs"
  }

  webhook_receiver {
    name            = "generic"
    type            = "generic"
    secret_name     = "generic-webhook-secret"
    generic_actions = jsonencode([{
      action = "Refresh"
      targetSelectionCriteria = [{
        kind = "Warehouse"
        name = "app"
      }]
    }])
  }
}

data "kargo_project_config" "live" {
  project    = kargo_project_config.live.project
  depends_on = [kargo_project_config.live]
}
`, selector, enabled)
}

func testProjectConfigResourceConfig(url, selector string, enabled bool) string {
	return fmt.Sprintf(`
provider "kargo" {
  api_url        = %q
  admin_password = "admin"
}

resource "kargo_project_config" "test" {
  project = "demo"

  promotion_policy {
    stage_selector {
      name = %q
    }
    auto_promotion_enabled = %t
  }

  webhook_receiver {
    name        = "github"
    type        = "github"
    secret_name = "hook-secret"
  }
}

data "kargo_project_config" "test" {
  project    = kargo_project_config.test.project
  depends_on = [kargo_project_config.test]
}
`, url, selector, enabled)
}
