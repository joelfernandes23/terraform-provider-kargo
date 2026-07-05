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

// flattenStage must return the server's config bytes even when prior state
// exists so out-of-band edits surface as drift; the framework's
// jsontypes.Normalized semantic equality (not a prior-state echo) is what
// keeps plans stable against the server's key re-sorting.
func TestFlattenStageReturnsServerConfig(t *testing.T) {
	serverConfig := `{"a":2,"b":1}`
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
			Config: jsontypes.NewNormalizedValue(`{"b":1,"a":2}`),
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
				Config: json.RawMessage(serverConfig),
			}}}},
		},
	}

	flattened := flattenStage(context.Background(), "demo", stage, prior)
	if flattened.PromotionTemplate == nil || len(flattened.PromotionTemplate.Step) != 1 {
		t.Fatalf("expected one promotion template step, got %+v", flattened.PromotionTemplate)
	}
	if got := flattened.PromotionTemplate.Step[0].Config.ValueString(); got != serverConfig {
		t.Errorf("expected server config %s in state, got %s", serverConfig, got)
	}
}

// flattenStage must reflect out-of-band edits to sources in state so refresh
// produces drift, while still preserving prior state for the zero values the
// wire format cannot round-trip (direct false and empty stages are dropped by
// omitempty).
func TestFlattenStageSourcesDriftAndAmbiguity(t *testing.T) {
	buildPrior := func(direct types.Bool, stages types.List) *StageResourceModel {
		return &StageResourceModel{
			Project: types.StringValue("demo"),
			Name:    types.StringValue("staging"),
			Shard:   types.StringNull(),
			RequestedFreight: []StageRequestedFreightModel{{
				Origin:  &StageFreightOriginModel{Kind: types.StringNull(), Name: types.StringValue("app")},
				Sources: &StageFreightSourcesModel{Direct: direct, Stages: stages},
			}},
		}
	}
	buildStage := func(sources client.FreightSources) *client.Stage {
		return &client.Stage{
			Metadata: client.StageMetadata{Name: "staging", Namespace: "demo"},
			Spec: client.StageSpec{
				RequestedFreight: []client.FreightRequest{{
					Origin:  client.FreightOrigin{Kind: "Warehouse", Name: "app"},
					Sources: sources,
				}},
			},
		}
	}
	flattenSources := func(t *testing.T, stage *client.Stage, prior *StageResourceModel) *StageFreightSourcesModel {
		t.Helper()
		data := flattenStage(context.Background(), "demo", stage, prior)
		if len(data.RequestedFreight) != 1 || data.RequestedFreight[0].Sources == nil {
			t.Fatalf("expected 1 requested freight with sources, got %+v", data.RequestedFreight)
		}
		return data.RequestedFreight[0].Sources
	}

	t.Run("out_of_band_stages_edit_reaches_state", func(t *testing.T) {
		prior := buildPrior(types.BoolNull(), testStageStringList(t, "test"))
		sources := flattenSources(t, buildStage(client.FreightSources{Stages: []string{"qa"}}), prior)

		var stages []string
		if diags := sources.Stages.ElementsAs(context.Background(), &stages, false); diags.HasError() {
			t.Fatalf("reading stages: %s", diags)
		}
		if !reflect.DeepEqual(stages, []string{"qa"}) {
			t.Errorf("expected out-of-band stages [qa] in state, got %v", stages)
		}
	})

	t.Run("out_of_band_direct_removal_reaches_state", func(t *testing.T) {
		prior := buildPrior(types.BoolValue(true), types.ListNull(types.StringType))
		sources := flattenSources(t, buildStage(client.FreightSources{Stages: []string{"qa"}}), prior)

		if !sources.Direct.IsNull() {
			t.Errorf("expected direct true removed on server to drift to null, got %s", sources.Direct)
		}
	})

	t.Run("planned_direct_false_preserved", func(t *testing.T) {
		prior := buildPrior(types.BoolValue(false), testStageStringList(t, "test"))
		sources := flattenSources(t, buildStage(client.FreightSources{Stages: []string{"test"}}), prior)

		if sources.Direct.IsNull() || sources.Direct.ValueBool() {
			t.Errorf("expected planned direct=false preserved against omitempty, got %s", sources.Direct)
		}
	})

	t.Run("planned_empty_stages_preserved", func(t *testing.T) {
		prior := buildPrior(types.BoolValue(true), testStageStringList(t))
		sources := flattenSources(t, buildStage(client.FreightSources{Direct: true}), prior)

		if sources.Stages.IsNull() || len(sources.Stages.Elements()) != 0 {
			t.Errorf("expected planned empty stages preserved against omitempty, got %s", sources.Stages)
		}
	})
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

func (s *stageTestServer) deleteStageForTest(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.stages, key)
}

