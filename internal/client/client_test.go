package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveEnvDefaults(t *testing.T) {
	t.Run("fills empty fields from env", func(t *testing.T) {
		t.Setenv(envAPIURL, "https://env-api")
		t.Setenv(envBearerToken, "env-token")
		t.Setenv(envAdminPassword, "env-pass")
		t.Setenv(envInsecureSkipTLSVerify, "true")

		cfg := Config{}
		cfg.resolveEnvDefaults()

		assertEqual(t, "https://env-api", cfg.APIURL)
		assertEqual(t, "env-token", cfg.BearerToken)
		assertEqual(t, "env-pass", cfg.AdminPassword)
		if !cfg.InsecureSkipTLSVerify {
			t.Error("expected InsecureSkipTLSVerify to be true")
		}
	})

	t.Run("explicit config takes precedence over env", func(t *testing.T) {
		t.Setenv(envAPIURL, "https://env-api")
		t.Setenv(envBearerToken, "env-token")

		cfg := Config{
			APIURL:      "https://explicit-api",
			BearerToken: "explicit-token",
		}
		cfg.resolveEnvDefaults()

		assertEqual(t, "https://explicit-api", cfg.APIURL)
		assertEqual(t, "explicit-token", cfg.BearerToken)
	})

	t.Run("insecure defaults to false", func(t *testing.T) {
		cfg := Config{}
		cfg.resolveEnvDefaults()

		if cfg.InsecureSkipTLSVerify {
			t.Error("expected InsecureSkipTLSVerify to default to false")
		}
	})

	t.Run("insecure env must be true (case-insensitive)", func(t *testing.T) {
		t.Setenv(envInsecureSkipTLSVerify, "TRUE")
		cfg := Config{}
		cfg.resolveEnvDefaults()
		if !cfg.InsecureSkipTLSVerify {
			t.Error("expected InsecureSkipTLSVerify to be true for 'TRUE'")
		}
	})

	t.Run("insecure env with non-true value stays false", func(t *testing.T) {
		t.Setenv(envInsecureSkipTLSVerify, "yes")
		cfg := Config{}
		cfg.resolveEnvDefaults()
		if cfg.InsecureSkipTLSVerify {
			t.Error("expected InsecureSkipTLSVerify to be false for 'yes'")
		}
	})

	t.Run("explicit insecure true not overridden by env", func(t *testing.T) {
		t.Setenv(envInsecureSkipTLSVerify, "false")
		cfg := Config{InsecureSkipTLSVerify: true}
		cfg.resolveEnvDefaults()
		if !cfg.InsecureSkipTLSVerify {
			t.Error("expected InsecureSkipTLSVerify to remain true")
		}
	})
}

func TestNewClient_MissingAPIURL(t *testing.T) {
	_, err := NewClient(context.Background(), Config{BearerToken: "tok"})
	assertErrorContains(t, err, "api_url is required")
}

func TestNewClient_MissingAuth(t *testing.T) {
	_, err := NewClient(context.Background(), Config{APIURL: "https://example.com"})
	assertErrorContains(t, err, "authentication required")
}

func TestNewClient_BearerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no requests should be made during bearer token auth")
	}))
	defer srv.Close()

	c, err := NewClient(context.Background(), Config{
		APIURL:      srv.URL,
		BearerToken: "my-token",
	})
	assertNoError(t, err)
	assertEqual(t, "my-token", c.token)
}

func TestNewClient_AdminPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertEqual(t, http.MethodPost, r.Method)
		assertSuffix(t, r.URL.Path, "/AdminLogin")
		assertEqual(t, "application/json", r.Header.Get("Content-Type"))

		var req adminLoginRequest
		assertNoError(t, json.NewDecoder(r.Body).Decode(&req))
		assertEqual(t, "admin-pass", req.Password)

		w.Header().Set("Content-Type", "application/json")
		assertNoError(t, json.NewEncoder(w).Encode(adminLoginResponse{IDToken: "jwt-from-server"})) //nolint:gosec // test fixture
	}))
	defer srv.Close()

	c, err := NewClient(context.Background(), Config{
		APIURL:        srv.URL,
		AdminPassword: "admin-pass",
	})
	assertNoError(t, err)
	assertEqual(t, "jwt-from-server", c.token)
}

func TestNewClient_AdminLoginFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unauthenticated","message":"wrong password"}`))
	}))
	defer srv.Close()

	_, err := NewClient(context.Background(), Config{
		APIURL:        srv.URL,
		AdminPassword: "wrong",
	})
	assertErrorContains(t, err, "admin login failed")
}

func TestNewClient_AdminLoginEmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assertNoError(t, json.NewEncoder(w).Encode(adminLoginResponse{IDToken: ""}))
	}))
	defer srv.Close()

	_, err := NewClient(context.Background(), Config{
		APIURL:        srv.URL,
		AdminPassword: "admin",
	})
	assertErrorContains(t, err, "no token")
}

