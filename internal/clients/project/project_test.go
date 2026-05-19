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

package project

import (
	"reflect"
	"testing"

	"github.com/goharbor/go-client/pkg/sdk/v2.0/models"

	v1alpha1 "github.com/EvannDev/provider-harbor/apis/project/v1alpha1"
)

const (
	// boolFalseStr is the string representation of boolean false.
	boolFalseStr = "false"
	// boolTrueStr is the string representation of boolean true.
	boolTrueStr = "true"
	// projectNameMyProject is a test project name.
	projectNameMyProject = "my-project"
	// projectNameOther is an alternate test project name.
	projectNameOther = "other-project"
)

// TestGenerateProjectObservation verifies that GenerateProjectObservation
// converts a models.Project into a v1alpha1.ProjectObservation correctly,
// and returns a zero value when the input is nil.
func TestGenerateProjectObservation(t *testing.T) {
	t.Parallel()

	ownerName := "alice"

	var projectID int32 = 42

	var repoCount int64 = 7

	cases := []struct {
		name string
		proj *models.Project
		want v1alpha1.ProjectObservation
	}{
		{
			name: "nil project returns zero observation",
			proj: nil,
			want: v1alpha1.ProjectObservation{},
		},
		{
			name: "populated project maps all fields",
			proj: &models.Project{
				ProjectID: projectID,
				OwnerName: ownerName,
				RepoCount: repoCount,
			},
			want: v1alpha1.ProjectObservation{
				ProjectID: &projectID,
				OwnerName: &ownerName,
				RepoCount: &repoCount,
			},
		},
		{
			name: "zero-value project maps zero fields",
			proj: &models.Project{},
			want: v1alpha1.ProjectObservation{
				ProjectID: new(int32(0)),
				OwnerName: new(""),
				RepoCount: new(int64(0)),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := GenerateProjectObservation(tc.proj)

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("GenerateProjectObservation() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestIsProjectUpToDate verifies all branches of IsProjectUpToDate,
// including the StorageLimit early-return, nil metadata short-circuits,
// and each individual metadata field mismatch via isMetadataUpToDate.
func TestIsProjectUpToDate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		params v1alpha1.ProjectParameters
		proj   *models.Project
		want   bool
	}{
		// StorageLimit branch — always returns false when set.
		{
			name: "StorageLimit non-nil returns false",
			params: v1alpha1.ProjectParameters{
				StorageLimit: new(int64(1024)),
			},
			proj: &models.Project{
				Metadata: &models.ProjectMetadata{},
			},
			want: false,
		},
		// Nil metadata in params — nothing to check, always up to date.
		{
			name: "nil params.Metadata returns true",
			params: v1alpha1.ProjectParameters{
				Metadata: nil,
			},
			proj: &models.Project{
				Metadata: &models.ProjectMetadata{},
			},
			want: true,
		},
		// Nil metadata on the project — always out of date.
		{
			name: "nil proj.Metadata with desired meta returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{},
			},
			proj: &models.Project{Metadata: nil},
			want: false,
		},
		// All metadata fields nil in desired → all helpers return true.
		{
			name: "all desired fields nil returns true",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{},
			},
			proj: &models.Project{
				Metadata: &models.ProjectMetadata{
					Public:                   boolFalseStr,
					AutoScan:                 new(boolFalseStr),
					EnableContentTrust:       new(boolFalseStr),
					EnableContentTrustCosign: new(boolFalseStr),
					PreventVul:               new(boolFalseStr),
					Severity:                 new("low"),
					ReuseSysCVEAllowlist:     new(boolFalseStr),
				},
			},
			want: true,
		},
		// All fields match exactly → true.
		{
			name: "all metadata fields match returns true",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					Public:                   new(true),
					AutoScan:                 new(false),
					EnableContentTrust:       new(false),
					EnableContentTrustCosign: new(false),
					PreventVulnerable:        new(true),
					Severity:                 new("high"),
					ReuseSysCVEAllowlist:     new(true),
				},
			},
			proj: &models.Project{
				Metadata: &models.ProjectMetadata{
					Public:                   boolTrueStr,
					AutoScan:                 new(boolFalseStr),
					EnableContentTrust:       new(boolFalseStr),
					EnableContentTrustCosign: new(boolFalseStr),
					PreventVul:               new(boolTrueStr),
					Severity:                 new("high"),
					ReuseSysCVEAllowlist:     new(boolTrueStr),
				},
			},
			want: true,
		},
		// Public mismatch: desired true, observed "false".
		{
			name: "Public mismatch returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					Public: new(true),
				},
			},
			proj: &models.Project{
				Metadata: &models.ProjectMetadata{
					Public: boolFalseStr,
				},
			},
			want: false,
		},
		// AutoScan mismatch: desired true, observed "false".
		{
			name: "AutoScan mismatch returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					AutoScan: new(true),
				},
			},
			proj: &models.Project{
				Metadata: &models.ProjectMetadata{
					AutoScan: new(boolFalseStr),
				},
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := IsProjectUpToDate(tc.params, tc.proj)

			if got != tc.want {
				t.Errorf(
					"IsProjectUpToDate() = %v, want %v (case: %s)",
					got, tc.want, tc.name,
				)
			}
		})
	}
}

