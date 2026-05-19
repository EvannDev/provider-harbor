// Copyright 2026 The Crossplane Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package project provides Harbor project client utilities and helpers.
package project

import (
	"context"
	"net/http"
	"strconv"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	goopenapiruntime "github.com/go-openapi/runtime"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/client/project"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/models"

	v1alpha1 "github.com/EvannDev/provider-harbor/apis/project/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/clients/common"
	"github.com/EvannDev/provider-harbor/internal/helpers"
)

// ProjectsClient defines the Harbor project operations needed by the
// controller.
type ProjectsClient interface {
	// GetProject fetches a Harbor project by name or ID.
	GetProject(ctx context.Context, nameOrID string) (*models.Project, error)
	// NewProject creates a new Harbor project.
	NewProject(ctx context.Context, projectReq *models.ProjectReq) error
	// UpdateProject applies changes to an existing Harbor project by name or ID.
	UpdateProject(ctx context.Context, nameOrID string, projectReq *models.ProjectReq) error
	// DeleteProject removes a Harbor project by name or ID.
	DeleteProject(ctx context.Context, nameOrID string) error
}

// projectClient wraps the Harbor v2 project API.
type projectClient struct {
	api *project.Client
}

// NewProjectsClient returns a ProjectsClient backed by the official Harbor SDK.
func NewProjectsClient(cfg common.Config) ProjectsClient {
	return &projectClient{api: common.NewHarborAPI(cfg).Project}
}

// GetProject fetches a Harbor project by name or ID.
func (c *projectClient) GetProject(ctx context.Context, nameOrID string) (*models.Project, error) {
	resp, err := c.api.GetProject(ctx, &project.GetProjectParams{ProjectNameOrID: nameOrID})
	if err != nil {
		return nil, errors.Wrap(err, "cannot get project")
	}

	return resp.Payload, nil
}

// NewProject creates a new Harbor project.
func (c *projectClient) NewProject(ctx context.Context, projectReq *models.ProjectReq) error {
	_, err := c.api.CreateProject(ctx, &project.CreateProjectParams{Project: projectReq})

	return errors.Wrap(err, "cannot create project")
}

// UpdateProject applies changes to an existing Harbor project by name or ID.
func (c *projectClient) UpdateProject(ctx context.Context, nameOrID string, projectReq *models.ProjectReq) error {
	_, err := c.api.UpdateProject(ctx, &project.UpdateProjectParams{
		ProjectNameOrID: nameOrID,
		Project:         projectReq,
	})

	return errors.Wrap(err, "cannot update project")
}

// DeleteProject removes a Harbor project by name or ID.
func (c *projectClient) DeleteProject(ctx context.Context, nameOrID string) error {
	_, err := c.api.DeleteProject(ctx, &project.DeleteProjectParams{ProjectNameOrID: nameOrID})

	return errors.Wrap(err, "cannot delete project")
}

// IsProjectNotFound reports whether err is a Harbor 404 project-not-found
// response.
func IsProjectNotFound(err error) bool {
	if err == nil {
		return false
	}

	var deleteNotFound *project.DeleteProjectNotFound
	if errors.As(err, &deleteNotFound) {
		return true
	}

	var apiErr *goopenapiruntime.APIError

	return errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound
}

// GenerateProjectObservation converts a Harbor Project into a
// ProjectObservation.
func GenerateProjectObservation(proj *models.Project) v1alpha1.ProjectObservation {
	if proj == nil {
		return v1alpha1.ProjectObservation{}
	}

	id := proj.ProjectID
	repoCount := proj.RepoCount

	return v1alpha1.ProjectObservation{
		ProjectID: &id,
		OwnerName: &proj.OwnerName,
		RepoCount: &repoCount,
	}
}

// IsProjectUpToDate returns true if the observed Harbor project matches the
// desired spec.
func IsProjectUpToDate(params v1alpha1.ProjectParameters, proj *models.Project) bool {
	if params.StorageLimit != nil {
		return false
	}

	if params.Metadata == nil {
		return true
	}

	if proj.Metadata == nil {
		return false
	}

	return isMetadataUpToDate(params.Metadata, proj.Metadata)
}

// isMetadataUpToDate checks if project metadata fields match desired state.
func isMetadataUpToDate(desired *v1alpha1.ProjectMetadataParameters, observed *models.ProjectMetadata) bool {
	return helpers.IsStringifiedBoolEqual(desired.Public, &observed.Public) &&
		helpers.IsStringifiedBoolEqual(desired.AutoScan, observed.AutoScan) &&
		helpers.IsStringifiedBoolEqual(desired.EnableContentTrust, observed.EnableContentTrust) &&
		helpers.IsStringifiedBoolEqual(desired.EnableContentTrustCosign, observed.EnableContentTrustCosign) &&
		helpers.IsStringifiedBoolEqual(desired.PreventVulnerable, observed.PreventVul) &&
		(desired.Severity == nil || helpers.IsComparablePtrEqualComparablePtr(desired.Severity, observed.Severity)) &&
		helpers.IsStringifiedBoolEqual(desired.ReuseSysCVEAllowlist, observed.ReuseSysCVEAllowlist)
}

// ToProjectReq converts CR parameters into a Harbor ProjectReq for creation
// or update.
func ToProjectReq(name string, params v1alpha1.ProjectParameters) *models.ProjectReq {
	req := &models.ProjectReq{
		ProjectName:  name,
		StorageLimit: params.StorageLimit,
	}

	if params.Metadata != nil {
		req.Metadata = ToHarborProjectMetadata(params.Metadata)
	}

	return req
}

// ToHarborProjectMetadata converts typed booleans into the string-based
// ProjectMetadata Harbor expects.
func ToHarborProjectMetadata(meta *v1alpha1.ProjectMetadataParameters) *models.ProjectMetadata {
	result := &models.ProjectMetadata{}

	if meta.Public != nil {
		result.Public = strconv.FormatBool(*meta.Public)
	}

	if meta.AutoScan != nil {
		result.AutoScan = new(string)
		*result.AutoScan = strconv.FormatBool(*meta.AutoScan)
	}

	if meta.EnableContentTrust != nil {
		result.EnableContentTrust = new(string)
		*result.EnableContentTrust = strconv.FormatBool(*meta.EnableContentTrust)
	}

	if meta.EnableContentTrustCosign != nil {
		result.EnableContentTrustCosign = new(string)
		*result.EnableContentTrustCosign = strconv.FormatBool(*meta.EnableContentTrustCosign)
	}

	if meta.PreventVulnerable != nil {
		result.PreventVul = new(string)
		*result.PreventVul = strconv.FormatBool(*meta.PreventVulnerable)
	}

	if meta.Severity != nil {
		result.Severity = meta.Severity
	}

	if meta.ReuseSysCVEAllowlist != nil {
		result.ReuseSysCVEAllowlist = new(string)
		*result.ReuseSysCVEAllowlist = strconv.FormatBool(*meta.ReuseSysCVEAllowlist)
	}

	return result
}
