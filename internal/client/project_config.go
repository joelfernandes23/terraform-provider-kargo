package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type ProjectConfig struct {
	Metadata ProjectConfigMetadata `json:"metadata"`
	Spec     ProjectConfigSpec     `json:"spec,omitempty"`
	Status   ProjectConfigStatus   `json:"status,omitempty"`
}

type ProjectConfigMetadata struct {
	Name              string  `json:"name"`
	Namespace         string  `json:"namespace,omitempty"`
	DeletionTimestamp *string `json:"deletionTimestamp,omitempty"`
}

type ProjectConfigSpec struct {
	PromotionPolicies []PromotionPolicy       `json:"promotionPolicies,omitempty"`
	WebhookReceivers  []WebhookReceiverConfig `json:"webhookReceivers,omitempty"`
}

type PromotionPolicy struct {
	StageSelector        PromotionPolicySelector `json:"stageSelector"`
	AutoPromotionEnabled bool                    `json:"autoPromotionEnabled,omitempty"`
}

type PromotionPolicySelector struct {
	Name             string                     `json:"name,omitempty"`
	MatchLabels      map[string]string          `json:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

type LabelSelector struct {
	MatchLabels      map[string]string          `json:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

type LabelSelectorRequirement struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

type WebhookReceiverConfig struct {
	Name        string                        `json:"name"`
	Bitbucket   *WebhookSecretRefConfig       `json:"bitbucket,omitempty"`
	DockerHub   *WebhookSecretRefConfig       `json:"dockerhub,omitempty"`
	GitHub      *WebhookSecretRefConfig       `json:"github,omitempty"`
	GitLab      *WebhookSecretRefConfig       `json:"gitlab,omitempty"`
	Harbor      *WebhookSecretRefConfig       `json:"harbor,omitempty"`
	Quay        *WebhookSecretRefConfig       `json:"quay,omitempty"`
	Artifactory *ArtifactoryWebhookConfig     `json:"artifactory,omitempty"`
	Azure       *WebhookSecretRefConfig       `json:"azure,omitempty"`
	Gitea       *WebhookSecretRefConfig       `json:"gitea,omitempty"`
	Generic     *GenericWebhookReceiverConfig `json:"generic,omitempty"`
}

type WebhookSecretRefConfig struct {
	SecretRef LocalObjectReference `json:"secretRef"`
}

type ArtifactoryWebhookConfig struct {
	SecretRef       LocalObjectReference `json:"secretRef"`
	VirtualRepoName string               `json:"virtualRepoName,omitempty"`
}

type LocalObjectReference struct {
	Name string `json:"name"`
}

type GenericWebhookReceiverConfig struct {
	SecretRef LocalObjectReference   `json:"secretRef"`
	Actions   []GenericWebhookAction `json:"actions"`
}

type GenericWebhookAction struct {
	Action                  string                                  `json:"action"`
	WhenExpression          string                                  `json:"whenExpression,omitempty"`
	Parameters              map[string]string                       `json:"parameters,omitempty"`
	TargetSelectionCriteria []GenericWebhookTargetSelectionCriteria `json:"targetSelectionCriteria"`
}

type GenericWebhookTargetSelectionCriteria struct {
	Kind          string         `json:"kind"`
	Name          string         `json:"name,omitempty"`
	LabelSelector *LabelSelector `json:"labelSelector,omitempty"`
	IndexSelector *IndexSelector `json:"indexSelector,omitempty"`
}

type IndexSelector struct {
	MatchIndices []IndexSelectorRequirement `json:"matchIndices"`
}

type IndexSelectorRequirement struct {
	Key      string `json:"key"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type ProjectConfigStatus struct {
	WebhookReceivers []WebhookReceiverDetails `json:"webhookReceivers,omitempty"`
}

type WebhookReceiverDetails struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
	URL  string `json:"url,omitempty"`
}

func (c *Client) GetProjectConfig(ctx context.Context, project string) (*ProjectConfig, error) {
	var resp struct {
		Raw []byte `json:"raw"`
	}
	if err := c.Do(ctx, "GetProjectConfig", map[string]string{
		"project": project,
		"format":  "RAW_FORMAT_JSON",
	}, &resp); err != nil {
		return nil, fmt.Errorf("getting project config %q: %w", project, err)
	}

	var config ProjectConfig
	if err := json.Unmarshal(resp.Raw, &config); err != nil {
		return nil, fmt.Errorf("decoding project config %q manifest: %w", project, err)
	}
	if config.Metadata.DeletionTimestamp != nil {
		return nil, nil
	}
	return &config, nil
}

type projectConfigManifest struct {
	APIVersion string                `json:"apiVersion"`
	Kind       string                `json:"kind"`
	Metadata   ProjectConfigMetadata `json:"metadata"`
	Spec       ProjectConfigSpec     `json:"spec"`
}

func marshalProjectConfigManifest(project string, spec ProjectConfigSpec) ([]byte, error) {
	if project == "" {
		return nil, fmt.Errorf("project must not be empty")
	}
	return json.Marshal(projectConfigManifest{
		APIVersion: "kargo.akuity.io/v1alpha1",
		Kind:       "ProjectConfig",
		Metadata: ProjectConfigMetadata{
			Name:      project,
			Namespace: project,
		},
		Spec: spec,
	})
}

func (c *Client) CreateProjectConfig(ctx context.Context, project string, spec ProjectConfigSpec) (*ProjectConfig, error) {
	return c.writeProjectConfig(ctx, "CreateResource", "creating", project, spec)
}

func (c *Client) UpdateProjectConfig(ctx context.Context, project string, spec ProjectConfigSpec) (*ProjectConfig, error) {
	return c.writeProjectConfig(ctx, "UpdateResource", "updating", project, spec)
}

func (c *Client) writeProjectConfig(
	ctx context.Context,
	method string,
	operation string,
	project string,
	spec ProjectConfigSpec,
) (*ProjectConfig, error) {
	manifest, err := marshalProjectConfigManifest(project, spec)
	if err != nil {
		return nil, fmt.Errorf("%s project config %q manifest: %w", operation, project, err)
	}

	var resp resourceResultResponse
	if err := c.Do(ctx, method, map[string]string{
		"manifest": base64.StdEncoding.EncodeToString(manifest),
	}, &resp); err != nil {
		return nil, fmt.Errorf("%s project config %q: %w", operation, project, err)
	}
	if err := checkResourceResult(resp); err != nil {
		return nil, fmt.Errorf("%s project config %q: %w", operation, project, err)
	}
	return c.GetProjectConfig(ctx, project)
}

func (c *Client) DeleteProjectConfig(ctx context.Context, project string) error {
	if err := c.Do(ctx, "DeleteProjectConfig", map[string]string{"project": project}, nil); err != nil {
		return fmt.Errorf("deleting project config %q: %w", project, err)
	}
	return nil
}
