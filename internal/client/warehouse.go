package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type WarehouseMetadata struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	UID               string            `json:"uid,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	DeletionTimestamp *string           `json:"deletionTimestamp,omitempty"`
}

type WarehouseStatus struct {
	Conditions          []WarehouseCondition `json:"conditions,omitempty"`
	LastHandledRefresh  string               `json:"lastHandledRefresh,omitempty"`
	ObservedGeneration  ProtobufInt64        `json:"observedGeneration,omitempty"`
	LastFreightID       string               `json:"lastFreightID,omitempty"`
	DiscoveredArtifacts json.RawMessage      `json:"discoveredArtifacts,omitempty"`
}

type WarehouseCondition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
	LastTransitionTime KargoTime `json:"lastTransitionTime,omitempty"`
}

type Warehouse struct {
	Metadata WarehouseMetadata `json:"metadata"`
	Spec     WarehouseSpec     `json:"spec,omitempty"`
	Status   WarehouseStatus   `json:"status,omitempty"`
}

type WarehouseSpec struct {
	Shard                   string                   `json:"shard,omitempty"`
	Interval                string                   `json:"interval,omitempty"`
	FreightCreationPolicy   string                   `json:"freightCreationPolicy,omitempty"`
	FreightCreationCriteria *FreightCreationCriteria `json:"freightCreationCriteria,omitempty"`
	Subscriptions           []WarehouseSubscription  `json:"subscriptions"`
}

type FreightCreationCriteria struct {
	Expression string `json:"expression,omitempty"`
}

type WarehouseSubscription struct {
	Name    string               `json:"name,omitempty"`
	Image   *ImageSubscription   `json:"image,omitempty"`
	Git     *GitSubscription     `json:"git,omitempty"`
	Chart   *ChartSubscription   `json:"chart,omitempty"`
	Generic *GenericSubscription `json:"-"`
}

// MarshalJSON preserves Kargo's extensible subscription shape: generic
// subscription kinds are represented by their kind as the top-level key.
func (s WarehouseSubscription) MarshalJSON() ([]byte, error) {
	if s.Generic != nil {
		return json.Marshal(map[string]any{s.Generic.Type: genericSubscriptionManifest{
			Name: s.Name, Config: s.Generic.Config, DiscoveryLimit: s.Generic.DiscoveryLimit,
		}})
	}
	type known WarehouseSubscription
	return json.Marshal(known(s))
}

func (s *WarehouseSubscription) UnmarshalJSON(data []byte) error {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if len(values) != 1 {
		return fmt.Errorf("warehouse subscription must specify exactly one type")
	}
	for _, value := range values {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(value, &object); err != nil || object == nil {
			return fmt.Errorf("warehouse subscription must be a JSON object")
		}
	}
	var known struct {
		Name  string             `json:"name,omitempty"`
		Image *ImageSubscription `json:"image,omitempty"`
		Git   *GitSubscription   `json:"git,omitempty"`
		Chart *ChartSubscription `json:"chart,omitempty"`
	}
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	*s = WarehouseSubscription{Name: known.Name, Image: known.Image, Git: known.Git, Chart: known.Chart}
	if s.Image != nil || s.Git != nil || s.Chart != nil {
		return nil
	}
	for kind, config := range values {
		var manifest genericSubscriptionManifest
		if err := json.Unmarshal(config, &manifest); err != nil {
			return err
		}
		s.Name = manifest.Name
		s.Generic = &GenericSubscription{Type: kind, Config: manifest.Config, DiscoveryLimit: manifest.DiscoveryLimit}
	}
	return nil
}

type GenericSubscription struct {
	Type           string
	Config         json.RawMessage
	DiscoveryLimit *int64
}

type genericSubscriptionManifest struct {
	Name           string          `json:"name"`
	Config         json.RawMessage `json:"config,omitempty"`
	DiscoveryLimit *int64          `json:"discoveryLimit,omitempty"`
}

type ImageSubscription struct {
	RepoURL                string   `json:"repoURL"`
	AllowTags              string   `json:"allowTags,omitempty"`
	Constraint             string   `json:"constraint,omitempty"`
	ImageSelectionStrategy string   `json:"imageSelectionStrategy,omitempty"`
	Platform               string   `json:"platform,omitempty"`
	AllowTagsRegexes       []string `json:"allowTagsRegexes,omitempty"`
	IgnoreTagsRegexes      []string `json:"ignoreTagsRegexes,omitempty"`
	IgnoreTags             []string `json:"ignoreTags,omitempty"`
	CacheByTag             *bool    `json:"cacheByTag,omitempty"`
	DiscoveryLimit         *int64   `json:"discoveryLimit,omitempty"`
	InsecureSkipTLSVerify  *bool    `json:"insecureSkipTLSVerify,omitempty"`
	StrictSemvers          *bool    `json:"strictSemvers,omitempty"`
}

type GitSubscription struct {
	RepoURL                 string   `json:"repoURL"`
	AllowTags               string   `json:"allowTags,omitempty"`
	Branch                  string   `json:"branch,omitempty"`
	CommitSelectionStrategy string   `json:"commitSelectionStrategy,omitempty"`
	SemverConstraint        string   `json:"semverConstraint,omitempty"`
	AllowTagsRegexes        []string `json:"allowTagsRegexes,omitempty"`
	IgnoreTagsRegexes       []string `json:"ignoreTagsRegexes,omitempty"`
	IgnoreTags              []string `json:"ignoreTags,omitempty"`
	IncludePaths            []string `json:"includePaths,omitempty"`
	ExcludePaths            []string `json:"excludePaths,omitempty"`
	ExpressionFilter        string   `json:"expressionFilter,omitempty"`
	Since                   string   `json:"since,omitempty"`
	Blobless                *bool    `json:"blobless,omitempty"`
	DiscoveryLimit          *int64   `json:"discoveryLimit,omitempty"`
	InsecureSkipTLSVerify   *bool    `json:"insecureSkipTLSVerify,omitempty"`
	StrictSemvers           *bool    `json:"strictSemvers,omitempty"`
}

type ChartSubscription struct {
	RepoURL               string `json:"repoURL"`
	Name                  string `json:"name,omitempty"`
	SemverConstraint      string `json:"semverConstraint,omitempty"`
	DiscoveryLimit        *int64 `json:"discoveryLimit,omitempty"`
	InsecureSkipTLSVerify *bool  `json:"insecureSkipTLSVerify,omitempty"`
}

type ProtobufInt64 struct {
	value int64
	set   bool
}

func (i *ProtobufInt64) UnmarshalJSON(data []byte) error {
	value, set, err := parseJSONInt64(data)
	if err != nil {
		return err
	}
	i.value = value
	i.set = set
	return nil
}

func (i ProtobufInt64) Value() int64 {
	return i.value
}

func (i ProtobufInt64) Set() bool {
	return i.set
}

type KargoTime string

func (t *KargoTime) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		*t = ""
		return nil
	}
	if strings.HasPrefix(raw, `"`) {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*t = KargoTime(value)
		return nil
	}

	var value struct {
		Seconds json.RawMessage `json:"seconds"`
		Nanos   int64           `json:"nanos"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	seconds, set, err := parseJSONInt64(value.Seconds)
	if err != nil {
		return err
	}
	if !set {
		*t = ""
		return nil
	}
	*t = KargoTime(time.Unix(seconds, value.Nanos).UTC().Format(time.RFC3339Nano))
	return nil
}

func (t KargoTime) String() string {
	return string(t)
}

type KargoDuration string

func (d *KargoDuration) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		*d = ""
		return nil
	}
	if strings.HasPrefix(raw, `"`) {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*d = KargoDuration(value)
		return nil
	}

	var value struct {
		Duration json.RawMessage `json:"duration"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	nanos, set, err := parseJSONInt64(value.Duration)
	if err != nil {
		return err
	}
	if !set {
		*d = ""
		return nil
	}
	*d = KargoDuration(time.Duration(nanos).String())
	return nil
}

func (d KargoDuration) String() string {
	return string(d)
}

type FreightMetadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

type Freight struct {
	Metadata  FreightMetadata     `json:"metadata"`
	Alias     string              `json:"alias,omitempty"`
	Origin    *FreightOrigin      `json:"origin,omitempty"`
	Commits   []GitCommit         `json:"commits,omitempty"`
	Images    []Image             `json:"images,omitempty"`
	Charts    []Chart             `json:"charts,omitempty"`
	Artifacts []ArtifactReference `json:"artifacts,omitempty"`
	Status    FreightStatus       `json:"status,omitempty"`
}

type FreightOrigin struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type GitCommit struct {
	RepoURL   string `json:"repoURL"`
	ID        string `json:"id"`
	Branch    string `json:"branch,omitempty"`
	Tag       string `json:"tag,omitempty"`
	Message   string `json:"message,omitempty"`
	Author    string `json:"author,omitempty"`
	Committer string `json:"committer,omitempty"`
}

type Image struct {
	RepoURL string `json:"repoURL"`
	Tag     string `json:"tag,omitempty"`
	Digest  string `json:"digest,omitempty"`
}

type Chart struct {
	RepoURL string `json:"repoURL"`
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
}

type ArtifactReference struct {
	ArtifactType     string `json:"artifactType"`
	SubscriptionName string `json:"subscriptionName"`
	Version          string `json:"version"`
}

type FreightStatus struct {
	CurrentlyIn map[string]CurrentStage  `json:"currentlyIn,omitempty"`
	VerifiedIn  map[string]VerifiedStage `json:"verifiedIn,omitempty"`
	ApprovedFor map[string]ApprovedStage `json:"approvedFor,omitempty"`
}

type CurrentStage struct {
	Since KargoTime `json:"since,omitempty"`
}

type VerifiedStage struct {
	VerifiedAt  KargoTime     `json:"verifiedAt,omitempty"`
	LongestSoak KargoDuration `json:"longestSoak,omitempty"`
}

type ApprovedStage struct {
	ApprovedAt KargoTime `json:"approvedAt,omitempty"`
}

type getWarehouseResponse struct {
	Raw []byte `json:"raw"`
}

type queryFreightResponse struct {
	Groups map[string]freightList `json:"groups"`
}

type freightList struct {
	Freight []Freight `json:"freight"`
}

type resourceResultResponse struct {
	Results []struct {
		CreatedResourceManifest string `json:"createdResourceManifest,omitempty"`
		UpdatedResourceManifest string `json:"updatedResourceManifest,omitempty"`
		Error                   string `json:"error,omitempty"`
	} `json:"results"`
}

type warehouseManifest struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   WarehouseMetadata `json:"metadata"`
	Spec       WarehouseSpec     `json:"spec"`
}

func marshalWarehouseManifest(project, name string, spec WarehouseSpec) ([]byte, error) {
	return json.Marshal(warehouseManifest{
		APIVersion: "kargo.akuity.io/v1alpha1",
		Kind:       "Warehouse",
		Metadata: WarehouseMetadata{
			Name:      name,
			Namespace: project,
		},
		Spec: spec,
	})
}

func checkResourceResult(resp resourceResultResponse) error {
	if len(resp.Results) == 0 {
		return fmt.Errorf("no result returned")
	}
	if resp.Results[0].Error != "" {
		return fmt.Errorf("%s", resp.Results[0].Error)
	}
	return nil
}

func (c *Client) CreateWarehouse(ctx context.Context, project, name string, spec WarehouseSpec) (*Warehouse, error) {
	manifest, err := marshalWarehouseManifest(project, name, spec)
	if err != nil {
		return nil, fmt.Errorf("encoding warehouse %q/%q: %w", project, name, err)
	}
	encoded := base64.StdEncoding.EncodeToString(manifest)

	var resp resourceResultResponse
	if err := c.Do(ctx, "CreateResource", map[string]string{"manifest": encoded}, &resp); err != nil {
		return nil, fmt.Errorf("creating warehouse %q/%q: %w", project, name, err)
	}
	if err := checkResourceResult(resp); err != nil {
		return nil, fmt.Errorf("creating warehouse %q/%q: %w", project, name, err)
	}

	return c.GetWarehouse(ctx, project, name)
}

// GetWarehouse returns (nil, nil) when the Warehouse exists but is being deleted.
func (c *Client) GetWarehouse(ctx context.Context, project, name string) (*Warehouse, error) {
	var resp getWarehouseResponse
	if err := c.Do(ctx, "GetWarehouse", map[string]string{"project": project, "name": name, "format": "RAW_FORMAT_JSON"}, &resp); err != nil {
		return nil, fmt.Errorf("getting warehouse %q/%q: %w", project, name, err)
	}
	var warehouse Warehouse
	if err := json.Unmarshal(resp.Raw, &warehouse); err != nil {
		return nil, fmt.Errorf("decoding warehouse %q/%q: %w", project, name, err)
	}
	if warehouse.Metadata.DeletionTimestamp != nil {
		return nil, nil
	}

	return &warehouse, nil
}

func (c *Client) ListWarehouseFreight(ctx context.Context, project, warehouse string) ([]Freight, error) {
	var resp queryFreightResponse
	req := map[string]any{
		"project": project,
		"origins": []string{warehouse},
	}
	if err := c.Do(ctx, "QueryFreight", req, &resp); err != nil {
		return nil, fmt.Errorf("listing freight for warehouse %q/%q: %w", project, warehouse, err)
	}

	group, ok := resp.Groups[""]
	if !ok {
		return []Freight{}, nil
	}
	return group.Freight, nil
}

func (c *Client) UpdateWarehouse(ctx context.Context, project, name string, spec WarehouseSpec) (*Warehouse, error) {
	manifest, err := marshalWarehouseManifest(project, name, spec)
	if err != nil {
		return nil, fmt.Errorf("encoding warehouse %q/%q: %w", project, name, err)
	}
	encoded := base64.StdEncoding.EncodeToString(manifest)

	var resp resourceResultResponse
	if err := c.updateWarehouseManifest(ctx, encoded, &resp); err != nil {
		return nil, fmt.Errorf("updating warehouse %q/%q: %w", project, name, err)
	}
	if err := checkResourceResult(resp); err != nil {
		return nil, fmt.Errorf("updating warehouse %q/%q: %w", project, name, err)
	}

	return c.GetWarehouse(ctx, project, name)
}

// Kargo reads the current resourceVersion before updating a manifest. Its
// controller can update status between those operations, requiring a fresh RPC.
func (c *Client) updateWarehouseManifest(ctx context.Context, manifest string, resp *resourceResultResponse) error {
	for attempt := 0; ; attempt++ {
		err := c.Do(ctx, "UpdateResource", map[string]string{"manifest": manifest}, resp)
		var apiErr *APIError
		if err == nil || attempt == 2 || !errors.As(err, &apiErr) || apiErr.HTTPStatus != 409 || !strings.Contains(apiErr.Message, "the object has been modified") {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) DeleteWarehouse(ctx context.Context, project, name string) error {
	if err := c.Do(ctx, "DeleteWarehouse", map[string]string{"project": project, "name": name}, nil); err != nil {
		return fmt.Errorf("deleting warehouse %q/%q: %w", project, name, err)
	}
	return nil
}

func parseJSONInt64(data []byte) (value int64, set bool, err error) {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" || raw == "{}" {
		return 0, false, nil
	}
	if strings.HasPrefix(raw, `"`) {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return 0, false, err
		}
		if value == "" {
			return 0, false, nil
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		return parsed, true, err
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	return parsed, true, err
}
