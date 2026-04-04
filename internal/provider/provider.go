package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

var _ provider.Provider = &KargoProvider{}

type KargoProvider struct {
	version string
}

type KargoProviderModel struct {
	APIURL                types.String `tfsdk:"api_url"`
	BearerToken           types.String `tfsdk:"bearer_token"`
	AdminPassword         types.String `tfsdk:"admin_password"`
	InsecureSkipTLSVerify types.Bool   `tfsdk:"insecure_skip_tls_verify"`
}

func (p *KargoProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "kargo"
	resp.Version = p.version
}

func (p *KargoProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Interact with Kargo.",
		Attributes: map[string]schema.Attribute{
			"api_url": schema.StringAttribute{
				Optional:    true,
				Description: "The URL of the Kargo API. Can also be set with the KARGO_API_URL environment variable.",
			},
			"bearer_token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Bearer token for Kargo API authentication. Can also be set with the KARGO_BEARER_TOKEN environment variable.",
			},
			"admin_password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Admin account password for Kargo API authentication. Can also be set with the KARGO_ADMIN_PASSWORD environment variable.",
			},
			"insecure_skip_tls_verify": schema.BoolAttribute{
				Optional:    true,
				Description: "Skip TLS certificate verification. Defaults to false. Can also be set with the KARGO_INSECURE_SKIP_TLS_VERIFY environment variable.",
			},
		},
	}
}

func (p *KargoProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data KargoProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg := client.Config{
		APIURL:                data.APIURL.ValueString(),
		BearerToken:           data.BearerToken.ValueString(),
		AdminPassword:         data.AdminPassword.ValueString(),
		InsecureSkipTLSVerify: data.InsecureSkipTLSVerify.ValueBool(),
	}

	kargoClient, err := client.NewClient(ctx, cfg)
	if err != nil {
		resp.Diagnostics.AddError("Failed to configure Kargo client", err.Error())
		return
	}

	resp.DataSourceData = kargoClient
	resp.ResourceData = kargoClient
}

func (p *KargoProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{}
}

func (p *KargoProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &KargoProvider{
			version: version,
		}
	}
}
