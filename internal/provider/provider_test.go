package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func testAccProtoV6ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"kargo": providerserver.NewProtocol6WithError(New("test")()),
	}
}

func testKargoServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/akuity.io.kargo.service.v1alpha1.KargoService/AdminLogin" {
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{"idToken": "test-jwt"})) //nolint:gosec // test
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestProviderMetadata(t *testing.T) {
	p := &KargoProvider{version: "1.2.3"}
	resp := &fwprovider.MetadataResponse{}
	p.Metadata(context.Background(), fwprovider.MetadataRequest{}, resp)

	if resp.TypeName != "kargo" {
		t.Errorf("expected TypeName %q, got %q", "kargo", resp.TypeName)
	}
	if resp.Version != "1.2.3" {
		t.Errorf("expected Version %q, got %q", "1.2.3", resp.Version)
	}
}

func TestProviderSchema(t *testing.T) {
	p := &KargoProvider{}
	resp := &fwprovider.SchemaResponse{}
	p.Schema(context.Background(), fwprovider.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %s", resp.Diagnostics)
	}

	attrs := resp.Schema.Attributes
	for _, name := range []string{"api_url", "bearer_token", "admin_password", "insecure_skip_tls_verify"} {
		if _, ok := attrs[name]; !ok {
			t.Errorf("expected attribute %q in schema", name)
		}
	}
}

func TestProviderSchemaAttributeProperties(t *testing.T) {
	p := &KargoProvider{}
	resp := &fwprovider.SchemaResponse{}
	p.Schema(context.Background(), fwprovider.SchemaRequest{}, resp)

	attrs := resp.Schema.Attributes

	if !attrs["bearer_token"].IsSensitive() {
		t.Error("bearer_token should be sensitive")
	}
	if !attrs["admin_password"].IsSensitive() {
		t.Error("admin_password should be sensitive")
	}
	if attrs["api_url"].IsSensitive() {
		t.Error("api_url should not be sensitive")
	}
	if attrs["insecure_skip_tls_verify"].IsSensitive() {
		t.Error("insecure_skip_tls_verify should not be sensitive")
	}
}

func TestProviderResources(t *testing.T) {
	p := &KargoProvider{}
	resources := p.Resources(context.Background())
	if resources == nil {
		t.Error("expected non-nil resources slice")
	}
}

func TestProviderDataSources(t *testing.T) {
	p := &KargoProvider{}
	dataSources := p.DataSources(context.Background())
	if dataSources == nil {
		t.Error("expected non-nil data sources slice")
	}
	if len(dataSources) != 1 {
		t.Errorf("expected 1 data source, got %d", len(dataSources))
	}
}

func TestNew(t *testing.T) {
	factory := New("test-version")
	p := factory()
	if p == nil {
		t.Fatal("expected non-nil provider")
	}

	kp, ok := p.(*KargoProvider)
	if !ok {
		t.Fatal("expected *KargoProvider")
	}
	if kp.version != "test-version" {
		t.Errorf("expected version %q, got %q", "test-version", kp.version)
	}
}

func TestProviderModelFields(t *testing.T) {
	m := KargoProviderModel{
		APIURL:                types.StringValue("https://example.com"),
		BearerToken:           types.StringValue("tok"),
		AdminPassword:         types.StringValue("pass"),
		InsecureSkipTLSVerify: types.BoolValue(true),
	}

	if m.APIURL.ValueString() != "https://example.com" {
		t.Error("APIURL mismatch")
	}
	if m.BearerToken.ValueString() != "tok" {
		t.Error("BearerToken mismatch")
	}
	if m.AdminPassword.ValueString() != "pass" {
		t.Error("AdminPassword mismatch")
	}
	if !m.InsecureSkipTLSVerify.ValueBool() {
		t.Error("InsecureSkipTLSVerify mismatch")
	}
}

func TestAccProviderConfigure_AdminPassword(t *testing.T) {
	srv := testKargoServer(t)
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig(srv.URL, "admin"),
			},
		},
	})
}

func TestAccProviderConfigure_BearerToken(t *testing.T) {
	srv := testKargoServer(t)
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testProviderConfigBearer(srv.URL, "my-jwt"),
			},
		},
	})
}

func testProviderConfig(url, password string) string {
	return `
provider "kargo" {
  api_url                  = "` + url + `"
  admin_password           = "` + password + `"
  insecure_skip_tls_verify = true
}
`
}

func testProviderConfigBearer(url, token string) string {
	return `
provider "kargo" {
  api_url                  = "` + url + `"
  bearer_token             = "` + token + `"
  insecure_skip_tls_verify = true
}
`
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
