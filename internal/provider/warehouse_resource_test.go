package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

func TestWarehouseResourceMetadata(t *testing.T) {
	r := &WarehouseResource{}
	resp := &fwresource.MetadataResponse{}
	r.Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "kargo"}, resp)

	if resp.TypeName != "kargo_warehouse" {
		t.Errorf("expected type name %q, got %q", "kargo_warehouse", resp.TypeName)
	}
}

func TestWarehouseResourceSchema(t *testing.T) {
	r := &WarehouseResource{}
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

	if _, ok := resp.Schema.Blocks["subscription"]; !ok {
		t.Fatal("expected 'subscription' block")
	}
}

func TestWarehouseResourceSchemaValid(t *testing.T) {
	r := &WarehouseResource{}
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	diags := resp.Schema.ValidateImplementation(context.Background())
	if diags.HasError() {
		t.Fatalf("schema implementation invalid: %s", diags)
	}
}

func TestWarehouseResourceConfigureWrongType(t *testing.T) {
	r := &WarehouseResource{}
	resp := &fwresource.ConfigureResponse{}
	r.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: "wrong"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

func TestWarehouseResourceConfigureNil(t *testing.T) {
	r := &WarehouseResource{}
	resp := &fwresource.ConfigureResponse{}
	r.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: nil}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error: %s", resp.Diagnostics)
	}
}

func TestNewWarehouseResource(t *testing.T) {
	r := NewWarehouseResource()
	if r == nil {
		t.Fatal("expected non-nil resource")
	}
	if _, ok := r.(*WarehouseResource); !ok {
		t.Error("expected *WarehouseResource")
	}
}

