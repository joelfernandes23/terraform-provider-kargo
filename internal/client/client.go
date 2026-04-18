package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	servicePath = "/akuity.io.kargo.service.v1alpha1.KargoService"

	envAPIURL                = "KARGO_API_URL"
	envBearerToken           = "KARGO_BEARER_TOKEN" //nolint:gosec // env var name, not a credential
	envAdminPassword         = "KARGO_ADMIN_PASSWORD"
	envInsecureSkipTLSVerify = "KARGO_INSECURE_SKIP_TLS_VERIFY"

	defaultTimeout = 30 * time.Second
)

type Config struct {
	APIURL                string
	BearerToken           string
	AdminPassword         string
	InsecureSkipTLSVerify bool
	Timeout               time.Duration
}

// KargoClient defines the Kargo API operations available to Terraform resources.
type KargoClient interface {
	CreateProject(ctx context.Context, name string) (*Project, error)
	GetProject(ctx context.Context, name string) (*Project, error)
	DeleteProject(ctx context.Context, name string) error
}

var _ KargoClient = (*Client)(nil)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func (c *Config) resolveEnvDefaults() {
	if c.APIURL == "" {
		c.APIURL = os.Getenv(envAPIURL)
	}
	if c.BearerToken == "" {
		c.BearerToken = os.Getenv(envBearerToken)
	}
	if c.AdminPassword == "" {
		c.AdminPassword = os.Getenv(envAdminPassword)
	}
	if !c.InsecureSkipTLSVerify {
		c.InsecureSkipTLSVerify = strings.EqualFold(os.Getenv(envInsecureSkipTLSVerify), "true")
	}
}

func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	cfg.resolveEnvDefaults()

	if cfg.APIURL == "" {
		return nil, fmt.Errorf("api_url is required (set via provider config or %s)", envAPIURL)
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}

	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cfg.InsecureSkipTLSVerify, //nolint:gosec // user-configured for self-signed certs in dev
			},
		},
	}

	c := &Client{
		baseURL:    strings.TrimRight(cfg.APIURL, "/"),
		httpClient: httpClient,
	}

	switch {
	case cfg.BearerToken != "":
		c.token = cfg.BearerToken
	case cfg.AdminPassword != "":
		token, err := c.adminLogin(ctx, cfg.AdminPassword)
		if err != nil {
			return nil, fmt.Errorf("admin login failed: %w", err)
		}
		c.token = token
	default:
		return nil, fmt.Errorf(
			"authentication required: set bearer_token or admin_password (via provider config or %s / %s)",
			envBearerToken, envAdminPassword,
		)
	}

	return c, nil
}

type adminLoginRequest struct {
	Password string `json:"password"`
}

type adminLoginResponse struct {
	IDToken string `json:"idToken"`
}

func (c *Client) adminLogin(ctx context.Context, password string) (string, error) {
	resp, err := c.doUnary(ctx, "AdminLogin", adminLoginRequest{Password: password}, false)
	if err != nil {
		return "", err
	}

	var loginResp adminLoginResponse
	if err := json.Unmarshal(resp, &loginResp); err != nil {
		return "", fmt.Errorf("decoding login response: %w", err)
	}
	if loginResp.IDToken == "" {
		return "", fmt.Errorf("login response contained no token")
	}
	return loginResp.IDToken, nil
}

// doUnary makes a Connect-protocol unary RPC call (JSON over HTTP POST).
func (c *Client) doUnary(ctx context.Context, method string, reqBody any, auth bool) ([]byte, error) {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	url := c.baseURL + servicePath + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if auth && c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		apiErr := &APIError{
			HTTPStatus: resp.StatusCode,
			Method:     method,
		}
		var connectErr struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(respBody, &connectErr) == nil && connectErr.Code != "" {
			apiErr.Code = connectErr.Code
			apiErr.Message = connectErr.Message
		} else {
			apiErr.Code = "unknown"
			apiErr.Message = string(respBody)
		}
		return nil, apiErr
	}

	return respBody, nil
}

// Do makes an authenticated Connect RPC call and unmarshals the response into dest.
func (c *Client) Do(ctx context.Context, method string, reqBody, dest any) error {
	respBody, err := c.doUnary(ctx, method, reqBody, true)
	if err != nil {
		return err
	}

	if dest != nil {
		if err := json.Unmarshal(respBody, dest); err != nil {
			return fmt.Errorf("decoding response for %s: %w", method, err)
		}
	}
	return nil
}