// TestToProjectReq verifies that ToProjectReq constructs a models.ProjectReq
// with the correct project name, storage limit, and optional metadata.
func TestToProjectReq(t *testing.T) {
	t.Parallel()

	storageLimit := int64(2048)

	cases := []struct {
		name    string
		reqName string
		params  v1alpha1.ProjectParameters
		want    *models.ProjectReq
	}{
		{
			name:    "nil metadata produces req without metadata",
			reqName: projectNameMyProject,
			params: v1alpha1.ProjectParameters{
				StorageLimit: &storageLimit,
				Metadata:     nil,
			},
			want: &models.ProjectReq{
				ProjectName:  projectNameMyProject,
				StorageLimit: &storageLimit,
				Metadata:     nil,
			},
		},
		{
			name:    "populated metadata is included in req",
			reqName: projectNameOther,
			params: v1alpha1.ProjectParameters{
				StorageLimit: nil,
				Metadata: &v1alpha1.ProjectMetadataParameters{
					Public:   new(true),
					AutoScan: new(false),
				},
			},
			want: &models.ProjectReq{
				ProjectName:  projectNameOther,
				StorageLimit: nil,
				Metadata: &models.ProjectMetadata{
					Public:   boolTrueStr,
					AutoScan: new(boolFalseStr),
				},
			},
		},
		{
			name:    "empty name with nil params produces minimal req",
			reqName: "",
			params:  v1alpha1.ProjectParameters{},
			want: &models.ProjectReq{
				ProjectName:  "",
				StorageLimit: nil,
				Metadata:     nil,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ToProjectReq(tc.reqName, tc.params)

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ToProjectReq() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestToHarborProjectMetadata verifies that ToHarborProjectMetadata converts
// each field of ProjectMetadataParameters into the string-based
// models.ProjectMetadata Harbor expects, skipping nil fields.
func TestToHarborProjectMetadata(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		meta *v1alpha1.ProjectMetadataParameters
		want *models.ProjectMetadata
	}{
		{
			name: "all fields nil returns empty metadata",
			meta: &v1alpha1.ProjectMetadataParameters{},
			want: &models.ProjectMetadata{},
		},
		{
			name: "Public true is converted to string true",
			meta: &v1alpha1.ProjectMetadataParameters{
				Public: new(true),
			},
			want: &models.ProjectMetadata{
				Public: boolTrueStr,
			},
		},
		{
			name: "AutoScan true is converted to pointer to string true",
			meta: &v1alpha1.ProjectMetadataParameters{
				AutoScan: new(true),
			},
			want: &models.ProjectMetadata{
				AutoScan: new(boolTrueStr),
			},
		},
		{
			name: "PreventVulnerable true maps to PreventVul string true",
			meta: &v1alpha1.ProjectMetadataParameters{
				PreventVulnerable: new(true),
			},
			want: &models.ProjectMetadata{
				PreventVul: new(boolTrueStr),
			},
		},
		{
			name: "Severity non-nil is passed through unchanged",
			meta: &v1alpha1.ProjectMetadataParameters{
				Severity: new("medium"),
			},
			want: &models.ProjectMetadata{
				Severity: new("medium"),
			},
		},
		{
			name: "all fields set produces fully populated metadata",
			meta: &v1alpha1.ProjectMetadataParameters{
				Public:                   new(true),
				AutoScan:                 new(true),
				EnableContentTrust:       new(false),
				EnableContentTrustCosign: new(false),
				PreventVulnerable:        new(true),
				Severity:                 new("critical"),
				ReuseSysCVEAllowlist:     new(false),
			},
			want: &models.ProjectMetadata{
				Public:                   boolTrueStr,
				AutoScan:                 new(boolTrueStr),
				EnableContentTrust:       new(boolFalseStr),
				EnableContentTrustCosign: new(boolFalseStr),
				PreventVul:               new(boolTrueStr),
				Severity:                 new("critical"),
				ReuseSysCVEAllowlist:     new(boolFalseStr),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ToHarborProjectMetadata(tc.meta)

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf(
					"ToHarborProjectMetadata() = %+v, want %+v",
					got, tc.want,
				)
			}
		})
	}
}