func TestParseWarehouseID(t *testing.T) {
	project, name, err := parseWarehouseID("demo/app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project != "demo" || name != "app" {
		t.Fatalf("expected demo/app, got %s/%s", project, name)
	}

	for _, id := range []string{"demo", "/app", "demo/", "demo/app/extra"} {
		t.Run(id, func(t *testing.T) {
			_, _, err := parseWarehouseID(id)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestExpandWarehouseSpecRequiresExactlyOneSubscriptionKind(t *testing.T) {
	_, err := expandWarehouseSpec(nil)
	if err == nil || !strings.Contains(err.Error(), "at least one subscription") {
		t.Fatalf("expected at-least-one error, got %v", err)
	}

	_, err = expandWarehouseSpec([]WarehouseSubscriptionModel{{}})
	if err == nil || !strings.Contains(err.Error(), "exactly one of image, git, or chart") {
		t.Fatalf("expected exactly-one error, got %v", err)
	}

	_, err = expandWarehouseSpec([]WarehouseSubscriptionModel{{
		Image: &WarehouseImageSubscriptionModel{RepoURL: types.StringValue("ghcr.io/example/app")},
		Git:   &WarehouseGitSubscriptionModel{RepoURL: types.StringValue("https://github.com/example/repo.git")},
	}})
	if err == nil || !strings.Contains(err.Error(), "exactly one of image, git, or chart") {
		t.Fatalf("expected exactly-one error, got %v", err)
	}
}

func TestExpandWarehouseSpecMapsImageFields(t *testing.T) {
	spec, err := expandWarehouseSpec([]WarehouseSubscriptionModel{{
		Image: &WarehouseImageSubscriptionModel{
			RepoURL:              types.StringValue("ghcr.io/example/app"),
			SemverConstraint:     types.StringValue("^1.0.0"),
			TagSelectionStrategy: types.StringValue("SemVer"),
			Platform:             types.StringValue("linux/amd64"),
		},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	image := spec.Subscriptions[0].Image
	if image == nil {
		t.Fatal("expected image subscription")
	} else {
		if image.Constraint != "^1.0.0" {
			t.Errorf("expected constraint %q, got %q", "^1.0.0", image.Constraint)
		}
		if image.ImageSelectionStrategy != "SemVer" {
			t.Errorf("expected strategy %q, got %q", "SemVer", image.ImageSelectionStrategy)
		}
		if image.Platform != "linux/amd64" {
			t.Errorf("expected platform %q, got %q", "linux/amd64", image.Platform)
		}
	}
}

func TestExpandWarehouseSpecRequiresRepoURLForSelectedKind(t *testing.T) {
	for name, sub := range map[string]WarehouseSubscriptionModel{
		"image": {Image: &WarehouseImageSubscriptionModel{RepoURL: types.StringNull()}},
		"git":   {Git: &WarehouseGitSubscriptionModel{RepoURL: types.StringNull()}},
		"chart": {Chart: &WarehouseChartSubscriptionModel{RepoURL: types.StringNull()}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := expandWarehouseSpec([]WarehouseSubscriptionModel{sub})
			if err == nil || !strings.Contains(err.Error(), name+".repo_url must be set") {
				t.Fatalf("expected repo_url error, got %v", err)
			}
		})
	}
}

func TestExpandWarehouseSpecMapsGitSemverStrategy(t *testing.T) {
	spec, err := expandWarehouseSpec([]WarehouseSubscriptionModel{{
		Git: &WarehouseGitSubscriptionModel{
			RepoURL:          types.StringValue("https://github.com/example/repo.git"),
			Branch:           types.StringValue("main"),
			SemverConstraint: types.StringValue("^1.0.0"),
		},
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	git := spec.Subscriptions[0].Git
	if git == nil {
		t.Fatal("expected git subscription")
	} else {
		if git.SemverConstraint != "^1.0.0" {
			t.Errorf("expected semver constraint %q, got %q", "^1.0.0", git.SemverConstraint)
		}
		if git.CommitSelectionStrategy != "SemVer" {
			t.Errorf("expected commit selection strategy %q, got %q", "SemVer", git.CommitSelectionStrategy)
		}
	}
}

func TestFlattenWarehousePreservesOmittedOptionalFields(t *testing.T) {
	prior := &WarehouseResourceModel{
		Project: types.StringValue("demo"),
		Name:    types.StringValue("app"),
		Subscription: []WarehouseSubscriptionModel{{
			Image: &WarehouseImageSubscriptionModel{
				RepoURL:              types.StringValue("ghcr.io/example/app"),
				SemverConstraint:     types.StringNull(),
				TagSelectionStrategy: types.StringNull(),
				Platform:             types.StringNull(),
			},
		}},
	}
	warehouse := &client.Warehouse{
		Metadata: client.WarehouseMetadata{Name: "app", Namespace: "demo"},
		Spec: client.WarehouseSpec{
			Subscriptions: []client.WarehouseSubscription{{
				Image: &client.ImageSubscription{
					RepoURL:                "ghcr.io/example/app",
					Constraint:             "^1.0.0",
					ImageSelectionStrategy: "SemVer",
					Platform:               "linux/amd64",
				},
			}},
		},
	}

	flattened := flattenWarehouse("demo", warehouse, prior)
	image := flattened.Subscription[0].Image
	if !image.SemverConstraint.IsNull() {
		t.Errorf("expected omitted semver constraint to stay null, got %s", image.SemverConstraint)
	}
	if !image.TagSelectionStrategy.IsNull() {
		t.Errorf("expected omitted tag selection strategy to stay null, got %s", image.TagSelectionStrategy)
	}
	if !image.Platform.IsNull() {
		t.Errorf("expected omitted platform to stay null, got %s", image.Platform)
	}

	imported := flattenWarehouse("demo", warehouse, nil)
	if imported.Subscription[0].Image.TagSelectionStrategy.ValueString() != "SemVer" {
		t.Errorf("expected import to include server value, got %s", imported.Subscription[0].Image.TagSelectionStrategy)
	}
}

type warehouseTestServer struct {
	*httptest.Server

	mu          sync.RWMutex
	warehouses  map[string]map[string]any
	updateCount int
}

func testWarehouseServer(t *testing.T) *warehouseTestServer {
	t.Helper()

	state := &warehouseTestServer{
		warehouses: map[string]map[string]any{},
	}

	state.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case endsWith(r.URL.Path, "/AdminLogin"):
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{"idToken": "test-jwt"})) //nolint:gosec // test
		case endsWith(r.URL.Path, "/CreateResource"):
			manifest, encoded := decodeWarehouseManifest(t, r)
			key := warehouseManifestKey(t, manifest)

			state.mu.Lock()
			state.warehouses[key] = storedWarehouse(manifest, fmt.Sprintf("%d", len(state.warehouses)+1))
			state.mu.Unlock()

			assertNoError(t, json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]string{{"createdResourceManifest": encoded}},
			}))
		case endsWith(r.URL.Path, "/UpdateResource"):
			manifest, encoded := decodeWarehouseManifest(t, r)
			key := warehouseManifestKey(t, manifest)

			state.mu.Lock()
			if _, ok := state.warehouses[key]; !ok {
				state.mu.Unlock()
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"code":"not_found","message":"warehouse not found"}`)
				return
			}
			state.updateCount++
			state.warehouses[key] = storedWarehouse(manifest, fmt.Sprintf("%d", state.updateCount+100))
			state.mu.Unlock()

			assertNoError(t, json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]string{{"updatedResourceManifest": encoded}},
			}))
		case endsWith(r.URL.Path, "/GetWarehouse"):
			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			key := warehouseID(body["project"], body["name"])

			state.mu.RLock()
			warehouse, ok := state.warehouses[key]
			state.mu.RUnlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"code":"not_found","message":"warehouse not found"}`)
				return
			}
			assertNoError(t, json.NewEncoder(w).Encode(map[string]any{"warehouse": warehouse}))
		case endsWith(r.URL.Path, "/DeleteWarehouse"):
			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			key := warehouseID(body["project"], body["name"])

			state.mu.Lock()
			delete(state.warehouses, key)
			state.mu.Unlock()

			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return state
}

func decodeWarehouseManifest(t *testing.T, r *http.Request) (manifest map[string]any, encoded string) {
	t.Helper()

	var body map[string]string
	assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
	encoded = body["manifest"]
	manifestJSON, err := base64.StdEncoding.DecodeString(encoded)
	assertNoError(t, err)

	assertNoError(t, json.Unmarshal(manifestJSON, &manifest))
	if manifest["kind"] != "Warehouse" {
		t.Fatalf("expected kind Warehouse, got %v", manifest["kind"])
	}
	return manifest, encoded
}

func warehouseManifestKey(t *testing.T, manifest map[string]any) string {
	t.Helper()

	metadata, ok := manifest["metadata"].(map[string]any)
	if !ok {
		t.Fatal("manifest metadata missing")
	}
	project, _ := metadata["namespace"].(string)
	name, _ := metadata["name"].(string)
	if project == "" || name == "" {
		t.Fatalf("manifest missing namespace/name: %#v", metadata)
	}
	return warehouseID(project, name)
}

func storedWarehouse(manifest map[string]any, resourceVersion string) map[string]any {
	metadata := manifest["metadata"].(map[string]any)
	project := metadata["namespace"].(string)
	name := metadata["name"].(string)

	return map[string]any{
		"metadata": map[string]any{
			"name":            name,
			"namespace":       project,
			"uid":             "uid-" + name,
			"resourceVersion": resourceVersion,
		},
		"spec": manifest["spec"],
		"status": map[string]any{
			"conditions": []map[string]string{{"type": "Ready", "status": "True"}},
		},
	}
}

func (s *warehouseTestServer) deleteWarehouseForTest(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.warehouses, key)
}

func (s *warehouseTestServer) updateCountForTest() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updateCount
}