// mutateStageForTest applies an out-of-band edit to a stored stage manifest,
// emulating a kubectl edit that bypasses Terraform.
func (s *stageTestServer) mutateStageForTest(key string, mutate func(stage map[string]any)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mutate(s.stages[key])
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
				Config: testStageResourceBasicConfig(srv.URL, "eu-west"),
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
				Config: testStageResourceBasicConfig(srv.URL, "us-east"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_stage.test", "shard", "us-east"),
					testCheckStageUpdateCountAtLeast(srv, 1),
				),
			},
			{
				// Idempotency gate (Pitfall 2): refresh must produce an empty plan.
				Config:   testStageResourceBasicConfig(srv.URL, "us-east"),
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
				Config: testStageResourceImportConfig(srv.URL),
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

func testStageResourceBasicConfig(url, shard string) string {
	return fmt.Sprintf(`%s
resource "kargo_stage" "test" {
  project = "tf-test-project"
  name    = "tf-test-stage"
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
`, testStageProviderConfig(url), shard)
}

// testStageResourceImportConfig sets origin.kind explicitly: import materializes
// the server's kind while a config that omits it keeps null in created state, so
// strict ImportStateVerify requires the value to be present in both.
func testStageResourceImportConfig(url string) string {
	return fmt.Sprintf(`%s
resource "kargo_stage" "test" {
  project = "tf-test-project"
  name    = "tf-test-stage"
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
`, testStageProviderConfig(url))
}

func TestAccStageResource_promotionSteps(t *testing.T) {
	srv := testStageServer(t)
	defer srv.Close()

	checkStepOneConfig := func(retries float64) resource.TestCheckFunc {
		return resource.TestCheckResourceAttrWith("kargo_stage.test", "promotion_template.step.1.config", func(value string) error {
			var config map[string]any
			if err := json.Unmarshal([]byte(value), &config); err != nil {
				return fmt.Errorf("config is not valid JSON: %w", err)
			}
			if got := config["retries"]; got != retries {
				return fmt.Errorf("expected retries %v as a JSON number, got %#v", retries, got)
			}
			if got := config["wait"]; got != true {
				return fmt.Errorf("expected wait true as a JSON bool, got %#v", got)
			}
			return nil
		})
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testStagePromotionStepsConfig(srv.URL, 3),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_stage.test", "promotion_template.step.#", "2"),
					resource.TestCheckResourceAttr("kargo_stage.test", "promotion_template.step.0.uses", "git-clone"),
					resource.TestCheckResourceAttr("kargo_stage.test", "promotion_template.step.1.uses", "argocd-update"),
					checkStepOneConfig(3),
				),
			},
			{
				// Pitfall 2 regression gate: same semantic config with the object keys
				// written in a different order must plan clean (jsonencode sorts keys
				// and Normalized semantic equality covers whitespace differences).
				Config:   testStagePromotionStepsReorderedConfig(srv.URL),
				PlanOnly: true,
			},
			{
				Config: testStagePromotionStepsConfig(srv.URL, 5),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkStepOneConfig(5),
					testCheckStageUpdateCountAtLeast(srv, 1),
				),
			},
		},
	})
}

func TestAccStageResource_outOfBandDeletion(t *testing.T) {
	srv := testStageServer(t)
	defer srv.Close()

	config := testStageResourceBasicConfig(srv.URL, "eu-west")
	key := "tf-test-project/tf-test-stage"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("kargo_stage.test", "id", key),
			},
			{
				PreConfig: func() {
					srv.deleteStageForTest(key)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("kargo_stage.test", "id", key),
			},
		},
	})
}