func TestNewClient_AdminLoginBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	_, err := NewClient(context.Background(), Config{
		APIURL:        srv.URL,
		AdminPassword: "admin",
	})
	assertErrorContains(t, err, "decoding login response")
}

func TestNewClient_BearerTokenPreferredOverAdminPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("admin login should not be called when bearer token is set")
	}))
	defer srv.Close()

	c, err := NewClient(context.Background(), Config{
		APIURL:        srv.URL,
		BearerToken:   "my-token",
		AdminPassword: "admin",
	})
	assertNoError(t, err)
	assertEqual(t, "my-token", c.token)
}

func TestNewClient_TrailingSlashTrimmed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected call")
	}))
	defer srv.Close()

	c, err := NewClient(context.Background(), Config{
		APIURL:      srv.URL + "///",
		BearerToken: "tok",
	})
	assertNoError(t, err)
	assertEqual(t, srv.URL, c.baseURL)
}

func TestDo(t *testing.T) {
	t.Run("sends auth header and unmarshals response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assertEqual(t, "Bearer test-token", r.Header.Get("Authorization"))
			assertEqual(t, "application/json", r.Header.Get("Content-Type"))
			assertSuffix(t, r.URL.Path, "/TestMethod")

			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			assertEqual(t, "bar", body["foo"])

			w.Header().Set("Content-Type", "application/json")
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{"result": "ok"}))
		}))
		defer srv.Close()

		c := &Client{baseURL: srv.URL, token: "test-token", httpClient: srv.Client()}
		var resp map[string]string
		err := c.Do(context.Background(), "TestMethod", map[string]string{"foo": "bar"}, &resp)
		assertNoError(t, err)
		assertEqual(t, "ok", resp["result"])
	})

	t.Run("nil dest skips unmarshal", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		c := &Client{baseURL: srv.URL, token: "tok", httpClient: srv.Client()}
		err := c.Do(context.Background(), "SomeMethod", map[string]string{}, nil)
		assertNoError(t, err)
	})

	t.Run("non-200 returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not found"))
		}))
		defer srv.Close()

		c := &Client{baseURL: srv.URL, token: "tok", httpClient: srv.Client()}
		err := c.Do(context.Background(), "Missing", struct{}{}, nil)
		assertErrorContains(t, err, "returned 404")
	})

	t.Run("bad response JSON returns error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{bad json`))
		}))
		defer srv.Close()

		c := &Client{baseURL: srv.URL, token: "tok", httpClient: srv.Client()}
		var resp map[string]string
		err := c.Do(context.Background(), "BadJSON", struct{}{}, &resp)
		assertErrorContains(t, err, "decoding response")
	})

	t.Run("connection error", func(t *testing.T) {
		c := &Client{baseURL: "http://127.0.0.1:1", token: "tok", httpClient: &http.Client{}}
		err := c.Do(context.Background(), "Fail", struct{}{}, nil)
		assertErrorContains(t, err, "executing request")
	})

	t.Run("context cancellation", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		c := &Client{baseURL: srv.URL, token: "tok", httpClient: srv.Client()}
		err := c.Do(ctx, "Cancelled", struct{}{}, nil)
		assertErrorContains(t, err, "executing request")
	})
}

func TestDoUnary_MarshalError(t *testing.T) {
	c := &Client{baseURL: "http://localhost", token: "tok", httpClient: &http.Client{}}
	err := c.Do(context.Background(), "Test", make(chan int), nil)
	assertErrorContains(t, err, "marshalling request")
}

func TestDoUnary_InvalidURL(t *testing.T) {
	c := &Client{baseURL: "://bad-url", token: "tok", httpClient: &http.Client{}}
	err := c.Do(context.Background(), "Test", struct{}{}, nil)
	assertErrorContains(t, err, "creating request")
}

func TestDoUnary_ReadBodyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{baseURL: srv.URL, token: "tok", httpClient: srv.Client()}
	err := c.Do(context.Background(), "ReadFail", struct{}{}, nil)
	assertErrorContains(t, err, "reading response body")
}

func TestNewClient_EnvVarAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assertNoError(t, json.NewEncoder(w).Encode(adminLoginResponse{IDToken: "env-jwt"}))
	}))
	defer srv.Close()

	t.Setenv(envAPIURL, srv.URL)
	t.Setenv(envAdminPassword, "env-admin")

	c, err := NewClient(context.Background(), Config{})
	assertNoError(t, err)
	assertEqual(t, "env-jwt", c.token)
}

func assertEqual(t *testing.T, expected, actual string) {
	t.Helper()
	if expected != actual {
		t.Errorf("expected %q, got %q", expected, actual)
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertErrorContains(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !contains(err.Error(), substr) {
		t.Errorf("expected error containing %q, got %q", substr, err.Error())
	}
}

func assertSuffix(t *testing.T, s, suffix string) {
	t.Helper()
	if len(s) < len(suffix) || s[len(s)-len(suffix):] != suffix {
		t.Errorf("expected %q to end with %q", s, suffix)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsInner(s, substr))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
