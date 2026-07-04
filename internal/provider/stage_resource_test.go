package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/joelfernandes23/terraform-provider-kargo/internal/client"
)

func TestStageResourceMetadata(t *testing.T) {
	r := &StageResource{}
	resp := &fwresource.MetadataResponse{}
	r.Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "kargo"}, resp)

	if resp.TypeName != "kargo_stage" {
		t.Errorf("expected type name %q, got %q", "kargo_stage", resp.TypeName)
	}
}

func TestStageResourceSchema(t *testing.T) {
	r := &StageResource{}
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %s", resp.Diagnostics)
	}

	projectAttr, ok := resp.Schema.Attributes["project"]
	if !ok {
		t.Fatal("expected 'project' attribute")
	}
	if !projectAttr.IsRequired() {
		t.Error("project should be required")
	}

	nameAttr, ok := resp.Schema.Attributes["name"]
	if !ok {
		t.Fatal("expected 'name' attribute")
	}
	if !nameAttr.IsRequired() {
		t.Error("name should be required")
	}

	idAttr, ok := resp.Schema.Attributes["id"]
	if !ok {
		t.Fatal("expected 'id' attribute")
	}
	if !idAttr.IsComputed() {
		t.Error("id should be computed")
	}

	shardAttr, ok := resp.Schema.Attributes["shard"]
	if !ok {
		t.Fatal("expected 'shard' attribute")
	}
	if !shardAttr.IsOptional() {
		t.Error("shard should be optional")
	}

	if _, ok := resp.Schema.Blocks["requested_freight"]; !ok {
		t.Fatal("expected 'requested_freight' block")
	}
	if _, ok := resp.Schema.Blocks["promotion_template"]; !ok {
		t.Fatal("expected 'promotion_template' block")
	}
}

func TestStageResourceSchemaValid(t *testing.T) {
	r := &StageResource{}
	resp := &fwresource.SchemaResponse{}
	r.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	diags := resp.Schema.ValidateImplementation(context.Background())
	if diags.HasError() {
		t.Fatalf("schema implementation invalid: %s", diags)
	}
}

func TestStageResourceConfigureWrongType(t *testing.T) {
	r := &StageResource{}
	resp := &fwresource.ConfigureResponse{}
	r.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: "wrong"}, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for wrong provider data type")
	}
}

func TestStageResourceConfigureNil(t *testing.T) {
	r := &StageResource{}
	resp := &fwresource.ConfigureResponse{}
	r.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: nil}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("unexpected error: %s", resp.Diagnostics)
	}
}

func TestNewStageResource(t *testing.T) {
	r := NewStageResource()
	if r == nil {
		t.Fatal("expected non-nil resource")
	}
	if _, ok := r.(*StageResource); !ok {
		t.Error("expected *StageResource")
	}
}

func testStageStringList(t *testing.T, values ...string) types.List {
	t.Helper()
	if values == nil {
		values = []string{}
	}
	list, diags := types.ListValueFrom(context.Background(), types.StringType, values)
	if diags.HasError() {
		t.Fatalf("building string list: %s", diags)
	}
	return list
}

