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

	apiv2 "github.com/mittwald/goharbor-client/v5/apiv2"
	modelv2 "github.com/mittwald/goharbor-client/v5/apiv2/model"

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
	// testStrHarbor is a generic test string value.
	testStrHarbor = "harbor"
)

// fakeHarborClient embeds a nil *apiv2.RESTClient to satisfy apiv2.Client
// without requiring a real connection. No methods are called; it is used
// only to verify that NewProjectsClient returns the same value passed in.
type fakeHarborClient struct {
	*apiv2.RESTClient
}

// boolPtr returns a pointer to the given bool value.
func boolPtr(val bool) *bool { return &val }

// int32Ptr returns a pointer to the given int32 value.
func int32Ptr(val int32) *int32 { return &val }

// int64Ptr returns a pointer to the given int64 value.
func int64Ptr(val int64) *int64 { return &val }

// stringPtr returns a pointer to the given string value.
func stringPtr(str string) *string { return &str }

// TestNewProjectsClient verifies that NewProjectsClient returns the
// apiv2.Client argument unchanged and that the returned value satisfies
// the ProjectsClient interface.
func TestNewProjectsClient(t *testing.T) {
	t.Parallel()

	cl := &fakeHarborClient{}
	got := NewProjectsClient(cl)

	if got != cl {
		t.Errorf("NewProjectsClient() = %v, want %v", got, cl)
	}
}

