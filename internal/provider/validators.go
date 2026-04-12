package provider

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// Kubernetes DNS-1123 label: lowercase alphanumeric and hyphens, start/end with alphanumeric, max 63 chars.
var rfc1123LabelRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func rfc1123NameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 63),
		stringvalidator.RegexMatches(
			rfc1123LabelRegex,
			"must be a valid RFC 1123 DNS label: lowercase alphanumeric characters or hyphens, "+
				"must start and end with an alphanumeric character",
		),
	}
}

type urlValidator struct{}

var _ validator.String = urlValidator{}

func (v urlValidator) Description(_ context.Context) string {
	return "value must be a valid URL with http or https scheme"
}

func (v urlValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v urlValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid URL",
			fmt.Sprintf("%q is not a valid URL: must use http or https scheme and include a host", value),
		)
	}
}

func urlFormatValidator() validator.String {
	return urlValidator{}
}