func TestParseStageID(t *testing.T) {
	project, name, err := parseStageID("demo/staging")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if project != "demo" || name != "staging" {
		t.Fatalf("expected demo/staging, got %s/%s", project, name)
	}

	for _, id := range []string{"demo", "/x", "x/", "a/b/c"} {
		t.Run(id, func(t *testing.T) {
			_, _, err := parseStageID(id)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestExpandStageSpecDefaultsOriginKind(t *testing.T) {
	spec, err := expandStageSpec(context.Background(), &StageResourceModel{
		RequestedFreight: []StageRequestedFreightModel{{
			Origin:  &StageFreightOriginModel{Kind: types.StringNull(), Name: types.StringValue("app")},
			Sources: &StageFreightSourcesModel{Direct: types.BoolValue(true), Stages: types.ListNull(types.StringType)},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := spec.RequestedFreight[0].Origin.Kind; got != "Warehouse" {
		t.Errorf("expected origin kind %q, got %q", "Warehouse", got)
	}
}

func TestExpandStageSpecRequiresOriginName(t *testing.T) {
	for name, origin := range map[string]*StageFreightOriginModel{
		"nil_origin": nil,
		"null_name":  {Kind: types.StringNull(), Name: types.StringNull()},
		"empty_name": {Kind: types.StringNull(), Name: types.StringValue("")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := expandStageSpec(context.Background(), &StageResourceModel{
				RequestedFreight: []StageRequestedFreightModel{{
					Origin:  origin,
					Sources: &StageFreightSourcesModel{Direct: types.BoolValue(true)},
				}},
			})
			if err == nil || !strings.Contains(err.Error(), "requested_freight 0 origin.name must be set") {
				t.Fatalf("expected origin.name error, got %v", err)
			}
		})
	}
}

func TestExpandStageSpecRequiresSources(t *testing.T) {
	for name, sources := range map[string]*StageFreightSourcesModel{
		"nil_sources":  nil,
		"direct_false": {Direct: types.BoolValue(false), Stages: types.ListNull(types.StringType)},
		"direct_null":  {Direct: types.BoolNull(), Stages: types.ListNull(types.StringType)},
		"empty_stages": {Direct: types.BoolNull(), Stages: testStageStringList(t)},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := expandStageSpec(context.Background(), &StageResourceModel{
				RequestedFreight: []StageRequestedFreightModel{{
					Origin:  &StageFreightOriginModel{Kind: types.StringNull(), Name: types.StringValue("app")},
					Sources: sources,
				}},
			})
			if err == nil || !strings.Contains(err.Error(), "requested_freight 0 sources must set direct = true or at least one stage") {
				t.Fatalf("expected sources error, got %v", err)
			}
		})
	}
}

func TestExpandStageSpecRejectsDuplicateOrigins(t *testing.T) {
	_, err := expandStageSpec(context.Background(), &StageResourceModel{
		RequestedFreight: []StageRequestedFreightModel{
			{
				Origin:  &StageFreightOriginModel{Kind: types.StringValue("Warehouse"), Name: types.StringValue("app")},
				Sources: &StageFreightSourcesModel{Direct: types.BoolValue(true)},
			},
			{
				Origin:  &StageFreightOriginModel{Kind: types.StringNull(), Name: types.StringValue("app")},
				Sources: &StageFreightSourcesModel{Direct: types.BoolValue(true)},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicates origin") {
		t.Fatalf("expected duplicate origin error, got %v", err)
	}
}

func TestExpandStageSpecMapsSources(t *testing.T) {
	spec, err := expandStageSpec(context.Background(), &StageResourceModel{
		RequestedFreight: []StageRequestedFreightModel{
			{
				Origin:  &StageFreightOriginModel{Kind: types.StringNull(), Name: types.StringValue("app")},
				Sources: &StageFreightSourcesModel{Direct: types.BoolValue(true), Stages: types.ListNull(types.StringType)},
			},
			{
				Origin:  &StageFreightOriginModel{Kind: types.StringNull(), Name: types.StringValue("other")},
				Sources: &StageFreightSourcesModel{Direct: types.BoolNull(), Stages: testStageStringList(t, "test", "qa")},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	direct := spec.RequestedFreight[0].Sources
	if !direct.Direct {
		t.Error("expected direct sources to have Direct true")
	}
	if direct.Stages != nil {
		t.Errorf("expected direct sources to have nil Stages, got %v", direct.Stages)
	}

	upstream := spec.RequestedFreight[1].Sources
	if upstream.Direct {
		t.Error("expected upstream sources to have Direct false")
	}
	if !reflect.DeepEqual(upstream.Stages, []string{"test", "qa"}) {
		t.Errorf("expected stages [test qa], got %v", upstream.Stages)
	}
}

func TestExpandStageSpecRequiresStepUses(t *testing.T) {
	_, err := expandStageSpec(context.Background(), &StageResourceModel{
		RequestedFreight: []StageRequestedFreightModel{{
			Origin:  &StageFreightOriginModel{Kind: types.StringNull(), Name: types.StringValue("app")},
			Sources: &StageFreightSourcesModel{Direct: types.BoolValue(true)},
		}},
		PromotionTemplate: &StagePromotionTemplateModel{Step: []StageStepModel{{
			Uses:   types.StringNull(),
			As:     types.StringNull(),
			Config: jsontypes.NewNormalizedNull(),
		}}},
	})
	if err == nil || !strings.Contains(err.Error(), "promotion_template step 0 uses must be set") {
		t.Fatalf("expected step uses error, got %v", err)
	}
}

func TestExpandStageSpecMapsStepConfig(t *testing.T) {
	configJSON := `{"repoURL":"https://github.com/example/repo.git"}`
	spec, err := expandStageSpec(context.Background(), &StageResourceModel{
		Shard: types.StringValue("eu-west"),
		RequestedFreight: []StageRequestedFreightModel{{
			Origin:  &StageFreightOriginModel{Kind: types.StringNull(), Name: types.StringValue("app")},
			Sources: &StageFreightSourcesModel{Direct: types.BoolValue(true)},
		}},
		PromotionTemplate: &StagePromotionTemplateModel{Step: []StageStepModel{
			{Uses: types.StringValue("git-clone"), As: types.StringValue("clone"), Config: jsontypes.NewNormalizedValue(configJSON)},
			{Uses: types.StringValue("compose-output"), As: types.StringNull(), Config: jsontypes.NewNormalizedNull()},
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Shard != "eu-west" {
		t.Errorf("expected shard %q, got %q", "eu-west", spec.Shard)
	}
	if spec.PromotionTemplate == nil {
		t.Fatal("expected promotion template")
	}
	steps := spec.PromotionTemplate.Spec.Steps
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if string(steps[0].Config) != configJSON {
		t.Errorf("expected config %s, got %s", configJSON, steps[0].Config)
	}
	if steps[0].As != "clone" {
		t.Errorf("expected as %q, got %q", "clone", steps[0].As)
	}
	if steps[1].Config != nil {
		t.Errorf("expected nil config for null value, got %s", steps[1].Config)
	}
}

func TestFlattenStagePreservesPlannedConfig(t *testing.T) {
	plannedConfig := `{"b":1,"a":2}`
	prior := &StageResourceModel{
		Project: types.StringValue("demo"),
		Name:    types.StringValue("staging"),
		Shard:   types.StringNull(),
		RequestedFreight: []StageRequestedFreightModel{{
			Origin:  &StageFreightOriginModel{Kind: types.StringNull(), Name: types.StringValue("app")},
			Sources: &StageFreightSourcesModel{Direct: types.BoolValue(true), Stages: types.ListNull(types.StringType)},
		}},
		PromotionTemplate: &StagePromotionTemplateModel{Step: []StageStepModel{{
			Uses:   types.StringValue("git-clone"),
			As:     types.StringNull(),
			Config: jsontypes.NewNormalizedValue(plannedConfig),
		}}},
	}
	stage := &client.Stage{
		Metadata: client.StageMetadata{Name: "staging", Namespace: "demo"},
		Spec: client.StageSpec{
			RequestedFreight: []client.FreightRequest{{
				Origin:  client.FreightOrigin{Kind: "Warehouse", Name: "app"},
				Sources: client.FreightSources{Direct: true},
			}},
			PromotionTemplate: &client.PromotionTemplate{Spec: client.PromotionTemplateSpec{Steps: []client.PromotionStep{{
				Uses:   "git-clone",
				Config: json.RawMessage(`{"a":2,"b":1}`),
			}}}},
		},
	}

	flattened := flattenStage(context.Background(), "demo", stage, prior)
	if flattened.PromotionTemplate == nil || len(flattened.PromotionTemplate.Step) != 1 {
		t.Fatalf("expected one promotion template step, got %+v", flattened.PromotionTemplate)
	}
	if got := flattened.PromotionTemplate.Step[0].Config.ValueString(); got != plannedConfig {
		t.Errorf("expected planned config %s echoed to state, got %s", plannedConfig, got)
	}
}

func TestFlattenStageImport(t *testing.T) {
	serverConfig := `{"repoURL":"https://github.com/example/repo.git"}`
	stage := &client.Stage{
		Metadata: client.StageMetadata{Name: "staging", Namespace: "demo"},
		Spec: client.StageSpec{
			Shard: "eu-west",
			RequestedFreight: []client.FreightRequest{{
				Origin:  client.FreightOrigin{Kind: "Warehouse", Name: "app"},
				Sources: client.FreightSources{Direct: true},
			}},
			PromotionTemplate: &client.PromotionTemplate{Spec: client.PromotionTemplateSpec{Steps: []client.PromotionStep{
				{Uses: "git-clone", As: "clone", Config: json.RawMessage(serverConfig)},
				{Uses: "compose-output"},
			}}},
		},
	}

	data := flattenStage(context.Background(), "demo", stage, nil)

	if got := data.ID.ValueString(); got != "demo/staging" {
		t.Errorf("expected id demo/staging, got %s", got)
	}
	if got := data.Shard.ValueString(); got != "eu-west" {
		t.Errorf("expected shard eu-west, got %s", got)
	}
	if len(data.RequestedFreight) != 1 {
		t.Fatalf("expected 1 requested freight, got %d", len(data.RequestedFreight))
	}
	freight := data.RequestedFreight[0]
	if freight.Origin == nil || freight.Origin.Kind.ValueString() != "Warehouse" {
		t.Errorf("expected imported origin kind Warehouse, got %+v", freight.Origin)
	}
	if freight.Origin != nil && freight.Origin.Name.ValueString() != "app" {
		t.Errorf("expected origin name app, got %s", freight.Origin.Name)
	}
	if freight.Sources == nil || !freight.Sources.Direct.ValueBool() {
		t.Errorf("expected imported sources direct true, got %+v", freight.Sources)
	}
	if freight.Sources != nil && !freight.Sources.Stages.IsNull() {
		t.Errorf("expected imported empty stages to be null, got %s", freight.Sources.Stages)
	}
	if data.PromotionTemplate == nil || len(data.PromotionTemplate.Step) != 2 {
		t.Fatalf("expected 2 promotion template steps, got %+v", data.PromotionTemplate)
	}
	steps := data.PromotionTemplate.Step
	if got := steps[0].Config.ValueString(); got != serverConfig {
		t.Errorf("expected imported config %s, got %s", serverConfig, got)
	}
	if got := steps[0].As.ValueString(); got != "clone" {
		t.Errorf("expected imported as clone, got %s", got)
	}
	if !steps[1].Config.IsNull() {
		t.Errorf("expected empty server config to import as null, got %s", steps[1].Config)
	}
}

func TestFlattenStagePreservesOmittedOptionals(t *testing.T) {
	prior := &StageResourceModel{
		Project: types.StringValue("demo"),
		Name:    types.StringValue("staging"),
		Shard:   types.StringNull(),
		RequestedFreight: []StageRequestedFreightModel{{
			Origin:  &StageFreightOriginModel{Kind: types.StringNull(), Name: types.StringValue("app")},
			Sources: &StageFreightSourcesModel{Direct: types.BoolValue(true), Stages: types.ListNull(types.StringType)},
		}},
		PromotionTemplate: &StagePromotionTemplateModel{Step: []StageStepModel{{
			Uses:   types.StringValue("git-clone"),
			As:     types.StringNull(),
			Config: jsontypes.NewNormalizedNull(),
		}}},
	}
	stage := &client.Stage{
		Metadata: client.StageMetadata{Name: "staging", Namespace: "demo"},
		Spec: client.StageSpec{
			Shard: "",
			RequestedFreight: []client.FreightRequest{{
				Origin:  client.FreightOrigin{Kind: "Warehouse", Name: "app"},
				Sources: client.FreightSources{Direct: true},
			}},
			PromotionTemplate: &client.PromotionTemplate{Spec: client.PromotionTemplateSpec{Steps: []client.PromotionStep{{
				Uses: "git-clone",
				As:   "",
			}}}},
		},
	}

	data := flattenStage(context.Background(), "demo", stage, prior)

	if !data.Shard.IsNull() {
		t.Errorf("expected omitted shard to stay null, got %s", data.Shard)
	}
	if len(data.RequestedFreight) != 1 || data.RequestedFreight[0].Origin == nil {
		t.Fatalf("expected 1 requested freight with origin, got %+v", data.RequestedFreight)
	}
	if !data.RequestedFreight[0].Origin.Kind.IsNull() {
		t.Errorf("expected omitted origin kind to stay null, got %s", data.RequestedFreight[0].Origin.Kind)
	}
	if data.PromotionTemplate == nil || len(data.PromotionTemplate.Step) != 1 {
		t.Fatalf("expected 1 promotion template step, got %+v", data.PromotionTemplate)
	}
	if !data.PromotionTemplate.Step[0].As.IsNull() {
		t.Errorf("expected omitted step as to stay null, got %s", data.PromotionTemplate.Step[0].As)
	}
}

type stageTestServer struct {
	*httptest.Server

	mu          sync.RWMutex
	stages      map[string]map[string]any // key: project/name; value: stored manifest (metadata + spec)
	updateCount int
}

// testStageServer emulates the wire contract observed against a real Kargo
// v1.10.8 (phase 09-01 live check): GetStage must be requested with
// format RAW_FORMAT_JSON and responds {"raw": base64(standard k8s JSON
// manifest)} with inline step config. The handler fails the test on any
// structured (non-raw) read so the suite cannot silently validate the
// protojson-corrupted path.
func testStageServer(t *testing.T) *stageTestServer {
	t.Helper()

	state := &stageTestServer{
		stages: map[string]map[string]any{},
	}

	state.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case endsWith(r.URL.Path, "/AdminLogin"):
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{"idToken": "test-jwt"})) //nolint:gosec // test
		case endsWith(r.URL.Path, "/CreateResource"):
			manifest, encoded := decodeStageManifest(t, r)
			key := stageManifestKey(t, manifest)

			state.mu.Lock()
			state.stages[key] = storedStage(manifest, fmt.Sprintf("%d", len(state.stages)+1))
			state.mu.Unlock()

			assertNoError(t, json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]string{{"createdResourceManifest": encoded}},
			}))
		case endsWith(r.URL.Path, "/UpdateResource"):
			manifest, encoded := decodeStageManifest(t, r)
			key := stageManifestKey(t, manifest)

			state.mu.Lock()
			if _, ok := state.stages[key]; !ok {
				state.mu.Unlock()
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"code":"not_found","message":"stage not found"}`)
				return
			}
			state.updateCount++
			state.stages[key] = storedStage(manifest, fmt.Sprintf("%d", state.updateCount+100))
			state.mu.Unlock()

			assertNoError(t, json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]string{{"updatedResourceManifest": encoded}},
			}))
		case endsWith(r.URL.Path, "/GetStage"):
			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			// Pitfall 3: a structured (non-raw) read returns protojson-encoded step
			// config ({"raw": base64}); the client must never regress to that path.
			if body["format"] != "RAW_FORMAT_JSON" {
				t.Errorf("GetStage must request RAW_FORMAT_JSON, got %q", body["format"])
			}
			key := stageID(body["project"], body["name"])

			state.mu.RLock()
			stage, ok := state.stages[key]
			state.mu.RUnlock()
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"code":"not_found","message":"stage not found"}`)
				return
			}
			// Marshaling the stored map sorts keys, emulating the k8s
			// re-serialization the real server produces for raw-format reads.
			manifestJSON, err := json.Marshal(stage)
			assertNoError(t, err)
			assertNoError(t, json.NewEncoder(w).Encode(map[string]string{
				"raw": base64.StdEncoding.EncodeToString(manifestJSON),
			}))
		case endsWith(r.URL.Path, "/DeleteStage"):
			var body map[string]string
			assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
			key := stageID(body["project"], body["name"])

			state.mu.Lock()
			if _, ok := state.stages[key]; !ok {
				state.mu.Unlock()
				// Real Kargo propagates the k8s 404 for missing stages (Pitfall 7).
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"code":"not_found","message":"stage not found"}`)
				return
			}
			delete(state.stages, key)
			state.mu.Unlock()

			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return state
}

func decodeStageManifest(t *testing.T, r *http.Request) (manifest map[string]any, encoded string) {
	t.Helper()

	var body map[string]string
	assertNoError(t, json.NewDecoder(r.Body).Decode(&body))
	encoded = body["manifest"]
	manifestJSON, err := base64.StdEncoding.DecodeString(encoded)
	assertNoError(t, err)

	assertNoError(t, json.Unmarshal(manifestJSON, &manifest))
	if manifest["kind"] != "Stage" {
		t.Fatalf("expected kind Stage, got %v", manifest["kind"])
	}
	return manifest, encoded
}

func stageManifestKey(t *testing.T, manifest map[string]any) string {
	t.Helper()

	metadata, ok := manifest["metadata"].(map[string]any)
	if !ok {
		t.Fatal("manifest metadata missing")
	}
	project, _ := metadata["namespace"].(string)
	name, _ := metadata["name"].(string)
	if project == "" || name == "" {
		t.Fatalf("manifest missing namespace/name: %#v", metadata)
	}
	return stageID(project, name)
}

// storedStage keeps exactly what CreateResource received (metadata + spec) so
// raw-format GetStage responses round-trip the manifest the client sent.
func storedStage(manifest map[string]any, resourceVersion string) map[string]any {
	metadata := manifest["metadata"].(map[string]any)
	name := metadata["name"].(string)

	return map[string]any{
		"metadata": map[string]any{
			"name":            name,
			"namespace":       metadata["namespace"],
			"uid":             "uid-" + name,
			"resourceVersion": resourceVersion,
		},
		"spec": manifest["spec"],
	}
}

func (s *stageTestServer) updateCountForTest() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.updateCount
}

func testCheckStageUpdateCountAtLeast(srv *stageTestServer, minimum int) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if got := srv.updateCountForTest(); got < minimum {
			return fmt.Errorf("expected at least %d stage updates, got %d", minimum, got)
		}
		return nil
	}
}

func TestAccStageResource_basic(t *testing.T) {
	srv := testStageServer(t)
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testStageResourceBasicConfig(srv.URL, "tf-test-project", "tf-test-stage", "eu-west"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_stage.test", "id", "tf-test-project/tf-test-stage"),
					resource.TestCheckResourceAttr("kargo_stage.test", "shard", "eu-west"),
					resource.TestCheckResourceAttr("kargo_stage.test", "requested_freight.0.origin.name", "app"),
					resource.TestCheckResourceAttr("kargo_stage.test", "requested_freight.0.sources.direct", "true"),
					resource.TestCheckResourceAttr("kargo_stage.test", "promotion_template.step.0.uses", "git-clone"),
					resource.TestCheckResourceAttr("kargo_stage.test", "promotion_template.step.0.as", "clone"),
				),
			},
			{
				Config: testStageResourceBasicConfig(srv.URL, "tf-test-project", "tf-test-stage", "us-east"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_stage.test", "shard", "us-east"),
					testCheckStageUpdateCountAtLeast(srv, 1),
				),
			},
			{
				// Idempotency gate (Pitfall 2): refresh must produce an empty plan.
				Config:   testStageResourceBasicConfig(srv.URL, "tf-test-project", "tf-test-stage", "us-east"),
				PlanOnly: true,
			},
		},
	})
}

func TestAccStageResource_import(t *testing.T) {
	srv := testStageServer(t)
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testStageResourceImportConfig(srv.URL, "tf-test-project", "tf-test-stage"),
			},
			{
				ResourceName:      "kargo_stage.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testStageProviderConfig(url string) string {
	return fmt.Sprintf(`
provider "kargo" {
  api_url                  = %q
  admin_password           = "admin"
  insecure_skip_tls_verify = true
}
`, url)
}

func testStageResourceBasicConfig(url, project, name, shard string) string {
	return fmt.Sprintf(`%s
resource "kargo_stage" "test" {
  project = %q
  name    = %q
  shard   = %q

  requested_freight {
    origin {
      name = "app"
    }
    sources {
      direct = true
    }
  }

  promotion_template {
    step {
      uses = "git-clone"
      as   = "clone"
      config = jsonencode({
        checkout = [{ branch = "main", path = "./src" }]
        repoURL  = "https://github.com/example/repo.git"
      })
    }
  }
}
`, testStageProviderConfig(url), project, name, shard)
}

// testStageResourceImportConfig sets origin.kind explicitly: import materializes
// the server's kind while a config that omits it keeps null in created state, so
// strict ImportStateVerify requires the value to be present in both.
func testStageResourceImportConfig(url, project, name string) string {
	return fmt.Sprintf(`%s
resource "kargo_stage" "test" {
  project = %q
  name    = %q
  shard   = "eu-west"

  requested_freight {
    origin {
      kind = "Warehouse"
      name = "app"
    }
    sources {
      direct = true
    }
  }

  promotion_template {
    step {
      uses = "git-clone"
      as   = "clone"
      config = jsonencode({
        checkout = [{ branch = "main", path = "./src" }]
        repoURL  = "https://github.com/example/repo.git"
      })
    }
  }
}
`, testStageProviderConfig(url), project, name)
}