func TestAccWarehouseResource_basic(t *testing.T) {
	srv := testWarehouseServer(t)
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testWarehouseResourceImageConfig(srv.URL, "tf-test-project", "tf-test-warehouse", "^1.0.0"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_warehouse.test", "id", "tf-test-project/tf-test-warehouse"),
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.0.image.repo_url", "ghcr.io/example/app"),
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.0.image.semver_constraint", "^1.0.0"),
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.0.image.tag_selection_strategy", "SemVer"),
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.0.image.platform", "linux/amd64"),
				),
			},
			{
				Config: testWarehouseResourceImageConfig(srv.URL, "tf-test-project", "tf-test-warehouse", "^2.0.0"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.0.image.semver_constraint", "^2.0.0"),
					testCheckWarehouseUpdateCountAtLeast(srv, 1),
				),
			},
			{
				ResourceName:      "kargo_warehouse.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: testWarehouseResourceGitConfig(srv.URL, "tf-test-project", "tf-test-warehouse"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.0.git.repo_url", "https://github.com/example/repo.git"),
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.0.git.branch", "main"),
					testCheckWarehouseUpdateCountAtLeast(srv, 2),
				),
			},
		},
	})
}

func TestAccWarehouseResource_multipleSubscriptions(t *testing.T) {
	srv := testWarehouseServer(t)
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testWarehouseResourceMultipleConfig(srv.URL, "tf-test-project", "tf-test-warehouse"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.#", "3"),
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.0.image.repo_url", "ghcr.io/example/app"),
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.1.git.repo_url", "https://github.com/example/repo.git"),
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.1.git.branch", "main"),
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.2.chart.repo_url", "https://charts.example.com"),
					resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.2.chart.name", "app"),
				),
			},
		},
	})
}

