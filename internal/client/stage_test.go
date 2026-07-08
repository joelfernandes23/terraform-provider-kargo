package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"testing"
)

func TestGetStage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		fixture, err := os.ReadFile("testdata/stage_response.json")
		assertNoError(t, err)

		c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			assertSuffix(t, r.URL.Path, "/GetStage")

			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			assertEqual(t, "demo", body["project"])
			assertEqual(t, "staging", body["name"])
			assertEqual(t, "RAW_FORMAT_JSON", body["format"])
			if len(body) != 3 {
				t.Errorf("expected exactly project/name/format request fields, got %v", body)
			}

			w.Header().Set("Content-Type", "application/json")
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{
				"raw": base64.StdEncoding.EncodeToString(fixture),
			}))
		})
		defer srv.Close()

		s, err := c.GetStage(context.Background(), "demo", "staging")
		assertNoError(t, err)
		assertEqual(t, "staging", s.Metadata.Name)
		assertEqual(t, "demo", s.Metadata.Namespace)
		assertEqual(t, "eu-west", s.Spec.Shard)
		assertEqual(t, "Warehouse", s.Spec.RequestedFreight[0].Origin.Kind)
		if got := s.Spec.RequestedFreight[0].Sources.Stages; len(got) != 1 || got[0] != "test" {
			t.Errorf("expected sources.stages [test], got %#v", got)
		}
		assertEqual(t, "git-clone", s.Spec.PromotionTemplate.Spec.Steps[0].Uses)

		var cfg map[string]any
		assertNoError(t, json.Unmarshal(s.Spec.PromotionTemplate.Spec.Steps[0].Config, &cfg))
		repoURL, ok := cfg["repoURL"].(string)
		if !ok || repoURL != "https://github.com/example/repo.git" {
			t.Errorf("expected inline config repoURL, got %#v", cfg["repoURL"])
		}

		// Status decode: the raw path carries standard k8s JSON, and unmodeled
		// fields (health.config/output, verificationHistory, per-condition
		// observedGeneration) in the fixture must be silently ignored.
		if s.Status.Health == nil {
			t.Fatal("expected status.health to be populated")
		}
		assertEqual(t, "Healthy", s.Status.Health.Status)
		if issues := s.Status.Health.Issues; len(issues) != 1 || issues[0] != "example issue" {
			t.Errorf("expected health issues [example issue], got %#v", issues)
		}
		assertEqual(t, "1/1 Fulfilled", s.Status.FreightSummary)
		if !s.Status.AutoPromotionEnabled {
			t.Error("expected autoPromotionEnabled true")
		}
		assertEqual(t, "refresh-1", s.Status.LastHandledRefresh)
		if s.Status.ObservedGeneration != 0 {
			t.Errorf("expected zero observedGeneration (top-level field absent in fixture), got %d", s.Status.ObservedGeneration)
		}
		if len(s.Status.Conditions) != 1 {
			t.Fatalf("expected 1 condition, got %d", len(s.Status.Conditions))
		}
		assertEqual(t, "Ready", s.Status.Conditions[0].Type)
		assertEqual(t, "Healthy", s.Status.Conditions[0].Reason)
		if len(s.Status.FreightHistory) != 1 {
			t.Fatalf("expected 1 freight collection, got %d", len(s.Status.FreightHistory))
		}
		assertEqual(t, "abc123def456", s.Status.FreightHistory[0].ID)
		freight, ok := s.Status.FreightHistory[0].Items["Warehouse/app"]
		if !ok {
			t.Fatalf("expected freight item keyed Warehouse/app, got %#v", s.Status.FreightHistory[0].Items)
		}
		assertEqual(t, "freight-1", freight.Name)
		assertEqual(t, "deadbeef", freight.Commits[0].ID)
		assertEqual(t, "1.2.3", freight.Images[0].Tag)
		assertEqual(t, "1.2.3", freight.Charts[0].Version)
		assertEqual(t, "v1", freight.Artifacts[0].Version)
		if s.Status.LastPromotion == nil {
			t.Fatal("expected status.lastPromotion to be populated")
		}
		assertEqual(t, "staging.01hxyz.abcdef", s.Status.LastPromotion.Name)
		assertEqual(t, "2026-07-02T09:30:00Z", s.Status.LastPromotion.FinishedAt)
		if s.Status.LastPromotion.Status == nil {
			t.Fatal("expected lastPromotion.status to be populated")
		}
		assertEqual(t, "Succeeded", s.Status.LastPromotion.Status.Phase)
	})

	t.Run("sparse_status", func(t *testing.T) {
		// Fresh, never-promoted stage: status carries only conditions + freightSummary.
		manifest := []byte(`{"metadata":{"name":"fresh","namespace":"demo"},"spec":{"requestedFreight":[{"origin":{"kind":"Warehouse","name":"app"},"sources":{"direct":true}}]},"status":{"conditions":[{"type":"Ready","status":"False","reason":"NoFreight","lastTransitionTime":"2026-07-05T13:23:57Z"}],"freightSummary":"0/1 Fulfilled"}}`)
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{
				"raw": base64.StdEncoding.EncodeToString(manifest),
			}))
		})
		defer srv.Close()

		s, err := c.GetStage(context.Background(), "demo", "fresh")
		assertNoError(t, err)
		if s.Status.Health != nil {
			t.Errorf("expected nil health on fresh stage, got %#v", s.Status.Health)
		}
		if len(s.Status.FreightHistory) != 0 {
			t.Errorf("expected empty freight history, got %#v", s.Status.FreightHistory)
		}
		if s.Status.LastPromotion != nil {
			t.Errorf("expected nil lastPromotion, got %#v", s.Status.LastPromotion)
		}
		if s.Status.ObservedGeneration != 0 {
			t.Errorf("expected zero observedGeneration, got %d", s.Status.ObservedGeneration)
		}
		if s.Status.AutoPromotionEnabled {
			t.Error("expected autoPromotionEnabled false")
		}
		assertEqual(t, "0/1 Fulfilled", s.Status.FreightSummary)
		if len(s.Status.Conditions) != 1 {
			t.Fatalf("expected 1 condition, got %d", len(s.Status.Conditions))
		}
		assertEqual(t, "NoFreight", s.Status.Conditions[0].Reason)
	})

	t.Run("deleting", func(t *testing.T) {
		manifest := []byte(`{"metadata":{"name":"dying","namespace":"demo","deletionTimestamp":"2026-07-04T00:00:00Z"}}`)
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{
				"raw": base64.StdEncoding.EncodeToString(manifest),
			}))
		})
		defer srv.Close()

		s, err := c.GetStage(context.Background(), "demo", "dying")
		assertNoError(t, err)
		if s != nil {
			t.Error("expected nil for stage being deleted")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"not_found","message":"stage not found"}`))
		})
		defer srv.Close()

		_, err := c.GetStage(context.Background(), "demo", "missing")
		assertErrorContains(t, err, "getting stage")
		if !IsNotFound(err) {
			t.Errorf("expected IsNotFound to recognize wrapped error: %v", err)
		}
	})

	t.Run("bad_manifest", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{
				"raw": base64.StdEncoding.EncodeToString([]byte(`{not json`)),
			}))
		})
		defer srv.Close()

		_, err := c.GetStage(context.Background(), "demo", "corrupt")
		assertErrorContains(t, err, "decoding stage")
	})

	t.Run("do_error", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":"internal","message":"boom"}`))
		})
		defer srv.Close()

		_, err := c.GetStage(context.Background(), "demo", "broken")
		assertErrorContains(t, err, "getting stage")
	})
}

