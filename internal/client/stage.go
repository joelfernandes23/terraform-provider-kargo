package client

import (
	"context"
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
func (c *Client) GetStage(_ context.Context, project, name string) (*Stage, error) {
	return nil, fmt.Errorf("stage client for %q/%q not implemented", project, name)
}