func TestAccStageResource_outOfBandDrift(t *testing.T) {
	srv := testStageServer(t)
	defer srv.Close()

	config := testStageDriftConfig(srv.URL)
	key := "tf-test-project/tf-test-stage"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_stage.test", "requested_freight.0.sources.stages.0", "test"),
					resource.TestCheckResourceAttr("kargo_stage.test", "promotion_template.step.0.config", `{"retries":3}`),
				),
			},
			{
				// WR-01 regression gate: kubectl-style edits to sources.stages and
				// step config must reach state on refresh and produce a plan.
				PreConfig: func() {
					srv.mutateStageForTest(key, func(stage map[string]any) {
						spec := stage["spec"].(map[string]any)
						freight := spec["requestedFreight"].([]any)[0].(map[string]any)
						freight["sources"].(map[string]any)["stages"] = []any{"qa"}
						template := spec["promotionTemplate"].(map[string]any)["spec"].(map[string]any)
						step := template["steps"].([]any)[0].(map[string]any)
						step["config"] = map[string]any{"retries": 99}
					})
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_stage.test", "requested_freight.0.sources.stages.0", "qa"),
					resource.TestCheckResourceAttr("kargo_stage.test", "promotion_template.step.0.config", `{"retries":99}`),
				),
			},
			{
				// Applying the same config reconciles the remote drift.
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_stage.test", "requested_freight.0.sources.stages.0", "test"),
					resource.TestCheckResourceAttr("kargo_stage.test", "promotion_template.step.0.config", `{"retries":3}`),
					testCheckStageUpdateCountAtLeast(srv, 1),
				),
			},
		},
	})
}

func testStageDriftConfig(url string) string {
	return fmt.Sprintf(`%s
resource "kargo_stage" "test" {
  project = "tf-test-project"
  name    = "tf-test-stage"

  requested_freight {
    origin {
      name = "app"
    }
    sources {
      stages = ["test"]
    }
  }

  promotion_template {
    step {
      uses = "argocd-update"
      config = jsonencode({
        retries = 3
      })
    }
  }
}
`, testStageProviderConfig(url))
}

func TestAccStageResource_multiStage(t *testing.T) {
	srv := testStageServer(t)
	defer srv.Close()

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: testStageMultiStageConfig(srv.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("kargo_stage.test", "id", "tf-test-project/test"),
					resource.TestCheckResourceAttr("kargo_stage.test", "requested_freight.0.sources.direct", "true"),
					resource.TestCheckResourceAttr("kargo_stage.staging", "id", "tf-test-project/staging"),
					resource.TestCheckResourceAttr("kargo_stage.staging", "requested_freight.0.sources.stages.0", "test"),
					resource.TestCheckResourceAttr("kargo_stage.prod", "id", "tf-test-project/prod"),
					resource.TestCheckResourceAttr("kargo_stage.prod", "requested_freight.0.sources.stages.0", "staging"),
				),
			},
		},
	})
}

func testStagePromotionStepsConfig(url string, retries int) string {
	return fmt.Sprintf(`%s
resource "kargo_stage" "test" {
  project = "tf-test-project"
  name    = "tf-test-stage"

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
    step {
      uses = "argocd-update"
      config = jsonencode({
        apps    = [{ name = "app-staging" }]
        retries = %d
        wait    = true
      })
    }
  }
}
`, testStageProviderConfig(url), retries)
}

// testStagePromotionStepsReorderedConfig is semantically identical to
// testStagePromotionStepsConfig(url, 3) but writes the config object keys in a
// different order in HCL.
func testStagePromotionStepsReorderedConfig(url string) string {
	return fmt.Sprintf(`%s
resource "kargo_stage" "test" {
  project = "tf-test-project"
  name    = "tf-test-stage"

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
        repoURL  = "https://github.com/example/repo.git"
        checkout = [{ path = "./src", branch = "main" }]
      })
    }
    step {
      uses = "argocd-update"
      config = jsonencode({
        wait    = true
        retries = 3
        apps    = [{ name = "app-staging" }]
      })
    }
  }
}
`, testStageProviderConfig(url))
}

func testStageMultiStageConfig(url string) string {
	return fmt.Sprintf(`%s
resource "kargo_stage" "test" {
  project = "tf-test-project"
  name    = "test"

  requested_freight {
    origin {
      name = "app"
    }
    sources {
      direct = true
    }
  }
}

resource "kargo_stage" "staging" {
  project = "tf-test-project"
  name    = "staging"

  requested_freight {
    origin {
      name = "app"
    }
    sources {
      stages = [kargo_stage.test.name]
    }
  }
}

resource "kargo_stage" "prod" {
  project = "tf-test-project"
  name    = "prod"

  requested_freight {
    origin {
      name = "app"
    }
    sources {
      stages = [kargo_stage.staging.name]
    }
  }
}
`, testStageProviderConfig(url))
}