func TestGetStageDecodesFixture(t *testing.T) {
	fixture, err := os.ReadFile("testdata/stage_response.json")
	assertNoError(t, err)

	c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assertNoError(t, json.NewEncoder(w).Encode(map[string]string{
			"raw": base64.StdEncoding.EncodeToString(fixture),
		}))
	})
	defer srv.Close()

	s, err := c.GetStage(context.Background(), "demo", "staging")
	assertNoError(t, err)

	// Navigate the raw fixture to the original step configs so the decoded
	// json.RawMessage can be byte-compared after normalization.
	var raw struct {
		Spec struct {
			PromotionTemplate struct {
				Spec struct {
					Steps []struct {
						Config json.RawMessage `json:"config"`
					} `json:"steps"`
				} `json:"spec"`
			} `json:"promotionTemplate"`
		} `json:"spec"`
	}
	assertNoError(t, json.Unmarshal(fixture, &raw))

	steps := s.Spec.PromotionTemplate.Spec.Steps
	if len(steps) != 2 {
		t.Fatalf("expected 2 promotion steps, got %d", len(steps))
	}

	for i, step := range steps {
		want := normalizeJSON(t, raw.Spec.PromotionTemplate.Spec.Steps[i].Config)
		got := normalizeJSON(t, step.Config)
		if !bytes.Equal(want, got) {
			t.Errorf("step %d config did not round-trip: want %s, got %s", i, want, got)
		}
	}

	// The nested object/array (checkout list) must survive the round-trip.
	var clone map[string]any
	assertNoError(t, json.Unmarshal(steps[0].Config, &clone))
	checkout, ok := clone["checkout"].([]any)
	if !ok || len(checkout) != 1 {
		t.Fatalf("expected checkout list with 1 entry, got %#v", clone["checkout"])
	}
	entry, ok := checkout[0].(map[string]any)
	if !ok {
		t.Fatalf("expected checkout entry object, got %#v", checkout[0])
	}
	assertEqual(t, "main", entry["branch"].(string))
	assertEqual(t, "./src", entry["path"].(string))
}

