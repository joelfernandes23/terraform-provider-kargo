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

func TestProjectDataSourceMetadata(t *testing.T) {
	d := &ProjectDataSource{}
	resp := &fwdatasource.MetadataResponse{}
	d.Metadata(context.Background(), fwdatasource.MetadataRequest{ProviderTypeName: "kargo"}, resp)

	if resp.TypeName != "kargo_project" {
		t.Errorf("expected type name %q, got %q", "kargo_project", resp.TypeName)
	}
}

func TestProjectDataSourceSchema(t *testing.T) {
	d := &ProjectDataSource{}
	resp := &fwdatasource.SchemaResponse{}
	d.Schema(context.Background(), fwdatasource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %s", resp.Diagnostics)
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

	statusAttr, ok := resp.Schema.Attributes["status"]
	if !ok {
		t.Fatal("expected 'status' attribute")
	}
	if !statusAttr.IsComputed() {
		t.Error("status should be computed")
	}
}

func TestProjectDataSourceSchemaValid(t *testing.T) {
	d := &ProjectDataSource{}
	resp := &fwdatasource.SchemaResponse{}
	d.Schema(context.Background(), fwdatasource.SchemaRequest{}, resp)

	diags := resp.Schema.ValidateImplementation(context.Background())
	if diags.HasError() {
		t.Fatalf("schema implementation invalid: %s", diags)
	}
}

func TestProjectDataSourceConfigureWrongType(t *testing.T) {
	d := &ProjectDataSource{}
	resp := &fwdatasource.ConfigureResponse{}
	d.Configure(context.Background(), fwdatasource.ConfigureRequest{ProviderData: "wrong"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

func TestProjectDataSourceConfigureNil(t *testing.T) {
	d := &ProjectDataSource{}
	resp := &fwdatasource.ConfigureResponse{}
	d.Configure(context.Background(), fwdatasource.ConfigureRequest{ProviderData: nil}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error: %s", resp.Diagnostics)
	}
}

func TestProjectDataSourceConfigureCorrectType(t *testing.T) {
	d := &ProjectDataSource{}
	resp := &fwdatasource.ConfigureResponse{}
	c := &client.Client{}
	d.Configure(context.Background(), fwdatasource.ConfigureRequest{ProviderData: c}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error: %s", resp.Diagnostics)
	}
}

func TestNewProjectDataSource(t *testing.T) {
	d := NewProjectDataSource()
	if d == nil {
		t.Fatal("expected non-nil data source")
	}
	if _, ok := d.(*ProjectDataSource); !ok {
		t.Error("expected *ProjectDataSource")
	}
}

func testProjectDataSourceServer(t *testing.T) *httptest.Server {
	t.Helper()
	projects := map[string]bool{}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case endsWith(r.URL.Path, "/AdminLogin"):
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{"idToken": "test-jwt"})) //nolint:gosec // test
		case endsWith(r.URL.Path, "/CreateResource"):
			projects["tf-test-project"] = true
			assertNoError(t, json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]string{{"createdResourceManifest": "dGVzdA=="}},
			}))
		case endsWith(r.URL.Path, "/GetProject"):
			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			name := body["name"]
			if !projects[name] {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprintf(w, `{"code":"not_found","message":"project %q not found"}`, name)
				return
			}
			assertNoError(t, json.NewEncoder(w).Encode(map[string]any{
				"project": map[string]any{
					"metadata": map[string]any{"name": name, "uid": "uid-" + name, "resourceVersion": "1"},
					"status": map[string]any{
						"phase": "Ready",
						"conditions": []map[string]string{{
							"type":               "Ready",
							"status":             "True",
							"reason":             "ProjectReady",
							"message":            "project is ready",
							"lastTransitionTime": "2024-01-01T00:00:00Z",
						}},
					},
				},
			}))
		case endsWith(r.URL.Path, "/DeleteProject"):
			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			delete(projects, body["name"])
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAccProjectDataSource_basic(t *testing.T) {
	srv := testProjectDataSourceServer(t)
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testProjectDataSourceConfig(srv.URL, "tf-test-project"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.kargo_project.test", "name", "tf-test-project"),
					resource.TestCheckResourceAttr("data.kargo_project.test", "id", "tf-test-project"),
					resource.TestCheckResourceAttr("data.kargo_project.test", "status.phase", "Ready"),
					resource.TestCheckResourceAttr("data.kargo_project.test", "status.conditions.#", "1"),
					resource.TestCheckResourceAttr("data.kargo_project.test", "status.conditions.0.type", "Ready"),
					resource.TestCheckResourceAttr("data.kargo_project.test", "status.conditions.0.status", "True"),
					resource.TestCheckResourceAttr("data.kargo_project.test", "status.conditions.0.reason", "ProjectReady"),
					resource.TestCheckResourceAttr("data.kargo_project.test", "status.conditions.0.message", "project is ready"),
					resource.TestCheckResourceAttr("data.kargo_project.test", "status.conditions.0.last_transition_time", "2024-01-01T00:00:00Z"),
				),
			},
		},
	})
}

func TestAccProjectDataSource_notFound(t *testing.T) {
	srv := testProjectDataSourceServer(t)
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      testProjectDataSourceNotFoundConfig(srv.URL),
				ExpectError: regexp.MustCompile(`(?i)(not.found|Failed to read project)`),
			},
		},
	})
}

func testProjectDataSourceConfig(url, name string) string {
	return fmt.Sprintf(`
provider "kargo" {
  api_url        = %q
  admin_password = "admin"
}

resource "kargo_project" "test" {
  name = %q
}

data "kargo_project" "test" {
  name = kargo_project.test.name

  depends_on = [kargo_project.test]
}
`, url, name)
}

func testProjectDataSourceNotFoundConfig(url string) string {
	return fmt.Sprintf(`
provider "kargo" {
  api_url        = %q
  admin_password = "admin"
}

data "kargo_project" "test" {
  name = "nonexistent-project"
}
`, url)
}
