package client

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"
)

// This test deliberately does not use the HTTP mock server. Restrict writes to
// a loopback API and a uniquely named project owned by this test.
func TestWarehouseLiveLifecycle(t *testing.T) {
	if os.Getenv("KARGO_LIVE_TEST") != "1" {
		t.Skip("set KARGO_LIVE_TEST=1 and KARGO_API_URL to a local Kargo API")
	}
	endpoint, err := url.Parse(os.Getenv("KARGO_API_URL"))
	assertNoError(t, err)
	if endpoint.Hostname() != "localhost" && endpoint.Hostname() != "127.0.0.1" && endpoint.Hostname() != "::1" {
		t.Fatal("live Warehouse test requires a loopback API URL")
	}
	ctx := context.Background()
	c, err := NewClient(ctx, Config{})
	assertNoError(t, err)
	project := fmt.Sprintf("tf-warehouse-live-%d", time.Now().UnixNano())
	_, err = c.CreateProject(ctx, project)
	assertNoError(t, err)
	t.Cleanup(func() {
		if err := c.DeleteProject(ctx, project); err != nil && !IsNotFound(err) {
			t.Errorf("cleaning up test project %s: %v", project, err)
		}
	})
	limit := int64(7)
	strict := false
	spec := WarehouseSpec{Subscriptions: []WarehouseSubscription{{Git: &GitSubscription{
		RepoURL: "https://github.com/akuity/kargo.git", Branch: "main",
		CommitSelectionStrategy: "NewestFromBranch", IncludePaths: []string{"docs/**"},
		DiscoveryLimit: &limit, StrictSemvers: &strict,
	}}}}
	w, err := c.CreateWarehouse(ctx, project, "source", spec)
	assertNoError(t, err)
	check := func(w *Warehouse, expected int64) {
		t.Helper()
		if w == nil || len(w.Spec.Subscriptions) != 1 || w.Spec.Subscriptions[0].Git == nil {
			t.Fatalf("unexpected live Warehouse: %#v", w)
		}
		git := w.Spec.Subscriptions[0].Git
		if git.CommitSelectionStrategy != "NewestFromBranch" || git.DiscoveryLimit == nil || *git.DiscoveryLimit != expected || git.StrictSemvers == nil || *git.StrictSemvers || len(git.IncludePaths) != 1 || git.IncludePaths[0] != "docs/**" {
			t.Fatalf("live API lost configured Git fields: %#v", git)
		}
	}
	check(w, 7)
	limit = 9
	w, err = c.UpdateWarehouse(ctx, project, "source", spec)
	assertNoError(t, err)
	check(w, 9)
	w, err = c.GetWarehouse(ctx, project, "source")
	assertNoError(t, err)
	check(w, 9)
	assertNoError(t, c.DeleteWarehouse(ctx, project, "source"))
	deadline := time.Now().Add(30 * time.Second)
	for {
		w, err = c.GetWarehouse(ctx, project, "source")
		if IsNotFound(err) || (err == nil && w == nil) {
			break
		}
		if err != nil || time.Now().After(deadline) {
			t.Fatalf("Warehouse still exists after deletion: warehouse=%#v error=%v", w, err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
