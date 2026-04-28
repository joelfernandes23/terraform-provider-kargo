package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	Conditions []WarehouseCondition `json:"conditions,omitempty"`
}

type WarehouseCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

type Warehouse struct {
	Metadata WarehouseMetadata `json:"metadata"`
	Spec     WarehouseSpec     `json:"spec,omitempty"`
	Status   WarehouseStatus   `json:"status,omitempty"`
}

type WarehouseSpec struct {
	Subscriptions []WarehouseSubscription `json:"subscriptions"`
}

type WarehouseSubscription struct {
	Image *ImageSubscription `json:"image,omitempty"`
	Git   *GitSubscription   `json:"git,omitempty"`
	Chart *ChartSubscription `json:"chart,omitempty"`
}

type ImageSubscription struct {
	RepoURL                string `json:"repoURL"`
	Constraint             string `json:"constraint,omitempty"`
	ImageSelectionStrategy string `json:"imageSelectionStrategy,omitempty"`
	Platform               string `json:"platform,omitempty"`
}

type GitSubscription struct {
	RepoURL                 string `json:"repoURL"`
	Branch                  string `json:"branch,omitempty"`
	CommitSelectionStrategy string `json:"commitSelectionStrategy,omitempty"`
	SemverConstraint        string `json:"semverConstraint,omitempty"`
}

type ChartSubscription struct {
	RepoURL          string `json:"repoURL"`
	Name             string `json:"name,omitempty"`
	SemverConstraint string `json:"semverConstraint,omitempty"`
}

type getWarehouseResponse struct {
	Warehouse Warehouse `json:"warehouse"`
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
		return nil, fmt.Errorf("creating warehouse %q/%q manifest: %w", project, name, err)
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
	if err := c.Do(ctx, "GetWarehouse", map[string]string{"project": project, "name": name}, &resp); err != nil {
		return nil, fmt.Errorf("getting warehouse %q/%q: %w", project, name, err)
	}

	if resp.Warehouse.Metadata.DeletionTimestamp != nil {
		return nil, nil
	}

	return &resp.Warehouse, nil
}

func (c *Client) UpdateWarehouse(ctx context.Context, project, name string, spec WarehouseSpec) (*Warehouse, error) {
	manifest, err := marshalWarehouseManifest(project, name, spec)
	if err != nil {
		return nil, fmt.Errorf("updating warehouse %q/%q manifest: %w", project, name, err)
	}

	encoded := base64.StdEncoding.EncodeToString(manifest)

	var resp resourceResultResponse
	if err := c.Do(ctx, "UpdateResource", map[string]string{"manifest": encoded}, &resp); err != nil {
		return nil, fmt.Errorf("updating warehouse %q/%q: %w", project, name, err)
	}
	if err := checkResourceResult(resp); err != nil {
		return nil, fmt.Errorf("updating warehouse %q/%q: %w", project, name, err)
	}

	return c.GetWarehouse(ctx, project, name)
}

func (c *Client) DeleteWarehouse(ctx context.Context, project, name string) error {
	if err := c.Do(ctx, "DeleteWarehouse", map[string]string{"project": project, "name": name}, nil); err != nil {
		return fmt.Errorf("deleting warehouse %q/%q: %w", project, name, err)
	}
	return nil
}