func TestAccWarehouseResource_outOfBandDeletion(t *testing.T) {
	srv := testWarehouseServer(t)
	defer srv.Close()

	config := testWarehouseResourceImageConfig(srv.URL, "tf-test-project", "tf-test-warehouse", "^1.0.0")
	key := "tf-test-project/tf-test-warehouse"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("kargo_warehouse.test", "id", key),
			},
			{
				PreConfig: func() {
					srv.deleteWarehouseForTest(key)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("kargo_warehouse.test", "id", key),
			},
		},
	})
}

func testCheckWarehouseUpdateCountAtLeast(srv *warehouseTestServer, minimum int) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if got := srv.updateCountForTest(); got < minimum {
			return fmt.Errorf("expected at least %d warehouse updates, got %d", minimum, got)
		}
		return nil
	}
}

func testWarehouseProviderConfig(url string) string {
	return fmt.Sprintf(`
provider "kargo" {
  api_url                  = %q
  admin_password           = "admin"
  insecure_skip_tls_verify = true
}
`, url)
}

func testWarehouseResourceImageConfig(url, project, name, constraint string) string {
	return fmt.Sprintf(`%s
resource "kargo_warehouse" "test" {
  project = %q
  name    = %q

  subscription {
    image {
      repo_url               = "ghcr.io/example/app"
      semver_constraint      = %q
      tag_selection_strategy = "SemVer"
      platform               = "linux/amd64"
    }
  }
}
`, testWarehouseProviderConfig(url), project, name, constraint)
}

func testWarehouseResourceGitConfig(url, project, name string) string {
	return fmt.Sprintf(`%s
resource "kargo_warehouse" "test" {
  project = %q
  name    = %q

  subscription {
    git {
      repo_url          = "https://github.com/example/repo.git"
      branch            = "main"
      semver_constraint = "^1.0.0"
    }
  }
}
`, testWarehouseProviderConfig(url), project, name)
}

func testWarehouseResourceMultipleConfig(url, project, name string) string {
	return fmt.Sprintf(`%s
resource "kargo_warehouse" "test" {
  project = %q
  name    = %q

  subscription {
    image {
      repo_url               = "ghcr.io/example/app"
      semver_constraint      = "^1.0.0"
      tag_selection_strategy = "SemVer"
    }
  }

  subscription {
    git {
      repo_url          = "https://github.com/example/repo.git"
      branch            = "main"
      semver_constraint = "^1.0.0"
    }
  }

  subscription {
    chart {
      repo_url          = "https://charts.example.com"
      name              = "app"
      semver_constraint = "^1.0.0"
    }
  }
}
`, testWarehouseProviderConfig(url), project, name)
}
