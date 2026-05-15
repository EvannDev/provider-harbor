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
	"strconv"

	apiv2 "github.com/mittwald/goharbor-client/v5/apiv2"
	modelv2 "github.com/mittwald/goharbor-client/v5/apiv2/model"

	v1alpha1 "github.com/EvannDev/provider-harbor/apis/project/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/helpers"
)

// ProjectsClient defines the Harbor project operations needed by the
// controller.
type ProjectsClient interface {
	// GetProject fetches a Harbor project by name.
	GetProject(ctx context.Context, projectName string) (*modelv2.Project, error)
	// NewProject creates a new Harbor project.
	NewProject(ctx context.Context, projectReq *modelv2.ProjectReq) error
	// UpdateProject applies changes to an existing Harbor project.
	UpdateProject(ctx context.Context, project *modelv2.Project, storageLimit *int64) error
	// DeleteProject removes a Harbor project by name.
	DeleteProject(ctx context.Context, projectName string) error
}

// NewProjectsClient returns the apiv2.Client as a ProjectsClient.
// apiv2.Client implements all ProjectsClient methods directly.
func NewProjectsClient(cl apiv2.Client) ProjectsClient {
	return cl
}

// GenerateProjectObservation converts a Harbor Project into a
// ProjectObservation.
func GenerateProjectObservation(proj *modelv2.Project) v1alpha1.ProjectObservation {
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
func IsProjectUpToDate(params v1alpha1.ProjectParameters, proj *modelv2.Project) bool {
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
func isMetadataUpToDate(desired *v1alpha1.ProjectMetadataParameters, observed *modelv2.ProjectMetadata) bool {
	return helpers.IsStringifiedBoolEqual(desired.Public, &observed.Public) &&
		helpers.IsStringifiedBoolEqual(desired.AutoScan, observed.AutoScan) &&
		helpers.IsStringifiedBoolEqual(desired.EnableContentTrust, observed.EnableContentTrust) &&
		helpers.IsStringifiedBoolEqual(desired.EnableContentTrustCosign, observed.EnableContentTrustCosign) &&
		helpers.IsStringifiedBoolEqual(desired.PreventVulnerable, observed.PreventVul) &&
		(desired.Severity == nil || helpers.IsComparablePtrEqualComparablePtr(desired.Severity, observed.Severity)) &&
		helpers.IsStringifiedBoolEqual(desired.ReuseSysCVEAllowlist, observed.ReuseSysCVEAllowlist)
}

// ToProjectReq converts CR parameters into a Harbor ProjectReq for creation.
func ToProjectReq(name string, params v1alpha1.ProjectParameters) *modelv2.ProjectReq {
	req := &modelv2.ProjectReq{
		ProjectName:  name,
		StorageLimit: params.StorageLimit,
	}

	if params.Metadata != nil {
		req.Metadata = ToHarborProjectMetadata(params.Metadata)
	}

	return req
}

// ApplyProjectParameters writes desired state from ForProvider onto an
// existing Harbor Project object.
func ApplyProjectParameters(params v1alpha1.ProjectParameters, proj *modelv2.Project) {
	if proj.Metadata == nil {
		proj.Metadata = &modelv2.ProjectMetadata{}
	}

	if params.Metadata != nil {
		desired := ToHarborProjectMetadata(params.Metadata)
		proj.Metadata.Public = desired.Public
		proj.Metadata.AutoScan = desired.AutoScan
		proj.Metadata.EnableContentTrust = desired.EnableContentTrust
		proj.Metadata.EnableContentTrustCosign = desired.EnableContentTrustCosign
		proj.Metadata.PreventVul = desired.PreventVul
		proj.Metadata.Severity = desired.Severity
		proj.Metadata.ReuseSysCVEAllowlist = desired.ReuseSysCVEAllowlist
	}
}

// ToHarborProjectMetadata converts typed booleans into the string-based
// ProjectMetadata Harbor expects.
func ToHarborProjectMetadata(meta *v1alpha1.ProjectMetadataParameters) *modelv2.ProjectMetadata {
	result := &modelv2.ProjectMetadata{}

	if meta.Public != nil {
		result.Public = strconv.FormatBool(*meta.Public)
	}

	if meta.AutoScan != nil {
		result.AutoScan = new(strconv.FormatBool(*meta.AutoScan))
	}

	if meta.EnableContentTrust != nil {
		result.EnableContentTrust = new(strconv.FormatBool(*meta.EnableContentTrust))
	}

	if meta.EnableContentTrustCosign != nil {
		result.EnableContentTrustCosign = new(strconv.FormatBool(*meta.EnableContentTrustCosign))
	}

	if meta.PreventVulnerable != nil {
		result.PreventVul = new(strconv.FormatBool(*meta.PreventVulnerable))
	}

	if meta.Severity != nil {
		result.Severity = meta.Severity
	}

	if meta.ReuseSysCVEAllowlist != nil {
		result.ReuseSysCVEAllowlist = new(strconv.FormatBool(*meta.ReuseSysCVEAllowlist))
	}

	return result
}