// TestGenerateProjectObservation verifies that GenerateProjectObservation
// converts a modelv2.Project into a v1alpha1.ProjectObservation correctly,
// and returns a zero value when the input is nil.
func TestGenerateProjectObservation(t *testing.T) {
	t.Parallel()

	ownerName := "alice"

	var projectID int32 = 42

	var repoCount int64 = 7

	cases := []struct {
		name string
		proj *modelv2.Project
		want v1alpha1.ProjectObservation
	}{
		{
			name: "nil project returns zero observation",
			proj: nil,
			want: v1alpha1.ProjectObservation{},
		},
		{
			name: "populated project maps all fields",
			proj: &modelv2.Project{
				ProjectID: projectID,
				OwnerName: ownerName,
				RepoCount: repoCount,
			},
			want: v1alpha1.ProjectObservation{
				ProjectID: int32Ptr(projectID),
				OwnerName: &ownerName,
				RepoCount: int64Ptr(repoCount),
			},
		},
		{
			name: "zero-value project maps zero fields",
			proj: &modelv2.Project{},
			want: v1alpha1.ProjectObservation{
				ProjectID: int32Ptr(0),
				OwnerName: stringPtr(""),
				RepoCount: int64Ptr(0),
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
		proj   *modelv2.Project
		want   bool
	}{
		// StorageLimit branch — always returns false when set.
		{
			name: "StorageLimit non-nil returns false",
			params: v1alpha1.ProjectParameters{
				StorageLimit: int64Ptr(1024),
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{},
			},
			want: false,
		},
		// Nil metadata in params — nothing to check, always up to date.
		{
			name: "nil params.Metadata returns true",
			params: v1alpha1.ProjectParameters{
				Metadata: nil,
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{},
			},
			want: true,
		},
		// Nil metadata on the project — always out of date.
		{
			name: "nil proj.Metadata with desired meta returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{},
			},
			proj: &modelv2.Project{Metadata: nil},
			want: false,
		},
		// All metadata fields nil in desired → all helpers return true.
		{
			name: "all desired fields nil returns true",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					Public:                   boolFalseStr,
					AutoScan:                 stringPtr(boolFalseStr),
					EnableContentTrust:       stringPtr(boolFalseStr),
					EnableContentTrustCosign: stringPtr(boolFalseStr),
					PreventVul:               stringPtr(boolFalseStr),
					Severity:                 stringPtr("low"),
					ReuseSysCVEAllowlist:     stringPtr(boolFalseStr),
				},
			},
			want: true,
		},
		// All fields match exactly → true.
		{
			name: "all metadata fields match returns true",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					Public:                   boolPtr(true),
					AutoScan:                 boolPtr(false),
					EnableContentTrust:       boolPtr(false),
					EnableContentTrustCosign: boolPtr(false),
					PreventVulnerable:        boolPtr(true),
					Severity:                 stringPtr("high"),
					ReuseSysCVEAllowlist:     boolPtr(true),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					Public:                   boolTrueStr,
					AutoScan:                 stringPtr(boolFalseStr),
					EnableContentTrust:       stringPtr(boolFalseStr),
					EnableContentTrustCosign: stringPtr(boolFalseStr),
					PreventVul:               stringPtr(boolTrueStr),
					Severity:                 stringPtr("high"),
					ReuseSysCVEAllowlist:     stringPtr(boolTrueStr),
				},
			},
			want: true,
		},
		// Public mismatch: desired true, observed "false".
		{
			name: "Public mismatch returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					Public: boolPtr(true),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					Public: boolFalseStr,
				},
			},
			want: false,
		},
		// Public: desired false, observed nil string (empty) → mismatch.
		{
			name: "Public desired false vs observed empty string returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					Public: boolPtr(false),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					Public: "",
				},
			},
			want: false,
		},
		// Public: desired false, observed "false" → match.
		{
			name: "Public desired false vs observed false returns true",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					Public: boolPtr(false),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					Public: boolFalseStr,
				},
			},
			want: true,
		},
		// AutoScan mismatch: desired true, observed "false".
		{
			name: "AutoScan mismatch returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					AutoScan: boolPtr(true),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					AutoScan: stringPtr(boolFalseStr),
				},
			},
			want: false,
		},
		// AutoScan: desired true, observed nil → derefStr returns "" → mismatch.
		{
			name: "AutoScan desired true vs observed nil returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					AutoScan: boolPtr(true),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					AutoScan: nil,
				},
			},
			want: false,
		},
		// EnableContentTrust mismatch.
		{
			name: "EnableContentTrust mismatch returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					EnableContentTrust: boolPtr(true),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					EnableContentTrust: stringPtr(boolFalseStr),
				},
			},
			want: false,
		},
		// EnableContentTrustCosign mismatch.
		{
			name: "EnableContentTrustCosign mismatch returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					EnableContentTrustCosign: boolPtr(true),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					EnableContentTrustCosign: stringPtr(boolFalseStr),
				},
			},
			want: false,
		},
		// PreventVulnerable mismatch.
		{
			name: "PreventVulnerable mismatch returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					PreventVulnerable: boolPtr(true),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					PreventVul: stringPtr(boolFalseStr),
				},
			},
			want: false,
		},
		// Severity mismatch.
		{
			name: "Severity mismatch returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					Severity: stringPtr("high"),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					Severity: stringPtr("low"),
				},
			},
			want: false,
		},
		// Severity: desired non-nil, observed nil → mismatch.
		{
			name: "Severity desired set vs observed nil returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					Severity: stringPtr("critical"),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					Severity: nil,
				},
			},
			want: false,
		},
		// ReuseSysCVEAllowlist mismatch.
		{
			name: "ReuseSysCVEAllowlist mismatch returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					ReuseSysCVEAllowlist: boolPtr(true),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					ReuseSysCVEAllowlist: stringPtr(boolFalseStr),
				},
			},
			want: false,
		},
		// ReuseSysCVEAllowlist: desired set, observed nil → mismatch.
		{
			name: "ReuseSysCVEAllowlist desired true vs observed nil returns false",
			params: v1alpha1.ProjectParameters{
				Metadata: &v1alpha1.ProjectMetadataParameters{
					ReuseSysCVEAllowlist: boolPtr(true),
				},
			},
			proj: &modelv2.Project{
				Metadata: &modelv2.ProjectMetadata{
					ReuseSysCVEAllowlist: nil,
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

// TestToProjectReq verifies that ToProjectReq constructs a modelv2.ProjectReq
// with the correct project name, storage limit, and optional metadata.
func TestToProjectReq(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		reqName string
		params  v1alpha1.ProjectParameters
		want    *modelv2.ProjectReq
	}{
		{
			name:    "nil metadata produces req without metadata",
			reqName: projectNameMyProject,
			params: v1alpha1.ProjectParameters{
				StorageLimit: int64Ptr(2048),
				Metadata:     nil,
			},
			want: &modelv2.ProjectReq{
				ProjectName:  projectNameMyProject,
				StorageLimit: int64Ptr(2048),
				Metadata:     nil,
			},
		},
		{
			name:    "populated metadata is included in req",
			reqName: projectNameOther,
			params: v1alpha1.ProjectParameters{
				StorageLimit: nil,
				Metadata: &v1alpha1.ProjectMetadataParameters{
					Public:   boolPtr(true),
					AutoScan: boolPtr(false),
				},
			},
			want: &modelv2.ProjectReq{
				ProjectName:  projectNameOther,
				StorageLimit: nil,
				Metadata: &modelv2.ProjectMetadata{
					Public:   boolTrueStr,
					AutoScan: stringPtr(boolFalseStr),
				},
			},
		},
		{
			name:    "empty name with nil params produces minimal req",
			reqName: "",
			params:  v1alpha1.ProjectParameters{},
			want: &modelv2.ProjectReq{
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

// TestApplyProjectParameters verifies that ApplyProjectParameters correctly
// initializes a nil proj.Metadata, is a no-op when params.Metadata is nil,
// and applies all fields when params.Metadata is populated.
func TestApplyProjectParameters(t *testing.T) {
	t.Parallel()

	t.Run("nil proj.Metadata is initialized to empty struct", func(t *testing.T) {
		t.Parallel()

		proj := &modelv2.Project{Metadata: nil}
		params := v1alpha1.ProjectParameters{Metadata: nil}

		ApplyProjectParameters(params, proj)

		if proj.Metadata == nil {
			t.Error("ApplyProjectParameters() left proj.Metadata nil, want initialized")
		}
	})

	t.Run("nil params.Metadata leaves proj.Metadata unchanged", func(t *testing.T) {
		t.Parallel()

		existing := &modelv2.ProjectMetadata{
			Public:   boolTrueStr,
			AutoScan: stringPtr(boolTrueStr),
		}
		proj := &modelv2.Project{Metadata: existing}
		params := v1alpha1.ProjectParameters{Metadata: nil}

		ApplyProjectParameters(params, proj)

		if !reflect.DeepEqual(proj.Metadata, existing) {
			t.Errorf(
				"ApplyProjectParameters() changed metadata to %+v, want %+v",
				proj.Metadata, existing,
			)
		}
	})

	t.Run("populated params.Metadata applies all fields", func(t *testing.T) {
		t.Parallel()

		proj := &modelv2.Project{
			Metadata: &modelv2.ProjectMetadata{
				Public:   boolFalseStr,
				AutoScan: stringPtr(boolFalseStr),
			},
		}
		params := v1alpha1.ProjectParameters{
			Metadata: &v1alpha1.ProjectMetadataParameters{
				Public:                   boolPtr(true),
				AutoScan:                 boolPtr(true),
				EnableContentTrust:       boolPtr(true),
				EnableContentTrustCosign: boolPtr(false),
				PreventVulnerable:        boolPtr(true),
				Severity:                 stringPtr("critical"),
				ReuseSysCVEAllowlist:     boolPtr(false),
			},
		}

		ApplyProjectParameters(params, proj)

		want := &modelv2.ProjectMetadata{
			Public:                   boolTrueStr,
			AutoScan:                 stringPtr(boolTrueStr),
			EnableContentTrust:       stringPtr(boolTrueStr),
			EnableContentTrustCosign: stringPtr(boolFalseStr),
			PreventVul:               stringPtr(boolTrueStr),
			Severity:                 stringPtr("critical"),
			ReuseSysCVEAllowlist:     stringPtr(boolFalseStr),
		}

		if !reflect.DeepEqual(proj.Metadata, want) {
			t.Errorf(
				"ApplyProjectParameters() metadata = %+v, want %+v",
				proj.Metadata, want,
			)
		}
	})

	t.Run("nil proj.Metadata initialized then params applied", func(t *testing.T) {
		t.Parallel()

		proj := &modelv2.Project{Metadata: nil}
		params := v1alpha1.ProjectParameters{
			Metadata: &v1alpha1.ProjectMetadataParameters{
				Public:   boolPtr(false),
				Severity: stringPtr("low"),
			},
		}

		ApplyProjectParameters(params, proj)

		want := &modelv2.ProjectMetadata{
			Public:   boolFalseStr,
			Severity: stringPtr("low"),
		}

		if !reflect.DeepEqual(proj.Metadata, want) {
			t.Errorf(
				"ApplyProjectParameters() metadata = %+v, want %+v",
				proj.Metadata, want,
			)
		}
	})
}

// TestToHarborProjectMetadata verifies that ToHarborProjectMetadata converts
// each field of ProjectMetadataParameters into the string-based
// modelv2.ProjectMetadata Harbor expects, skipping nil fields.
func TestToHarborProjectMetadata(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		meta *v1alpha1.ProjectMetadataParameters
		want *modelv2.ProjectMetadata
	}{
		{
			name: "all fields nil returns empty metadata",
			meta: &v1alpha1.ProjectMetadataParameters{},
			want: &modelv2.ProjectMetadata{},
		},
		{
			name: "Public true is converted to string true",
			meta: &v1alpha1.ProjectMetadataParameters{
				Public: boolPtr(true),
			},
			want: &modelv2.ProjectMetadata{
				Public: boolTrueStr,
			},
		},
		{
			name: "Public false is converted to string false",
			meta: &v1alpha1.ProjectMetadataParameters{
				Public: boolPtr(false),
			},
			want: &modelv2.ProjectMetadata{
				Public: boolFalseStr,
			},
		},
		{
			name: "AutoScan true is converted to pointer to string true",
			meta: &v1alpha1.ProjectMetadataParameters{
				AutoScan: boolPtr(true),
			},
			want: &modelv2.ProjectMetadata{
				AutoScan: stringPtr(boolTrueStr),
			},
		},
		{
			name: "AutoScan false is converted to pointer to string false",
			meta: &v1alpha1.ProjectMetadataParameters{
				AutoScan: boolPtr(false),
			},
			want: &modelv2.ProjectMetadata{
				AutoScan: stringPtr(boolFalseStr),
			},
		},
		{
			name: "EnableContentTrust true is converted correctly",
			meta: &v1alpha1.ProjectMetadataParameters{
				EnableContentTrust: boolPtr(true),
			},
			want: &modelv2.ProjectMetadata{
				EnableContentTrust: stringPtr(boolTrueStr),
			},
		},
		{
			name: "EnableContentTrust false is converted correctly",
			meta: &v1alpha1.ProjectMetadataParameters{
				EnableContentTrust: boolPtr(false),
			},
			want: &modelv2.ProjectMetadata{
				EnableContentTrust: stringPtr(boolFalseStr),
			},
		},
		{
			name: "EnableContentTrustCosign true is converted correctly",
			meta: &v1alpha1.ProjectMetadataParameters{
				EnableContentTrustCosign: boolPtr(true),
			},
			want: &modelv2.ProjectMetadata{
				EnableContentTrustCosign: stringPtr(boolTrueStr),
			},
		},
		{
			name: "EnableContentTrustCosign false is converted correctly",
			meta: &v1alpha1.ProjectMetadataParameters{
				EnableContentTrustCosign: boolPtr(false),
			},
			want: &modelv2.ProjectMetadata{
				EnableContentTrustCosign: stringPtr(boolFalseStr),
			},
		},
		{
			name: "PreventVulnerable true maps to PreventVul string true",
			meta: &v1alpha1.ProjectMetadataParameters{
				PreventVulnerable: boolPtr(true),
			},
			want: &modelv2.ProjectMetadata{
				PreventVul: stringPtr(boolTrueStr),
			},
		},
		{
			name: "PreventVulnerable false maps to PreventVul string false",
			meta: &v1alpha1.ProjectMetadataParameters{
				PreventVulnerable: boolPtr(false),
			},
			want: &modelv2.ProjectMetadata{
				PreventVul: stringPtr(boolFalseStr),
			},
		},
		{
			name: "Severity non-nil is passed through unchanged",
			meta: &v1alpha1.ProjectMetadataParameters{
				Severity: stringPtr("medium"),
			},
			want: &modelv2.ProjectMetadata{
				Severity: stringPtr("medium"),
			},
		},
		{
			name: "ReuseSysCVEAllowlist true is converted correctly",
			meta: &v1alpha1.ProjectMetadataParameters{
				ReuseSysCVEAllowlist: boolPtr(true),
			},
			want: &modelv2.ProjectMetadata{
				ReuseSysCVEAllowlist: stringPtr(boolTrueStr),
			},
		},
		{
			name: "ReuseSysCVEAllowlist false is converted correctly",
			meta: &v1alpha1.ProjectMetadataParameters{
				ReuseSysCVEAllowlist: boolPtr(false),
			},
			want: &modelv2.ProjectMetadata{
				ReuseSysCVEAllowlist: stringPtr(boolFalseStr),
			},
		},
		{
			name: "all fields set produces fully populated metadata",
			meta: &v1alpha1.ProjectMetadataParameters{
				Public:                   boolPtr(true),
				AutoScan:                 boolPtr(true),
				EnableContentTrust:       boolPtr(false),
				EnableContentTrustCosign: boolPtr(false),
				PreventVulnerable:        boolPtr(true),
				Severity:                 stringPtr("critical"),
				ReuseSysCVEAllowlist:     boolPtr(false),
			},
			want: &modelv2.ProjectMetadata{
				Public:                   boolTrueStr,
				AutoScan:                 stringPtr(boolTrueStr),
				EnableContentTrust:       stringPtr(boolFalseStr),
				EnableContentTrustCosign: stringPtr(boolFalseStr),
				PreventVul:               stringPtr(boolTrueStr),
				Severity:                 stringPtr("critical"),
				ReuseSysCVEAllowlist:     stringPtr(boolFalseStr),
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
