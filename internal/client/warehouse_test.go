package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
)

func TestCreateWarehouse(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		callCount := 0
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")

			if callCount == 1 {
				assertSuffix(t, r.URL.Path, "/CreateResource")

				var body map[string]string
				assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
				manifest, err := base64.StdEncoding.DecodeString(body["manifest"])
				assertNoError(t, err)

				var obj map[string]any
				assertNoError(t, json.Unmarshal(manifest, &obj))
				assertEqual(t, "Warehouse", obj["kind"].(string))
				metadata := obj["metadata"].(map[string]any)
				assertEqual(t, "app", metadata["name"].(string))
				assertEqual(t, "demo", metadata["namespace"].(string))

				spec := obj["spec"].(map[string]any)
				subscriptions := spec["subscriptions"].([]any)
				image := subscriptions[0].(map[string]any)["image"].(map[string]any)
				assertEqual(t, "ghcr.io/example/app", image["repoURL"].(string))
				assertEqual(t, "^1.0.0", image["constraint"].(string))
				assertEqual(t, "SemVer", image["imageSelectionStrategy"].(string))
				assertEqual(t, "linux/amd64", image["platform"].(string))

				assertNoError(t, json.NewEncoder(w).Encode(resourceResultResponse{
					Results: []struct {
						CreatedResourceManifest string `json:"createdResourceManifest,omitempty"`
						UpdatedResourceManifest string `json:"updatedResourceManifest,omitempty"`
						Error                   string `json:"error,omitempty"`
					}{{CreatedResourceManifest: "dGVzdA=="}},
				}))
				return
			}

			assertSuffix(t, r.URL.Path, "/GetWarehouse")
			assertNoError(t, json.NewEncoder(w).Encode(getWarehouseResponse{
				Warehouse: Warehouse{
					Metadata: WarehouseMetadata{Name: "app", Namespace: "demo", UID: "uid-123"},
				},
			}))
		})
		defer srv.Close()

		w, err := c.CreateWarehouse(context.Background(), "demo", "app", WarehouseSpec{
			Subscriptions: []WarehouseSubscription{{
				Image: &ImageSubscription{
					RepoURL:                "ghcr.io/example/app",
					Constraint:             "^1.0.0",
					ImageSelectionStrategy: "SemVer",
					Platform:               "linux/amd64",
				},
			}},
		})
		assertNoError(t, err)
		assertEqual(t, "app", w.Metadata.Name)
		assertEqual(t, "demo", w.Metadata.Namespace)
	})

	t.Run("create fails", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"already_exists","message":"warehouse exists"}`))
		})
		defer srv.Close()

		_, err := c.CreateWarehouse(context.Background(), "demo", "dup", WarehouseSpec{})
		assertErrorContains(t, err, "creating warehouse")
	})
}

func TestGetWarehouse(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			assertNoError(t, json.NewEncoder(w).Encode(getWarehouseResponse{
				Warehouse: Warehouse{
					Metadata: WarehouseMetadata{Name: "app", Namespace: "demo", UID: "uid-456"},
					Status: WarehouseStatus{
						Conditions: []WarehouseCondition{{Type: "Ready", Status: "True"}},
					},
				},
			}))
		})
		defer srv.Close()

		w, err := c.GetWarehouse(context.Background(), "demo", "app")
		assertNoError(t, err)
		assertEqual(t, "app", w.Metadata.Name)
		assertEqual(t, "demo", w.Metadata.Namespace)
		assertEqual(t, "True", w.Status.Conditions[0].Status)
	})

	t.Run("being deleted returns nil", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"warehouse":{"metadata":{"name":"dying","deletionTimestamp":"2026-04-28T00:00:00Z"}}}`))
		})
		defer srv.Close()

		w, err := c.GetWarehouse(context.Background(), "demo", "dying")
		assertNoError(t, err)
		if w != nil {
			t.Error("expected nil for warehouse being deleted")
		}
	})

	t.Run("API not found", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"not_found","message":"not found"}`))
		})
		defer srv.Close()

		_, err := c.GetWarehouse(context.Background(), "demo", "missing")
		assertErrorContains(t, err, "getting warehouse")
		if !IsNotFound(err) {
			t.Errorf("expected IsNotFound to recognize wrapped error: %v", err)
		}
	})
}

func TestUpdateWarehouse(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		callCount := 0
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")

			if callCount == 1 {
				assertSuffix(t, r.URL.Path, "/UpdateResource")

				var body map[string]string
				assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
				manifest, err := base64.StdEncoding.DecodeString(body["manifest"])
				assertNoError(t, err)

				var obj map[string]any
				assertNoError(t, json.Unmarshal(manifest, &obj))
				spec := obj["spec"].(map[string]any)
				subscriptions := spec["subscriptions"].([]any)
				image := subscriptions[0].(map[string]any)["image"].(map[string]any)
				assertEqual(t, "^2.0.0", image["constraint"].(string))

				assertNoError(t, json.NewEncoder(w).Encode(resourceResultResponse{
					Results: []struct {
						CreatedResourceManifest string `json:"createdResourceManifest,omitempty"`
						UpdatedResourceManifest string `json:"updatedResourceManifest,omitempty"`
						Error                   string `json:"error,omitempty"`
					}{{UpdatedResourceManifest: "dGVzdA=="}},
				}))
				return
			}

			assertSuffix(t, r.URL.Path, "/GetWarehouse")
			assertNoError(t, json.NewEncoder(w).Encode(getWarehouseResponse{
				Warehouse: Warehouse{
					Metadata: WarehouseMetadata{Name: "app", Namespace: "demo"},
				},
			}))
		})
		defer srv.Close()

		_, err := c.UpdateWarehouse(context.Background(), "demo", "app", WarehouseSpec{
			Subscriptions: []WarehouseSubscription{{
				Image: &ImageSubscription{
					RepoURL:    "ghcr.io/example/app",
					Constraint: "^2.0.0",
				},
			}},
		})
		assertNoError(t, err)
	})

	t.Run("zero results", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			assertNoError(t, json.NewEncoder(w).Encode(resourceResultResponse{}))
		})
		defer srv.Close()

		_, err := c.UpdateWarehouse(context.Background(), "demo", "app", WarehouseSpec{})
		assertErrorContains(t, err, "no result returned")
	})
}

func TestDeleteWarehouse(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			assertSuffix(t, r.URL.Path, "/DeleteWarehouse")

			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			assertEqual(t, "demo", body["project"])
			assertEqual(t, "app", body["name"])

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		})
		defer srv.Close()

		err := c.DeleteWarehouse(context.Background(), "demo", "app")
		assertNoError(t, err)
	})

	t.Run("API error", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":"internal","message":"boom"}`))
		})
		defer srv.Close()

		err := c.DeleteWarehouse(context.Background(), "demo", "fail")
		assertErrorContains(t, err, "deleting warehouse")
	})
}
