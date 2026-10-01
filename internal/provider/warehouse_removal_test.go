package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccWarehouseResource_removePathFilters(t *testing.T) {
	srv := testWarehouseServer(t)
	defer srv.Close()
	initial := testWarehouseResourceCompleteConfig(srv.URL, "test-project", "filters")
	removed := strings.Join(filterWarehouseConfigLines(strings.Split(initial, "\n"), "include_paths", "exclude_paths"), "\n")
	empty := strings.Replace(removed, "git {", "git {\ninclude_paths = []\nexclude_paths = []", 1)
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{Config: initial},
			{Config: removed, Check: func(_ *terraform.State) error {
				srv.mu.RLock()
				defer srv.mu.RUnlock()
				warehouse := srv.warehouses["test-project/filters"]
				spec := warehouse["spec"].(map[string]any)
				subs := spec["subscriptions"].([]any)
				git := subs[1].(map[string]any)["git"].(map[string]any)
				for _, field := range []string{"includePaths", "excludePaths"} {
					if value, ok := git[field]; ok {
						return fmt.Errorf("removed %s still present in API: %v", field, value)
					}
				}
				return nil
			}},
			{Config: removed, PlanOnly: true},
			{Config: empty, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.1.git.include_paths.#", "0"),
				resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.1.git.exclude_paths.#", "0"),
			)},
			{Config: empty, PlanOnly: true},
			{Config: initial},
			{Config: initial, PreConfig: func() {
				srv.mu.Lock()
				defer srv.mu.Unlock()
				spec := srv.warehouses["test-project/filters"]["spec"].(map[string]any)
				git := spec["subscriptions"].([]any)[1].(map[string]any)["git"].(map[string]any)
				git["includePaths"] = []any{"external/**"}
			}, Check: resource.TestCheckResourceAttr("kargo_warehouse.test", "subscription.1.git.include_paths.0", "apps/**")},
			{Config: initial, PlanOnly: true},
		},
	})
}

func filterWarehouseConfigLines(lines []string, attributes ...string) []string {
	var result []string
	for _, line := range lines {
		remove := false
		for _, attribute := range attributes {
			if strings.HasPrefix(strings.TrimSpace(line), attribute+" ") {
				remove = true
			}
		}
		if !remove {
			result = append(result, line)
		}
	}
	return result
}
