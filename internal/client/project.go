package client

import (
	"context"
	"encoding/base64"
	"fmt"
)

type ProjectMetadata struct {
	Name              string            `json:"name"`
	UID               string            `json:"uid,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	DeletionTimestamp any               `json:"deletionTimestamp,omitempty"`
}

type ProjectStatus struct {
	Conditions []ProjectCondition `json:"conditions,omitempty"`
}

type ProjectCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type Project struct {
	Metadata ProjectMetadata `json:"metadata"`
	Status   ProjectStatus   `json:"status,omitempty"`
}

type getProjectResponse struct {
	Project Project `json:"project"`
}

type createResourceResponse struct {
	Results []struct {
		CreatedResourceManifest string `json:"createdResourceManifest"`
	} `json:"results"`
}

func (c *Client) CreateProject(ctx context.Context, name string) (*Project, error) {
	manifest := fmt.Sprintf(`apiVersion: kargo.akuity.io/v1alpha1
kind: Project
metadata:
  name: %s`, name)

	encoded := base64.StdEncoding.EncodeToString([]byte(manifest))

	var resp createResourceResponse
	if err := c.Do(ctx, "CreateResource", map[string]string{"manifest": encoded}, &resp); err != nil {
		return nil, fmt.Errorf("creating project %q: %w", name, err)
	}

	return c.GetProject(ctx, name)
}

func (c *Client) GetProject(ctx context.Context, name string) (*Project, error) {
	var resp getProjectResponse
	if err := c.Do(ctx, "GetProject", map[string]string{"name": name}, &resp); err != nil {
		return nil, fmt.Errorf("getting project %q: %w", name, err)
	}

	// project is being deleted
	if resp.Project.Metadata.DeletionTimestamp != nil {
		return nil, nil
	}

	return &resp.Project, nil
}

func (c *Client) DeleteProject(ctx context.Context, name string) error {
	if err := c.Do(ctx, "DeleteProject", map[string]string{"name": name}, nil); err != nil {
		return fmt.Errorf("deleting project %q: %w", name, err)
	}
	return nil
}
