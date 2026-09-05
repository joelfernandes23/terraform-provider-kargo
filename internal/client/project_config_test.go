package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
)

func projectConfigFixture(deleting bool) []byte {
	deletion := ""
	if deleting {
		deletion = `,"deletionTimestamp":"2026-01-01T00:00:00Z"`
	}
	return []byte(`{
  "metadata":{"name":"demo","namespace":"demo"` + deletion + `},
  "spec":{
    "promotionPolicies":[{"stageSelector":{"name":"glob:dev-*","matchLabels":{"team":"platform"},"matchExpressions":[{"key":"tier","operator":"In","values":["one"]}]},"autoPromotionEnabled":true}],
    "webhookReceivers":[{"name":"github","github":{"secretRef":{"name":"github-secret"}}}]
  },
  "status":{"webhookReceivers":[{"name":"github","path":"/hook","url":"https://example.test/hook"}]}
}`)
}

func writeRawProjectConfig(t *testing.T, w http.ResponseWriter, raw []byte) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	assertNoError(t, json.NewEncoder(w).Encode(map[string]string{"raw": base64.StdEncoding.EncodeToString(raw)}))
}

func TestGetProjectConfig(t *testing.T) {
	t.Run("complete manifest", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			assertSuffix(t, r.URL.Path, "/GetProjectConfig")
			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			assertEqual(t, "demo", body["project"])
			assertEqual(t, "RAW_FORMAT_JSON", body["format"])
			writeRawProjectConfig(t, w, projectConfigFixture(false))
		})
		defer srv.Close()

		config, err := c.GetProjectConfig(context.Background(), "demo")
		assertNoError(t, err)
		assertEqual(t, "demo", config.Metadata.Name)
		assertEqual(t, "glob:dev-*", config.Spec.PromotionPolicies[0].StageSelector.Name)
		assertEqual(t, "github-secret", config.Spec.WebhookReceivers[0].GitHub.SecretRef.Name)
		assertEqual(t, "/hook", config.Status.WebhookReceivers[0].Path)
	})

	t.Run("terminating", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeRawProjectConfig(t, w, projectConfigFixture(true))
		})
		defer srv.Close()
		config, err := c.GetProjectConfig(context.Background(), "demo")
		assertNoError(t, err)
		if config != nil {
			t.Fatal("expected terminating ProjectConfig to be absent")
		}
	})

	t.Run("invalid raw manifest", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			writeRawProjectConfig(t, w, []byte(`{invalid`))
		})
		defer srv.Close()
		_, err := c.GetProjectConfig(context.Background(), "demo")
		assertErrorContains(t, err, "decoding project config")
	})

	t.Run("API error", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"not_found","message":"missing"}`))
		})
		defer srv.Close()
		_, err := c.GetProjectConfig(context.Background(), "demo")
		assertErrorContains(t, err, "getting project config")
		if !IsNotFound(err) {
			t.Fatal("expected wrapped not-found error")
		}
	})
}

func testProjectConfigSpec() ProjectConfigSpec {
	return ProjectConfigSpec{
		PromotionPolicies: []PromotionPolicy{{
			StageSelector: PromotionPolicySelector{Name: "dev"}, AutoPromotionEnabled: true,
		}},
		WebhookReceivers: []WebhookReceiverConfig{{
			Name: "github", GitHub: &WebhookSecretRefConfig{SecretRef: LocalObjectReference{Name: "hook-secret"}},
		}},
	}
}

func assertProjectConfigManifest(t *testing.T, r *http.Request) {
	t.Helper()
	var body map[string]string
	assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
	raw, err := base64.StdEncoding.DecodeString(body["manifest"])
	assertNoError(t, err)
	var manifest projectConfigManifest
	assertNoError(t, json.Unmarshal(raw, &manifest))
	assertEqual(t, "kargo.akuity.io/v1alpha1", manifest.APIVersion)
	assertEqual(t, "ProjectConfig", manifest.Kind)
	assertEqual(t, "demo", manifest.Metadata.Name)
	assertEqual(t, "demo", manifest.Metadata.Namespace)
	assertEqual(t, "dev", manifest.Spec.PromotionPolicies[0].StageSelector.Name)
}

func TestWriteProjectConfig(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     string
		resultBody string
		call       func(*Client) (*ProjectConfig, error)
	}{
		{name: "create", method: "CreateResource", resultBody: `{"results":[{"createdResourceManifest":"dGVzdA=="}]}`, call: func(c *Client) (*ProjectConfig, error) {
			return c.CreateProjectConfig(context.Background(), "demo", testProjectConfigSpec())
		}},
		{name: "update", method: "UpdateResource", resultBody: `{"results":[{"updatedResourceManifest":"dGVzdA=="}]}`, call: func(c *Client) (*ProjectConfig, error) {
			return c.UpdateProjectConfig(context.Background(), "demo", testProjectConfigSpec())
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					assertSuffix(t, r.URL.Path, "/"+tc.method)
					assertProjectConfigManifest(t, r)
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tc.resultBody))
					return
				}
				assertSuffix(t, r.URL.Path, "/GetProjectConfig")
				writeRawProjectConfig(t, w, projectConfigFixture(false))
			})
			defer srv.Close()

			config, err := tc.call(c)
			assertNoError(t, err)
			if config == nil {
				t.Fatal("expected ProjectConfig")
			}
		})
	}

	t.Run("result error", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"error":"already exists"}]}`))
		})
		defer srv.Close()
		_, err := c.CreateProjectConfig(context.Background(), "demo", testProjectConfigSpec())
		assertErrorContains(t, err, "already exists")
	})

	t.Run("API error", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":"internal","message":"boom"}`))
		})
		defer srv.Close()
		_, err := c.UpdateProjectConfig(context.Background(), "demo", testProjectConfigSpec())
		assertErrorContains(t, err, "updating project config")
	})

	t.Run("invalid project", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(_ http.ResponseWriter, _ *http.Request) {
			t.Fatal("server must not be called for an empty project")
		})
		defer srv.Close()
		_, err := c.CreateProjectConfig(context.Background(), "", testProjectConfigSpec())
		assertErrorContains(t, err, "project must not be empty")
	})
}

func TestDeleteProjectConfig(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			assertSuffix(t, r.URL.Path, "/DeleteProjectConfig")
			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			assertEqual(t, "demo", body["project"])
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		})
		defer srv.Close()
		assertNoError(t, c.DeleteProjectConfig(context.Background(), "demo"))
	})

	t.Run("API error", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":"internal","message":"boom"}`))
		})
		defer srv.Close()
		err := c.DeleteProjectConfig(context.Background(), "demo")
		assertErrorContains(t, err, "deleting project config")
	})
}
