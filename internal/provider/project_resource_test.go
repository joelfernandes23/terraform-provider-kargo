package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

func TestProjectResourceMetadata(t *testing.T) {
	r := &ProjectResource{}
	resp := &fwresource.MetadataResponse{}
	r.Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "kargo"}, resp)

	if resp.TypeName != "kargo_project" {
		t.Errorf("expected type name %q, got %q", "kargo_project", resp.TypeName)
	}
}

func TestProjectResourceSchema(t *testing.T) {
	r := &ProjectResource{}
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

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
}

func TestProjectResourceSchemaValid(t *testing.T) {
	r := &ProjectResource{}
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	diags := resp.Schema.ValidateImplementation(context.Background())
	if diags.HasError() {
		t.Fatalf("schema implementation invalid: %s", diags)
	}
}

func TestProjectResourceConfigureWrongType(t *testing.T) {
	r := &ProjectResource{}
	resp := &fwresource.ConfigureResponse{}
	r.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: "wrong"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

func TestProjectResourceConfigureNil(t *testing.T) {
	r := &ProjectResource{}
	resp := &fwresource.ConfigureResponse{}
	r.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: nil}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error: %s", resp.Diagnostics)
	}
}

func TestProjectResourceConfigureCorrectType(t *testing.T) {
	r := &ProjectResource{}
	resp := &fwresource.ConfigureResponse{}
	c := &client.Client{}
	r.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: c}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error: %s", resp.Diagnostics)
	}
}

func TestNewProjectResource(t *testing.T) {
	r := NewProjectResource()
	if r == nil {
		t.Fatal("expected non-nil resource")
	}
	if _, ok := r.(*ProjectResource); !ok {
		t.Error("expected *ProjectResource")
	}
}

func testProjectServer(t *testing.T) *httptest.Server {
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
					"status":   map[string]any{"conditions": []map[string]string{{"type": "Ready", "status": "True"}}},
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

func TestAccProjectResource_basic(t *testing.T) {
	srv := testProjectServer(t)
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testProjectResourceConfig(srv.URL, "tf-test-project"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_project.test", "name", "tf-test-project"),
					resource.TestCheckResourceAttr("kargo_project.test", "id", "tf-test-project"),
				),
			},
			{
				ResourceName:      "kargo_project.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testProjectResourceConfig(url, name string) string {
	return fmt.Sprintf(`
provider "kargo" {
  api_url        = %q
  admin_password = "admin"
}

resource "kargo_project" "test" {
  name = %q
}
`, url, name)
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
