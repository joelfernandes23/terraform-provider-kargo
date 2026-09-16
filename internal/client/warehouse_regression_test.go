package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestGenericWarehouseSubscriptionManifest(t *testing.T) {
	limit := int64(17)
	sub := WarehouseSubscription{Name: "packages", Generic: &GenericSubscription{
		Type: "npm", Config: json.RawMessage(`{"package":"example"}`), DiscoveryLimit: &limit,
	}}
	raw, err := json.Marshal(sub)
	assertNoError(t, err)
	var envelope map[string]json.RawMessage
	assertNoError(t, json.Unmarshal(raw, &envelope))
	if len(envelope) != 1 {
		t.Fatalf("Kargo requires exactly one subscription key: %s", raw)
	}
	var fields map[string]json.RawMessage
	assertNoError(t, json.Unmarshal(envelope["npm"], &fields))
	if string(fields["name"]) != `"packages"` || string(fields["config"]) != `{"package":"example"}` || string(fields["discoveryLimit"]) != "17" {
		t.Fatalf("incorrect generic subscription shape: %s", raw)
	}
	var decoded WarehouseSubscription
	assertNoError(t, json.Unmarshal(raw, &decoded))
	if decoded.Name != "packages" || decoded.Generic == nil || decoded.Generic.DiscoveryLimit == nil || *decoded.Generic.DiscoveryLimit != 17 || string(decoded.Generic.Config) != `{"package":"example"}` {
		t.Fatalf("generic subscription round trip lost fields: %#v", decoded)
	}
	assertNoError(t, json.Unmarshal([]byte(`{"git":{"repoURL":"https://example.com/repo"}}`), &decoded))
	if decoded.Generic != nil || decoded.Name != "" || decoded.Git == nil {
		t.Fatalf("decoding reused value retained stale subscription: %#v", decoded)
	}
}

func TestWarehouseUpdateConflictRetries(t *testing.T) {
	for _, tc := range []struct {
		name      string
		message   string
		failures  int
		wantCalls int
		wantError bool
	}{
		{"transient conflict", "the object has been modified", 2, 3, false},
		{"persistent conflict", "the object has been modified", 5, 3, true},
		{"unrelated conflict", "resource exists", 5, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls <= tc.failures {
					w.WriteHeader(http.StatusConflict)
					assertNoError(t, json.NewEncoder(w).Encode(map[string]string{"code": "already_exists", "message": tc.message}))
					return
				}
				_, _ = w.Write([]byte(`{"results":[{}]}`))
			})
			defer srv.Close()
			var response resourceResultResponse
			err := c.updateWarehouseManifest(context.Background(), "manifest", &response)
			if (err != nil) != tc.wantError || calls != tc.wantCalls {
				t.Fatalf("calls=%d error=%v, want calls=%d error=%t", calls, err, tc.wantCalls, tc.wantError)
			}
		})
	}
}

func TestGetWarehouseRejectsMalformedRawResponse(t *testing.T) {
	c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		var request map[string]string
		assertNoError(t, json.NewDecoder(r.Body).Decode(&request))
		assertEqual(t, "RAW_FORMAT_JSON", request["format"])
		assertNoError(t, json.NewEncoder(w).Encode(getWarehouseResponse{Raw: []byte("{")}))
	})
	defer srv.Close()
	_, err := c.GetWarehouse(context.Background(), "project", "name")
	assertErrorContains(t, err, "decoding warehouse")
}

func TestWarehouseSubscriptionRejectsInvalidShapes(t *testing.T) {
	for _, input := range []string{`null`, `[]`, `{}`, `{"git":null}`, `{"npm":42}`, `{"git":{},"image":{}}`, `{"git":{"repoURL":42}}`, `{"npm":{"discoveryLimit":"invalid"}}`} {
		t.Run(input, func(t *testing.T) {
			var sub WarehouseSubscription
			if err := json.Unmarshal([]byte(input), &sub); err == nil {
				t.Fatalf("expected malformed subscription to fail: %s", input)
			}
		})
	}
}

func TestWarehouseUpdateConflictCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c, srv := testClientWithServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"code":"already_exists","message":"the object has been modified"}`))
	})
	defer srv.Close()
	// Cancel during the backoff, after the first RPC has returned its conflict.
	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()
	var response resourceResultResponse
	if err := c.updateWarehouseManifest(ctx, "manifest", &response); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