// normalizeJSON re-marshals arbitrary JSON through map decoding so two
// semantically equal documents become byte-comparable (sorted keys).
func normalizeJSON(t *testing.T, data json.RawMessage) []byte {
	t.Helper()
	var v any
	assertNoError(t, json.Unmarshal(data, &v))
	out, err := json.Marshal(v)
	assertNoError(t, err)
	return out
}

func testStageSpec() StageSpec {
	return StageSpec{
		Shard: "eu-west",
		RequestedFreight: []FreightRequest{{
			Origin:  FreightOrigin{Kind: "Warehouse", Name: "app"},
			Sources: FreightSources{Stages: []string{"test"}},
		}},
		PromotionTemplate: &PromotionTemplate{Spec: PromotionTemplateSpec{Steps: []PromotionStep{{
			Uses:   "git-clone",
			As:     "clone",
			Config: json.RawMessage(`{"repoURL":"https://github.com/example/repo.git","checkout":[{"branch":"main","path":"./src"}]}`),
		}}}},
	}
}

func assertStageManifest(t *testing.T, r *http.Request, wantConfig json.RawMessage) {
	t.Helper()

	var body map[string]string
	assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
	manifest, err := base64.StdEncoding.DecodeString(body["manifest"])
	assertNoError(t, err)

	var obj map[string]any
	assertNoError(t, json.Unmarshal(manifest, &obj))
	assertEqual(t, "Stage", obj["kind"].(string))
	assertEqual(t, "kargo.akuity.io/v1alpha1", obj["apiVersion"].(string))
	metadata := obj["metadata"].(map[string]any)
	assertEqual(t, "staging", metadata["name"].(string))
	assertEqual(t, "demo", metadata["namespace"].(string))

	spec := obj["spec"].(map[string]any)
	freight := spec["requestedFreight"].([]any)
	origin := freight[0].(map[string]any)["origin"].(map[string]any)
	assertEqual(t, "app", origin["name"].(string))

	steps := spec["promotionTemplate"].(map[string]any)["spec"].(map[string]any)["steps"].([]any)
	gotConfig, err := json.Marshal(steps[0].(map[string]any)["config"])
	assertNoError(t, err)
	if !bytes.Equal(normalizeJSON(t, wantConfig), normalizeJSON(t, gotConfig)) {
		t.Errorf("manifest step config mismatch: want %s, got %s", wantConfig, gotConfig)
	}
}

func rawStageFixtureHandler(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	fixture, err := os.ReadFile("testdata/stage_response.json")
	assertNoError(t, err)
	assertNoError(t, json.NewEncoder(w).Encode(map[string]string{
		"raw": base64.StdEncoding.EncodeToString(fixture),
	}))
}

