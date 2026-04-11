package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testClientWithServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c := &Client{baseURL: srv.URL, token: "test-token", httpClient: srv.Client()}
	return c, srv
}

func TestCreateProject(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		callCount := 0
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")

			if callCount == 1 {
				assertSuffix(t, r.URL.Path, "/CreateResource")
				assertNoError(t, json.NewEncoder(w).Encode(createResourceResponse{
					Results: []struct {
						CreatedResourceManifest string `json:"createdResourceManifest"`
					}{{CreatedResourceManifest: "dGVzdA=="}},
				}))
				return
			}

			assertSuffix(t, r.URL.Path, "/GetProject")
			assertNoError(t, json.NewEncoder(w).Encode(getProjectResponse{
				Project: Project{
					Metadata: ProjectMetadata{Name: "test-proj", UID: "uid-123"},
				},
			}))
		})
		defer srv.Close()

		p, err := c.CreateProject(context.Background(), "test-proj")
		assertNoError(t, err)
		assertEqual(t, "test-proj", p.Metadata.Name)
		assertEqual(t, "uid-123", p.Metadata.UID)
	})

	t.Run("create fails", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"already_exists","message":"project exists"}`))
		})
		defer srv.Close()

		_, err := c.CreateProject(context.Background(), "dup")
		assertErrorContains(t, err, "creating project")
	})
}

func TestGetProject(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			assertNoError(t, json.NewEncoder(w).Encode(getProjectResponse{
				Project: Project{
					Metadata: ProjectMetadata{Name: "my-proj", UID: "uid-456", ResourceVersion: "100"},
					Status: ProjectStatus{
						Conditions: []ProjectCondition{{Type: "Ready", Status: "True"}},
					},
				},
			}))
		})
		defer srv.Close()

		p, err := c.GetProject(context.Background(), "my-proj")
		assertNoError(t, err)
		assertEqual(t, "my-proj", p.Metadata.Name)
		assertEqual(t, "uid-456", p.Metadata.UID)
		assertEqual(t, "True", p.Status.Conditions[0].Status)
	})

	t.Run("being deleted returns nil", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// deletionTimestamp is a non-nil empty object when set
			_, _ = w.Write([]byte(`{"project":{"metadata":{"name":"dying","deletionTimestamp":{}}}}`))
		})
		defer srv.Close()

		p, err := c.GetProject(context.Background(), "dying")
		assertNoError(t, err)
		if p != nil {
			t.Error("expected nil for project being deleted")
		}
	})

	t.Run("API error", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"not_found","message":"not found"}`))
		})
		defer srv.Close()

		_, err := c.GetProject(context.Background(), "missing")
		assertErrorContains(t, err, "getting project")
	})
}

func TestDeleteProject(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			assertSuffix(t, r.URL.Path, "/DeleteProject")

			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			assertEqual(t, "del-proj", body["name"])

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		})
		defer srv.Close()

		err := c.DeleteProject(context.Background(), "del-proj")
		assertNoError(t, err)
	})

	t.Run("API error", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":"internal","message":"boom"}`))
		})
		defer srv.Close()

		err := c.DeleteProject(context.Background(), "fail")
		assertErrorContains(t, err, "deleting project")
	})
}
