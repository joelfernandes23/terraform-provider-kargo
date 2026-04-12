package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestRFC1123NameValidators_Valid(t *testing.T) {
	valid := []string{"my-project", "abc", "a", "a1b2c3", "test-123-name", "a-b", "0start", "end0"}
	validators := rfc1123NameValidators()

	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			for _, v := range validators {
				req := validator.StringRequest{
					Path:        path.Root("name"),
					ConfigValue: types.StringValue(name),
				}
				resp := &validator.StringResponse{}
				v.ValidateString(context.Background(), req, resp)
				if resp.Diagnostics.HasError() {
					t.Errorf("expected %q to be valid, got errors: %s", name, resp.Diagnostics)
				}
			}
		})
	}
}

func TestRFC1123NameValidators_Invalid(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"uppercase", "My-Project"},
		{"starts-with-hyphen", "-start"},
		{"ends-with-hyphen", "end-"},
		{"underscore", "has_underscore"},
		{"dot", "has.dot"},
		{"all-uppercase", "UPPER"},
		{"too-long", strings.Repeat("a", 64)},
		{"empty", ""},
	}
	validators := rfc1123NameValidators()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hasError := false
			for _, v := range validators {
				req := validator.StringRequest{
					Path:        path.Root("name"),
					ConfigValue: types.StringValue(tc.input),
				}
				resp := &validator.StringResponse{}
				v.ValidateString(context.Background(), req, resp)
				if resp.Diagnostics.HasError() {
					hasError = true
					break
				}
			}
			if !hasError {
				t.Errorf("expected %q to be invalid", tc.input)
			}
		})
	}
}

func TestURLValidator_Valid(t *testing.T) {
	valid := []string{
		"https://kargo.example.com",
		"http://localhost:8080",
		"https://10.0.0.1:443/api",
		"http://kargo.dev",
		"https://kargo:8443",
	}

	v := urlFormatValidator()
	for _, u := range valid {
		t.Run(u, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("api_url"),
				ConfigValue: types.StringValue(u),
			}
			resp := &validator.StringResponse{}
			v.ValidateString(context.Background(), req, resp)
			if resp.Diagnostics.HasError() {
				t.Errorf("expected %q to be valid, got errors: %s", u, resp.Diagnostics)
			}
		})
	}
}

func TestURLValidator_Invalid(t *testing.T) {
	invalid := []string{
		"not-a-url",
		"ftp://example.com",
		"://no-scheme",
		"just-host.com",
	}

	v := urlFormatValidator()
	for _, u := range invalid {
		t.Run(u, func(t *testing.T) {
			req := validator.StringRequest{
				Path:        path.Root("api_url"),
				ConfigValue: types.StringValue(u),
			}
			resp := &validator.StringResponse{}
			v.ValidateString(context.Background(), req, resp)
			if !resp.Diagnostics.HasError() {
				t.Errorf("expected %q to be invalid", u)
			}
		})
	}
}

func TestURLValidator_NullAndUnknown(t *testing.T) {
	v := urlFormatValidator()

	t.Run("null", func(t *testing.T) {
		req := validator.StringRequest{
			Path:        path.Root("api_url"),
			ConfigValue: types.StringNull(),
		}
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), req, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("expected null to pass validation, got errors: %s", resp.Diagnostics)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		req := validator.StringRequest{
			Path:        path.Root("api_url"),
			ConfigValue: types.StringUnknown(),
		}
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), req, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("expected unknown to pass validation, got errors: %s", resp.Diagnostics)
		}
	})
}

func TestURLValidator_Description(t *testing.T) {
	v := urlFormatValidator()
	desc := v.Description(context.Background())
	if desc == "" {
		t.Error("expected non-empty description")
	}
	mdDesc := v.MarkdownDescription(context.Background())
	if mdDesc == "" {
		t.Error("expected non-empty markdown description")
	}
}
