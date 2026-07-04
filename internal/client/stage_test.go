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
