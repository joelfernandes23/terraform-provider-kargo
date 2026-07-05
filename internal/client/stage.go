package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type StageMetadata struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace,omitempty"`
	UID               string            `json:"uid,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	DeletionTimestamp *string           `json:"deletionTimestamp,omitempty"`
}

type Stage struct {
	Metadata StageMetadata `json:"metadata"`
	Spec     StageSpec     `json:"spec,omitempty"`
}

type StageSpec struct {
	Shard             string             `json:"shard,omitempty"`
	RequestedFreight  []FreightRequest   `json:"requestedFreight"`
	PromotionTemplate *PromotionTemplate `json:"promotionTemplate,omitempty"`
}

type FreightRequest struct {
	Origin  FreightOrigin  `json:"origin"`
	Sources FreightSources `json:"sources"`
}

type FreightSources struct {
	Direct bool     `json:"direct,omitempty"`
	Stages []string `json:"stages,omitempty"`
}

type PromotionTemplate struct {
	Spec PromotionTemplateSpec `json:"spec"`
}

type PromotionTemplateSpec struct {
	Steps []PromotionStep `json:"steps"`
}

type PromotionStep struct {
	Uses   string          `json:"uses,omitempty"`
	As     string          `json:"as,omitempty"`
	Config json.RawMessage `json:"config,omitempty"`
}

// GetStage returns (nil, nil) when the Stage exists but is being deleted.
//
// It reads with format RAW_FORMAT_JSON (base64 of the full k8s JSON manifest) because
// the structured response protojson-encodes promotionTemplate step config as
// {"raw": "<base64>"} instead of inline JSON.
func (c *Client) GetStage(ctx context.Context, project, name string) (*Stage, error) {
	var resp struct {
		Raw []byte `json:"raw"` // encoding/json base64-decodes into []byte automatically
	}
	req := map[string]string{"project": project, "name": name, "format": "RAW_FORMAT_JSON"}
	if err := c.Do(ctx, "GetStage", req, &resp); err != nil {
		return nil, fmt.Errorf("getting stage %q/%q: %w", project, name, err)
	}

	var stage Stage
	if err := json.Unmarshal(resp.Raw, &stage); err != nil {
		return nil, fmt.Errorf("decoding stage %q/%q manifest: %w", project, name, err)
	}

	if stage.Metadata.DeletionTimestamp != nil {
		return nil, nil
	}

	return &stage, nil
}

type stageManifest struct {
	APIVersion string        `json:"apiVersion"`
	Kind       string        `json:"kind"`
	Metadata   StageMetadata `json:"metadata"`
	Spec       StageSpec     `json:"spec"`
}

func marshalStageManifest(project, name string, spec StageSpec) ([]byte, error) {
	return json.Marshal(stageManifest{
		APIVersion: "kargo.akuity.io/v1alpha1",
		Kind:       "Stage",
		Metadata: StageMetadata{
			Name:      name,
			Namespace: project,
		},
		Spec: spec,
	})
}

func (c *Client) CreateStage(ctx context.Context, project, name string, spec StageSpec) (*Stage, error) {
	manifest, err := marshalStageManifest(project, name, spec)
	if err != nil {
		return nil, fmt.Errorf("creating stage %q/%q manifest: %w", project, name, err)
	}

	encoded := base64.StdEncoding.EncodeToString(manifest)

	var resp resourceResultResponse
	if err := c.Do(ctx, "CreateResource", map[string]string{"manifest": encoded}, &resp); err != nil {
		return nil, fmt.Errorf("creating stage %q/%q: %w", project, name, err)
	}
	if err := checkResourceResult(resp); err != nil {
		return nil, fmt.Errorf("creating stage %q/%q: %w", project, name, err)
	}

	return c.GetStage(ctx, project, name)
}

func (c *Client) UpdateStage(ctx context.Context, project, name string, spec StageSpec) (*Stage, error) {
	manifest, err := marshalStageManifest(project, name, spec)
	if err != nil {
		return nil, fmt.Errorf("updating stage %q/%q manifest: %w", project, name, err)
	}

	encoded := base64.StdEncoding.EncodeToString(manifest)

	var resp resourceResultResponse
	if err := c.Do(ctx, "UpdateResource", map[string]string{"manifest": encoded}, &resp); err != nil {
		return nil, fmt.Errorf("updating stage %q/%q: %w", project, name, err)
	}
	if err := checkResourceResult(resp); err != nil {
		return nil, fmt.Errorf("updating stage %q/%q: %w", project, name, err)
	}

	return c.GetStage(ctx, project, name)
}

// DeleteStage does not swallow not-found: Kargo propagates the k8s 404 as a
// connect not_found error, which callers detect via IsNotFound.
func (c *Client) DeleteStage(ctx context.Context, project, name string) error {
	if err := c.Do(ctx, "DeleteStage", map[string]string{"project": project, "name": name}, nil); err != nil {
		return fmt.Errorf("deleting stage %q/%q: %w", project, name, err)
	}
	return nil
}
