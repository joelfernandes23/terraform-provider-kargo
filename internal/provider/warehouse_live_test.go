package provider

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

func TestAccWarehouseResource_live(t *testing.T) {
	if os.Getenv("KARGO_LIVE_TEST") != "1" {
		t.Skip("set KARGO_LIVE_TEST=1 to test local Kargo")
	}
	endpoint, err := url.Parse(os.Getenv("KARGO_API_URL"))
	if err != nil || (endpoint.Hostname() != "localhost" && endpoint.Hostname() != "127.0.0.1" && endpoint.Hostname() != "::1") {
		t.Fatal("live Warehouse tests require a loopback API URL")
	}
	project := fmt.Sprintf("tf-warehouse-acc-%d", time.Now().UnixNano())
	api, err := client.NewClient(context.Background(), client.Config{})
	if err != nil {
		t.Fatal(err)
	}
	config := func(limit int) string {
		return fmt.Sprintf(`
provider "kargo" {}
resource "kargo_project" "live" { name = %q }
resource "kargo_warehouse" "live" {
  project = kargo_project.live.name
  name = "source"
  interval = "10m"
  subscription {
    git {
      repo_url = "https://github.com/akuity/kargo.git"
      branch = "main"
      commit_selection_strategy = "NewestFromBranch"
      include_paths = ["docs/**"]
      discovery_limit = %d
      strict_semvers = false
      exclude_paths = ["docs/assets/**"]
      blobless = false
      since = "2026-01-01T00:00:00Z"
      insecure_skip_tls_verify = false
    }
  }
  subscription {
    image {
      repo_url = "docker.io/library/nginx"
      tag_selection_strategy = "SemVer"
      semver_constraint = ">= 1.0.0"
      platform = "linux/arm64"
      allow_tags_regexes = ["^1\\."]
      ignore_tags_regexes = ["-alpine$"]
      cache_by_tag = true
      discovery_limit = 5
      strict_semvers = false
      insecure_skip_tls_verify = false
    }
  }
  subscription {
    chart {
      repo_url = "https://stefanprodan.github.io/podinfo"
      name = "podinfo"
      semver_constraint = ">= 6.0.0"
      discovery_limit = 4
      insecure_skip_tls_verify = false
    }
  }
}
`, project, limit)
	}
	removed := strings.Join(filterWarehouseConfigLines(strings.Split(config(9), "\n"), "include_paths", "exclude_paths", "allow_tags_regexes", "ignore_tags_regexes", "since", "semver_constraint", "platform", "branch", "strict_semvers", "cache_by_tag", "discovery_limit", "commit_selection_strategy", "tag_selection_strategy", "interval"), "\n")
	checkRemoved := func(_ *terraform.State) error {
		warehouse, err := api.GetWarehouse(context.Background(), project, "source")
		if err != nil {
			return err
		}
		if warehouse == nil || len(warehouse.Spec.Subscriptions) != 3 {
			return fmt.Errorf("expected three live subscriptions")
		}
		git, image := warehouse.Spec.Subscriptions[0].Git, warehouse.Spec.Subscriptions[1].Image
		if len(git.IncludePaths)+len(git.ExcludePaths)+len(image.AllowTagsRegexes)+len(image.IgnoreTagsRegexes) != 0 {
			return fmt.Errorf("removed filters remain in live Kargo")
		}
		if git.Since != "" || git.Branch != "" || image.Constraint != "" || image.Platform != "" || warehouse.Spec.Subscriptions[2].Chart.SemverConstraint != "" {
			return fmt.Errorf("removed string settings remain in live Kargo")
		}
		if git.DiscoveryLimit == nil || *git.DiscoveryLimit != 20 || image.DiscoveryLimit == nil || *image.DiscoveryLimit != 20 || git.StrictSemvers == nil || !*git.StrictSemvers || image.StrictSemvers == nil || !*image.StrictSemvers || (image.CacheByTag != nil && *image.CacheByTag) {
			return fmt.Errorf("removed settings did not return to Kargo defaults")
		}
		interval, err := time.ParseDuration(warehouse.Spec.Interval)
		if err != nil || interval != 5*time.Minute {
			return fmt.Errorf("removed interval did not reset: %q", warehouse.Spec.Interval)
		}
		return nil
	}
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: config(7), Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("kargo_warehouse.live", "subscription.0.git.discovery_limit", "7"),
				resource.TestCheckResourceAttr("kargo_warehouse.live", "subscription.0.git.commit_selection_strategy", "NewestFromBranch"),
				resource.TestCheckResourceAttr("kargo_warehouse.live", "subscription.0.git.include_paths.0", "docs/**"),
				resource.TestCheckResourceAttr("kargo_warehouse.live", "subscription.0.git.strict_semvers", "false"),
				resource.TestCheckResourceAttr("kargo_warehouse.live", "subscription.0.git.since", "2026-01-01T00:00:00Z"),
				resource.TestCheckResourceAttr("kargo_warehouse.live", "subscription.1.image.cache_by_tag", "true"),
				resource.TestCheckResourceAttr("kargo_warehouse.live", "subscription.1.image.discovery_limit", "5"),
				resource.TestCheckResourceAttr("kargo_warehouse.live", "subscription.2.chart.discovery_limit", "4"),
			)},
			{Config: config(9), Check: resource.TestCheckResourceAttr("kargo_warehouse.live", "subscription.0.git.discovery_limit", "9")},
			{
				ResourceName: "kargo_warehouse.live", ImportState: true, ImportStateVerify: true,
				// Kargo canonicalizes 10m to 10m0s. Check duration equivalence
				// explicitly because import verification compares plain strings.
				ImportStateVerifyIgnore: []string{"interval"},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected one imported Warehouse, got %d", len(states))
					}
					interval, err := time.ParseDuration(states[0].Attributes["interval"])
					if err != nil || interval != 10*time.Minute {
						return fmt.Errorf("imported interval differs: %q", states[0].Attributes["interval"])
					}
					return nil
				},
			},
			{Config: config(9), PlanOnly: true},
			{Config: removed, Check: checkRemoved},
			{Config: removed, PlanOnly: true},
			{Config: config(9)},
			{Config: config(9), PreConfig: func() {
				warehouse, err := api.GetWarehouse(context.Background(), project, "source")
				if err != nil {
					t.Fatal(err)
				}
				warehouse.Spec.Subscriptions[0].Git.IncludePaths = []string{"external/**"}
				if _, err := api.UpdateWarehouse(context.Background(), project, "source", warehouse.Spec); err != nil {
					t.Fatal(err)
				}
			}, Check: func(_ *terraform.State) error {
				warehouse, err := api.GetWarehouse(context.Background(), project, "source")
				if err != nil {
					return err
				}
				paths := warehouse.Spec.Subscriptions[0].Git.IncludePaths
				if len(paths) != 1 || paths[0] != "docs/**" {
					return fmt.Errorf("live drift was not repaired: %v", paths)
				}
				return nil
			}},
			{Config: config(9), PlanOnly: true},
		},
	})
}
