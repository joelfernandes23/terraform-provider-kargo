package provider

import (
	"context"
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

func TestWarehouseDataSourceMetadata(t *testing.T) {
	d := &WarehouseDataSource{}
	resp := &fwdatasource.MetadataResponse{}
	d.Metadata(context.Background(), fwdatasource.MetadataRequest{ProviderTypeName: "kargo"}, resp)

	if resp.TypeName != "kargo_warehouse" {
		t.Errorf("expected type name %q, got %q", "kargo_warehouse", resp.TypeName)
	}
}

func TestWarehouseDataSourceSchema(t *testing.T) {
	d := &WarehouseDataSource{}
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

	for _, name := range []string{"id", "subscription", "status", "freight"} {
		attr, ok := resp.Schema.Attributes[name]
		if !ok {
			t.Fatalf("expected %q attribute", name)
		}
		if !attr.IsComputed() {
			t.Errorf("%s should be computed", name)
		}
	}
}

func TestWarehouseDataSourceSchemaValid(t *testing.T) {
	d := &WarehouseDataSource{}
	resp := &fwdatasource.SchemaResponse{}
	d.Schema(context.Background(), fwdatasource.SchemaRequest{}, resp)

	diags := resp.Schema.ValidateImplementation(context.Background())
	if diags.HasError() {
		t.Fatalf("schema implementation invalid: %s", diags)
	}
}

func TestWarehouseDataSourceConfigureWrongType(t *testing.T) {
	d := &WarehouseDataSource{}
	resp := &fwdatasource.ConfigureResponse{}
	d.Configure(context.Background(), fwdatasource.ConfigureRequest{ProviderData: "wrong"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

func TestWarehouseDataSourceConfigureNil(t *testing.T) {
	d := &WarehouseDataSource{}
	resp := &fwdatasource.ConfigureResponse{}
	d.Configure(context.Background(), fwdatasource.ConfigureRequest{ProviderData: nil}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error: %s", resp.Diagnostics)
	}
}

func TestWarehouseDataSourceConfigureCorrectType(t *testing.T) {
	d := &WarehouseDataSource{}
	resp := &fwdatasource.ConfigureResponse{}
	c := &client.Client{}
	d.Configure(context.Background(), fwdatasource.ConfigureRequest{ProviderData: c}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error: %s", resp.Diagnostics)
	}
}

func TestNewWarehouseDataSource(t *testing.T) {
	d := NewWarehouseDataSource()
	if d == nil {
		t.Fatal("expected non-nil data source")
	}
	if _, ok := d.(*WarehouseDataSource); !ok {
		t.Error("expected *WarehouseDataSource")
	}
}

type warehouseDataSourceServerOptions struct {
	noFreight bool
	notFound  bool
}

func testWarehouseDataSourceServer(t *testing.T, opts warehouseDataSourceServerOptions) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case endsWith(r.URL.Path, "/AdminLogin"):
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{"idToken": "test-jwt"})) //nolint:gosec // test
		case endsWith(r.URL.Path, "/GetWarehouse"):
			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			if opts.notFound {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"code":"not_found","message":"warehouse %q not found"}`, body["name"])
				return
			}
			assertNoError(t, json.NewEncoder(w).Encode(testWarehouseDataSourceWarehouse(body["project"], body["name"])))
		case endsWith(r.URL.Path, "/QueryFreight"):
			var body struct {
				Project string   `json:"project"`
				Origins []string `json:"origins"`
			}
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			if body.Project != "demo" {
				t.Errorf("expected QueryFreight project demo, got %q", body.Project)
			}
			if len(body.Origins) != 1 || body.Origins[0] != "app" {
				t.Errorf("expected QueryFreight origins [app], got %#v", body.Origins)
			}
			if opts.noFreight {
				assertNoError(t, json.NewEncoder(w).Encode(map[string]any{
					"groups": map[string]any{"": map[string]any{"freight": []any{}}},
				}))
				return
			}
			assertNoError(t, json.NewEncoder(w).Encode(testWarehouseDataSourceFreight()))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func testWarehouseDataSourceWarehouse(project, name string) map[string]any {
	return map[string]any{
		"warehouse": map[string]any{
			"metadata": map[string]any{
				"name":            name,
				"namespace":       project,
				"uid":             "uid-" + name,
				"resourceVersion": "1",
			},
			"spec": map[string]any{
				"subscriptions": []map[string]any{
					{
						"image": map[string]any{
							"repoURL":                "ghcr.io/example/app",
							"constraint":             "^1.0.0",
							"imageSelectionStrategy": "SemVer",
							"platform":               "linux/amd64",
						},
					},
					{
						"git": map[string]any{
							"repoURL":          "https://github.com/example/app.git",
							"branch":           "main",
							"semverConstraint": "^1.0.0",
						},
					},
					{
						"chart": map[string]any{
							"repoURL":          "https://charts.example.com",
							"name":             "app",
							"semverConstraint": "^1.0.0",
						},
					},
				},
			},
			"status": map[string]any{
				"conditions": []map[string]string{
					{
						"type":               "Reconciling",
						"status":             "False",
						"reason":             "Idle",
						"message":            "warehouse is idle",
						"lastTransitionTime": "2024-05-01T00:00:00Z",
					},
					{
						"type":               "Ready",
						"status":             "True",
						"reason":             "WarehouseReady",
						"message":            "warehouse is ready",
						"lastTransitionTime": "2024-05-01T00:01:00Z",
					},
				},
				"lastHandledRefresh": "refresh-1",
				"observedGeneration": "42",
				"lastFreightID":      "freight-current",
			},
		},
	}
}

func testWarehouseDataSourceFreight() map[string]any {
	return map[string]any{
		"groups": map[string]any{
			"": map[string]any{
				"freight": []map[string]any{
					{
						"metadata": map[string]any{"name": "freight-old", "namespace": "demo"},
						"alias":    "previous-build",
						"origin":   map[string]string{"kind": "Warehouse", "name": "app"},
						"images": []map[string]string{{
							"repoURL": "ghcr.io/example/app",
							"tag":     "1.2.2",
							"digest":  "sha256:old",
						}},
					},
					{
						"metadata": map[string]any{"name": "freight-current", "namespace": "demo"},
						"alias":    "current-build",
						"origin":   map[string]string{"kind": "Warehouse", "name": "app"},
						"commits": []map[string]string{{
							"repoURL":   "https://github.com/example/app.git",
							"id":        "abc123",
							"branch":    "main",
							"tag":       "v1.2.3",
							"message":   "Release v1.2.3",
							"author":    "Ada Example",
							"committer": "CI Example",
						}},
						"images": []map[string]any{{
							"repoURL": "ghcr.io/example/app",
							"tag":     "1.2.3",
							"digest":  "sha256:123",
							"annotations": map[string]string{
								"example.com/internal-build": "ignored",
							},
						}},
						"charts": []map[string]string{{
							"repoURL": "https://charts.example.com",
							"name":    "app",
							"version": "1.2.3",
						}},
						"artifacts": []map[string]any{{
							"artifactType":     "application/example",
							"subscriptionName": "bundle",
							"version":          "v1.2.3",
							"metadata":         map[string]string{"internal": "ignored"},
						}},
						"status": map[string]any{
							"currentlyIn": map[string]any{
								"test": map[string]string{"since": "2024-05-01T00:00:00Z"},
								"prod": map[string]string{"since": "2024-05-02T00:00:00Z"},
							},
							"verifiedIn": map[string]any{
								"test": map[string]any{
									"verifiedAt":  "2024-05-01T01:00:00Z",
									"longestSoak": "1h0m0s",
								},
							},
							"approvedFor": map[string]any{
								"prod": map[string]string{"approvedAt": "2024-05-01T02:00:00Z"},
							},
							"metadata": map[string]any{"internal": "ignored"},
						},
					},
				},
			},
		},
	}
}

func TestAccWarehouseDataSource_basic(t *testing.T) {
	srv := testWarehouseDataSourceServer(t, warehouseDataSourceServerOptions{})
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testWarehouseDataSourceConfig(srv.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "project", "demo"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "name", "app"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "id", "demo/app"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "subscription.#", "3"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "subscription.0.image.repo_url", "ghcr.io/example/app"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "subscription.1.git.repo_url", "https://github.com/example/app.git"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "subscription.2.chart.repo_url", "https://charts.example.com"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "status.last_handled_refresh", "refresh-1"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "status.observed_generation", "42"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "status.last_freight_id", "freight-current"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "status.conditions.#", "2"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "status.conditions.0.type", "Ready"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "status.conditions.1.type", "Reconciling"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "freight.#", "1"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "freight.0.name", "freight-current"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "freight.0.images.0.repo_url", "ghcr.io/example/app"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "freight.0.commits.0.id", "abc123"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "freight.0.currently_in.0.stage", "prod"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "freight.0.currently_in.1.stage", "test"),
					resource.TestCheckNoResourceAttr("data.kargo_warehouse.test", "freight.0.images.0.annotations.%"),
				),
			},
		},
	})
}

func TestAccWarehouseDataSource_noFreight(t *testing.T) {
	srv := testWarehouseDataSourceServer(t, warehouseDataSourceServerOptions{noFreight: true})
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testWarehouseDataSourceConfig(srv.URL),
				Check:  resource.TestCheckResourceAttr("data.kargo_warehouse.test", "freight.#", "0"),
			},
		},
	})
}

func TestAccWarehouseDataSource_filtersToLastFreightID(t *testing.T) {
	srv := testWarehouseDataSourceServer(t, warehouseDataSourceServerOptions{})
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testWarehouseDataSourceConfig(srv.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "freight.#", "1"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "freight.0.name", "freight-current"),
					resource.TestCheckNoResourceAttr("data.kargo_warehouse.test", "freight.1.name"),
				),
			},
		},
	})
}

func TestAccWarehouseDataSource_sortsConditions(t *testing.T) {
	srv := testWarehouseDataSourceServer(t, warehouseDataSourceServerOptions{})
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testWarehouseDataSourceConfig(srv.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "status.conditions.0.type", "Ready"),
					resource.TestCheckResourceAttr("data.kargo_warehouse.test", "status.conditions.1.type", "Reconciling"),
				),
			},
		},
	})
}

func TestAccWarehouseDataSource_notFound(t *testing.T) {
	srv := testWarehouseDataSourceServer(t, warehouseDataSourceServerOptions{notFound: true})
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      testWarehouseDataSourceConfig(srv.URL),
				ExpectError: regexp.MustCompile(`(?i)(warehouse not found|not_found)`),
			},
		},
	})
}

func TestFlattenWarehouseDataSourceFreightFiltersToLastFreightID(t *testing.T) {
	freight := []client.Freight{
		{Metadata: client.FreightMetadata{Name: "freight-z"}},
		{Metadata: client.FreightMetadata{Name: "freight-current"}},
	}

	models := flattenWarehouseDataSourceFreight("freight-current", freight)
	if len(models) != 1 {
		t.Fatalf("expected one current freight item, got %d", len(models))
	}
	if models[0].Name.ValueString() != "freight-current" {
		t.Fatalf("expected current freight, got %s", models[0].Name.ValueString())
	}

	if got := flattenWarehouseDataSourceFreight("", freight); len(got) != 0 {
		t.Fatalf("expected no freight when last_freight_id is empty, got %d", len(got))
	}
}

func testWarehouseDataSourceConfig(url string) string {
	return fmt.Sprintf(`
provider "kargo" {
  api_url        = %q
  admin_password = "admin"
}

data "kargo_warehouse" "test" {
  project = "demo"
  name    = "app"
}
`, url)
}