func TestCreateStage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		spec := testStageSpec()
		callCount := 0
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")

			if callCount == 1 {
				assertSuffix(t, r.URL.Path, "/CreateResource")
				assertStageManifest(t, r, spec.PromotionTemplate.Spec.Steps[0].Config)
				assertNoError(t, json.NewEncoder(w).Encode(resourceResultResponse{
					Results: []struct {
						CreatedResourceManifest string `json:"createdResourceManifest,omitempty"`
						UpdatedResourceManifest string `json:"updatedResourceManifest,omitempty"`
						Error                   string `json:"error,omitempty"`
					}{{CreatedResourceManifest: "dGVzdA=="}},
				}))
				return
			}

			assertSuffix(t, r.URL.Path, "/GetStage")
			rawStageFixtureHandler(t, w)
		})
		defer srv.Close()

		s, err := c.CreateStage(context.Background(), "demo", "staging", spec)
		assertNoError(t, err)
		if s == nil {
			t.Fatal("expected non-nil stage after create")
		}
		assertEqual(t, "staging", s.Metadata.Name)
		assertEqual(t, "demo", s.Metadata.Namespace)
	})

	t.Run("result_error", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"error":"stage already exists"}]}`))
		})
		defer srv.Close()

		_, err := c.CreateStage(context.Background(), "demo", "staging", testStageSpec())
		assertErrorContains(t, err, "creating stage")
		assertErrorContains(t, err, "stage already exists")
	})

	t.Run("no_results", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[]}`))
		})
		defer srv.Close()

		_, err := c.CreateStage(context.Background(), "demo", "staging", testStageSpec())
		assertErrorContains(t, err, "no result returned")
	})

	t.Run("marshal_error", func(t *testing.T) {
		spec := testStageSpec()
		spec.PromotionTemplate.Spec.Steps[0].Config = json.RawMessage(`{invalid`)

		c, srv := testClientWithServer(t, func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("server must not be called when manifest marshaling fails")
		})
		defer srv.Close()

		_, err := c.CreateStage(context.Background(), "demo", "staging", spec)
		assertErrorContains(t, err, "manifest")
	})

	t.Run("do_error", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":"internal","message":"boom"}`))
		})
		defer srv.Close()

		_, err := c.CreateStage(context.Background(), "demo", "staging", testStageSpec())
		assertErrorContains(t, err, "creating stage")
	})
}

func TestUpdateStage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		spec := testStageSpec()
		callCount := 0
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")

			if callCount == 1 {
				assertSuffix(t, r.URL.Path, "/UpdateResource")
				assertStageManifest(t, r, spec.PromotionTemplate.Spec.Steps[0].Config)
				assertNoError(t, json.NewEncoder(w).Encode(resourceResultResponse{
					Results: []struct {
						CreatedResourceManifest string `json:"createdResourceManifest,omitempty"`
						UpdatedResourceManifest string `json:"updatedResourceManifest,omitempty"`
						Error                   string `json:"error,omitempty"`
					}{{UpdatedResourceManifest: "dGVzdA=="}},
				}))
				return
			}

			assertSuffix(t, r.URL.Path, "/GetStage")
			rawStageFixtureHandler(t, w)
		})
		defer srv.Close()

		s, err := c.UpdateStage(context.Background(), "demo", "staging", spec)
		assertNoError(t, err)
		if s == nil {
			t.Fatal("expected non-nil stage after update")
		}
		assertEqual(t, "staging", s.Metadata.Name)
	})

	t.Run("result_error", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"error":"webhook rejected"}]}`))
		})
		defer srv.Close()

		_, err := c.UpdateStage(context.Background(), "demo", "staging", testStageSpec())
		assertErrorContains(t, err, "updating stage")
		assertErrorContains(t, err, "webhook rejected")
	})

	t.Run("marshal_error", func(t *testing.T) {
		spec := testStageSpec()
		spec.PromotionTemplate.Spec.Steps[0].Config = json.RawMessage(`{invalid`)

		c, srv := testClientWithServer(t, func(_ http.ResponseWriter, _ *http.Request) {
			t.Error("server must not be called when manifest marshaling fails")
		})
		defer srv.Close()

		_, err := c.UpdateStage(context.Background(), "demo", "staging", spec)
		assertErrorContains(t, err, "manifest")
	})

	t.Run("do_error", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":"internal","message":"boom"}`))
		})
		defer srv.Close()

		_, err := c.UpdateStage(context.Background(), "demo", "staging", testStageSpec())
		assertErrorContains(t, err, "updating stage")
	})
}

func TestDeleteStage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
			assertSuffix(t, r.URL.Path, "/DeleteStage")

			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			assertEqual(t, "demo", body["project"])
			assertEqual(t, "staging", body["name"])

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		})
		defer srv.Close()

		assertNoError(t, c.DeleteStage(context.Background(), "demo", "staging"))
	})

	t.Run("not_found", func(t *testing.T) {
		c, srv := testClientWithServer(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"not_found","message":"stage not found"}`))
		})
		defer srv.Close()

		err := c.DeleteStage(context.Background(), "demo", "gone")
		assertErrorContains(t, err, "deleting stage")
		if !IsNotFound(err) {
			t.Errorf("expected IsNotFound to recognize wrapped error: %v", err)
		}
	})
}
