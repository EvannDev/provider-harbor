/*
Copyright 2026 The Crossplane Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package iam

import (
	"testing"

	"github.com/goharbor/go-client/pkg/sdk/v2.0/models"

	v1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/clients/common"
)

// TestIsGroupUpToDate verifies spec-vs-observation comparisons.
func TestIsGroupUpToDate(t *testing.T) {
	t.Parallel()

	ldapDN := "cn=group,dc=example,dc=com"
	otherDN := "cn=other,dc=example,dc=com"

	tests := []struct {
		name        string
		spec        *v1alpha1.GroupParameters
		observation *v1alpha1.GroupObservation
		want        bool
	}{
		{
			name: "nil spec returns true",
			spec: nil,
			observation: &v1alpha1.GroupObservation{
				ID:   1,
				Name: "mygroup",
				Type: GroupTypeInternal,
			},
			want: true,
		},
		{
			name:        "nil observation returns false",
			spec:        &v1alpha1.GroupParameters{Name: "mygroup", Type: GroupTypeInternal},
			observation: nil,
			want:        false,
		},
		{
			name: "name mismatch returns false",
			spec: &v1alpha1.GroupParameters{Name: "newname", Type: GroupTypeInternal},
			observation: &v1alpha1.GroupObservation{
				ID:   1,
				Name: "oldname",
				Type: GroupTypeInternal,
			},
			want: false,
		},
		{
			name: "type mismatch returns false",
			spec: &v1alpha1.GroupParameters{Name: "mygroup", Type: GroupTypeOIDC},
			observation: &v1alpha1.GroupObservation{
				ID:   1,
				Name: "mygroup",
				Type: GroupTypeInternal,
			},
			want: false,
		},
		{
			name: "ldap dn mismatch returns false",
			spec: &v1alpha1.GroupParameters{
				Name:        "mygroup",
				Type:        GroupTypeLDAP,
				LDAPGroupDN: &ldapDN,
			},
			observation: &v1alpha1.GroupObservation{
				ID:          1,
				Name:        "mygroup",
				Type:        GroupTypeLDAP,
				LDAPGroupDN: &otherDN,
			},
			want: false,
		},
		{
			name: "spec has dn obs does not",
			spec: &v1alpha1.GroupParameters{
				Name:        "mygroup",
				Type:        GroupTypeLDAP,
				LDAPGroupDN: &ldapDN,
			},
			observation: &v1alpha1.GroupObservation{
				ID:   1,
				Name: "mygroup",
				Type: GroupTypeLDAP,
			},
			want: false,
		},
		{
			name: "fully matching ldap group returns true",
			spec: &v1alpha1.GroupParameters{
				Name:        "mygroup",
				Type:        GroupTypeLDAP,
				LDAPGroupDN: &ldapDN,
			},
			observation: &v1alpha1.GroupObservation{
				ID:          1,
				Name:        "mygroup",
				Type:        GroupTypeLDAP,
				LDAPGroupDN: &ldapDN,
			},
			want: true,
		},
		{
			name:        "internal group no dn returns true",
			spec:        &v1alpha1.GroupParameters{Name: "mygroup", Type: GroupTypeInternal},
			observation: &v1alpha1.GroupObservation{ID: 1, Name: "mygroup", Type: GroupTypeInternal},
			want:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := IsGroupUpToDate(tc.spec, tc.observation)

			if got != tc.want {
				t.Errorf("IsGroupUpToDate() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestLateInitializeGroup verifies that no spec fields are modified.
func TestLateInitializeGroup(t *testing.T) {
	t.Parallel()

	spec := &v1alpha1.GroupParameters{
		Name: "mygroup",
		Type: GroupTypeInternal,
	}
	obs := &v1alpha1.GroupObservation{
		ID:   1,
		Name: "mygroup",
		Type: GroupTypeInternal,
	}

	before := *spec

	LateInitializeGroup(spec, obs)

	if spec.Name != before.Name || spec.Type != before.Type {
		t.Errorf(
			"LateInitializeGroup: spec changed from %+v to %+v",
			before, *spec,
		)
	}

	// Cover the nil guard branches.
	LateInitializeGroup(nil, nil)
	LateInitializeGroup(nil, &v1alpha1.GroupObservation{})
}

// TestNewGroupsClient verifies that NewGroupsClient returns a non-nil client.
func TestNewGroupsClient(t *testing.T) {
	t.Parallel()

	cfg := common.Config{
		URL:      "http://localhost:9000",
		Username: "admin",
		Password: "pass",
	}

	got := NewGroupsClient(cfg)
	if got == nil {
		t.Error("NewGroupsClient() returned nil, want non-nil")
	}
}

// TestIsGroupLateInitialized verifies it always returns false.
func TestIsGroupLateInitialized(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		former  *v1alpha1.GroupParameters
		current *v1alpha1.GroupParameters
	}{
		{
			name:    "nil params",
			former:  nil,
			current: nil,
		},
		{
			name:    "identical params",
			former:  &v1alpha1.GroupParameters{Name: "g", Type: GroupTypeInternal},
			current: &v1alpha1.GroupParameters{Name: "g", Type: GroupTypeInternal},
		},
		{
			name:    "different params",
			former:  &v1alpha1.GroupParameters{Name: "old", Type: GroupTypeInternal},
			current: &v1alpha1.GroupParameters{Name: "new", Type: GroupTypeOIDC},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := IsGroupLateInitialized(tc.former, tc.current)

			if got {
				t.Error("IsGroupLateInitialized() = true, want false")
			}
		})
	}
}

// TestGenerateGroupObservation verifies Harbor model to observation conversion.
func TestGenerateGroupObservation(t *testing.T) {
	t.Parallel()

	ldapDN := "cn=group,dc=example,dc=com"

	tests := []struct {
		name  string
		input *models.UserGroup
		want  v1alpha1.GroupObservation
	}{
		{
			name:  "nil input returns empty observation",
			input: nil,
			want:  v1alpha1.GroupObservation{},
		},
		{
			name: "internal group no dn",
			input: &models.UserGroup{
				ID:        42,
				GroupName: "mygroup",
				GroupType: groupTypeInternalInt,
			},
			want: v1alpha1.GroupObservation{
				ID:   42,
				Name: "mygroup",
				Type: GroupTypeInternal,
			},
		},
		{
			name: "ldap group with dn",
			input: &models.UserGroup{
				ID:          7,
				GroupName:   "ldapgroup",
				GroupType:   groupTypeLDAPInt,
				LdapGroupDn: ldapDN,
			},
			want: v1alpha1.GroupObservation{
				ID:          7,
				Name:        "ldapgroup",
				Type:        GroupTypeLDAP,
				LDAPGroupDN: &ldapDN,
			},
		},
		{
			name: "oidc group",
			input: &models.UserGroup{
				ID:        3,
				GroupName: "oidcgroup",
				GroupType: groupTypeOIDCInt,
			},
			want: v1alpha1.GroupObservation{
				ID:   3,
				Name: "oidcgroup",
				Type: GroupTypeOIDC,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := GenerateGroupObservation(tc.input)

			if got.ID != tc.want.ID || got.Name != tc.want.Name || got.Type != tc.want.Type {
				t.Errorf(
					"GenerateGroupObservation(): got {ID:%d Name:%s Type:%s}, "+
						"want {ID:%d Name:%s Type:%s}",
					got.ID, got.Name, got.Type,
					tc.want.ID, tc.want.Name, tc.want.Type,
				)
			}

			checkDNPointer(t, got.LDAPGroupDN, tc.want.LDAPGroupDN)
		})
	}
}

// checkDNPointer compares two LDAP DN pointer values.
func checkDNPointer(t *testing.T, got, want *string) {
	t.Helper()

	if got == nil && want == nil {
		return
	}

	if got == nil || want == nil {
		t.Errorf("LDAPGroupDN: got %v, want %v", got, want)

		return
	}

	if *got != *want {
		t.Errorf("LDAPGroupDN: got %q, want %q", *got, *want)
	}
}

// TestGroupTypeConversions verifies round-trip type conversions.
func TestGroupTypeConversions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		str     string
		integer int64
	}{
		{name: GroupTypeLDAP, str: GroupTypeLDAP, integer: groupTypeLDAPInt},
		{name: GroupTypeInternal, str: GroupTypeInternal, integer: groupTypeInternalInt},
		{name: GroupTypeOIDC, str: GroupTypeOIDC, integer: groupTypeOIDCInt},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotInt := groupTypeToInt(tc.str)
			if gotInt != tc.integer {
				t.Errorf("groupTypeToInt(%q) = %d, want %d", tc.str, gotInt, tc.integer)
			}

			gotStr := groupTypeToString(tc.integer)
			if gotStr != tc.str {
				t.Errorf("groupTypeToString(%d) = %q, want %q", tc.integer, gotStr, tc.str)
			}
		})
	}
}

// TestIsGroupNotFound verifies the not-found sentinel error detection.
func TestIsGroupNotFound(t *testing.T) {
	t.Parallel()

	if !IsGroupNotFound(ErrGroupNotFound) {
		t.Error("IsGroupNotFound(ErrGroupNotFound) = false, want true")
	}

	if IsGroupNotFound(nil) {
		t.Error("IsGroupNotFound(nil) = true, want false")
	}
}

// TestGenerateUserGroupSearchOpts verifies search parameter generation.
func TestGenerateUserGroupSearchOpts(t *testing.T) {
	t.Parallel()

	page := int64(2)
	pageSize := int64(50)
	spec := v1alpha1.GroupParameters{Name: "testgroup", Type: GroupTypeInternal}

	tests := []struct {
		name           string
		paginationArgs *common.PaginationArgs
		wantPage       *int64
		wantPageSize   *int64
	}{
		{
			name:           "nil paginationArgs returns params without page",
			paginationArgs: nil,
			wantPage:       nil,
			wantPageSize:   nil,
		},
		{
			name:           "with pagination sets page and page size",
			paginationArgs: &common.PaginationArgs{Page: &page, PageSize: &pageSize},
			wantPage:       &page,
			wantPageSize:   &pageSize,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := GenerateUserGroupSearchOpts(spec, tc.paginationArgs)

			if got.Groupname != spec.Name {
				t.Errorf("Groupname = %q, want %q", got.Groupname, spec.Name)
			}

			if tc.wantPage == nil && got.Page != nil {
				t.Errorf("Page = %v, want nil", got.Page)
			}

			if tc.wantPage != nil && (got.Page == nil || *got.Page != *tc.wantPage) {
				t.Errorf("Page = %v, want %d", got.Page, *tc.wantPage)
			}

			if tc.wantPageSize == nil && got.PageSize != nil {
				t.Errorf("PageSize = %v, want nil", got.PageSize)
			}

			if tc.wantPageSize != nil && (got.PageSize == nil || *got.PageSize != *tc.wantPageSize) {
				t.Errorf("PageSize = %v, want %d", got.PageSize, *tc.wantPageSize)
			}
		})
	}
}

// TestGenerateUserGroupGetOpts verifies get parameter generation.
func TestGenerateUserGroupGetOpts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		groupID int64
	}{
		{name: "zero id", groupID: 0},
		{name: "positive id", groupID: 42},
		{name: "large id", groupID: 999999},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := GenerateUserGroupGetOpts(tc.groupID)

			if got == nil {
				t.Fatal("GenerateUserGroupGetOpts() returned nil")
			}

			if got.GroupID != tc.groupID {
				t.Errorf("GroupID = %d, want %d", got.GroupID, tc.groupID)
			}
		})
	}
}

// TestGenerateUserGroupDeleteOpts verifies delete parameter generation.
func TestGenerateUserGroupDeleteOpts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		groupID int64
	}{
		{name: "zero id", groupID: 0},
		{name: "positive id", groupID: 7},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := GenerateUserGroupDeleteOpts(tc.groupID)

			if got == nil {
				t.Fatal("GenerateUserGroupDeleteOpts() returned nil")
			}

			if got.GroupID != tc.groupID {
				t.Errorf("GroupID = %d, want %d", got.GroupID, tc.groupID)
			}
		})
	}
}

// TestGenerateUserGroupCreateOpts verifies create parameter generation.
func TestGenerateUserGroupCreateOpts(t *testing.T) {
	t.Parallel()

	ldapDN := "cn=group,dc=example,dc=com"

	tests := []struct {
		name          string
		params        *v1alpha1.GroupParameters
		wantName      string
		wantGroupType int64
		wantLDAPDN    string
	}{
		{
			name:          "nil params returns empty create params",
			params:        nil,
			wantName:      "",
			wantGroupType: 0,
			wantLDAPDN:    "",
		},
		{
			name:          "internal group without LDAP DN",
			params:        &v1alpha1.GroupParameters{Name: "mygroup", Type: GroupTypeInternal},
			wantName:      "mygroup",
			wantGroupType: groupTypeInternalInt,
			wantLDAPDN:    "",
		},
		{
			name:          "ldap group with DN",
			params:        &v1alpha1.GroupParameters{Name: "ldapgroup", Type: GroupTypeLDAP, LDAPGroupDN: &ldapDN},
			wantName:      "ldapgroup",
			wantGroupType: groupTypeLDAPInt,
			wantLDAPDN:    ldapDN,
		},
		{
			name:          "oidc group",
			params:        &v1alpha1.GroupParameters{Name: "oidcgroup", Type: GroupTypeOIDC},
			wantName:      "oidcgroup",
			wantGroupType: groupTypeOIDCInt,
			wantLDAPDN:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := GenerateUserGroupCreateOpts(tc.params)

			if got == nil {
				t.Fatal("GenerateUserGroupCreateOpts() returned nil")
			}

			if tc.params == nil {
				if got.Usergroup != nil {
					t.Errorf("Usergroup = %v, want nil for nil params", got.Usergroup)
				}

				return
			}

			if got.Usergroup == nil {
				t.Fatal("Usergroup is nil, want non-nil")
			}

			if got.Usergroup.GroupName != tc.wantName {
				t.Errorf("GroupName = %q, want %q", got.Usergroup.GroupName, tc.wantName)
			}

			if got.Usergroup.GroupType != tc.wantGroupType {
				t.Errorf("GroupType = %d, want %d", got.Usergroup.GroupType, tc.wantGroupType)
			}

			if got.Usergroup.LdapGroupDn != tc.wantLDAPDN {
				t.Errorf("LdapGroupDn = %q, want %q", got.Usergroup.LdapGroupDn, tc.wantLDAPDN)
			}
		})
	}
}

// TestGenerateUserGroupUpdateOpts verifies update parameter generation.
func TestGenerateUserGroupUpdateOpts(t *testing.T) {
	t.Parallel()

	ldapDN := "cn=group,dc=example,dc=com"

	tests := []struct {
		name          string
		groupID       int64
		params        *v1alpha1.GroupParameters
		wantGroupID   int64
		wantName      string
		wantGroupType int64
		wantLDAPDN    string
	}{
		{
			name:        "nil params returns empty update params",
			groupID:     5,
			params:      nil,
			wantGroupID: 0,
		},
		{
			name:          "internal group",
			groupID:       10,
			params:        &v1alpha1.GroupParameters{Name: "mygroup", Type: GroupTypeInternal},
			wantGroupID:   10,
			wantName:      "mygroup",
			wantGroupType: groupTypeInternalInt,
			wantLDAPDN:    "",
		},
		{
			name:          "ldap group with DN",
			groupID:       20,
			params:        &v1alpha1.GroupParameters{Name: "ldapgroup", Type: GroupTypeLDAP, LDAPGroupDN: &ldapDN},
			wantGroupID:   20,
			wantName:      "ldapgroup",
			wantGroupType: groupTypeLDAPInt,
			wantLDAPDN:    ldapDN,
		},
		{
			name:          "oidc group",
			groupID:       30,
			params:        &v1alpha1.GroupParameters{Name: "oidcgroup", Type: GroupTypeOIDC},
			wantGroupID:   30,
			wantName:      "oidcgroup",
			wantGroupType: groupTypeOIDCInt,
			wantLDAPDN:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := GenerateUserGroupUpdateOpts(tc.groupID, tc.params)

			if got == nil {
				t.Fatal("GenerateUserGroupUpdateOpts() returned nil")
			}

			if tc.params == nil {
				if got.Usergroup != nil {
					t.Errorf("Usergroup = %v, want nil for nil params", got.Usergroup)
				}

				return
			}

			if got.GroupID != tc.wantGroupID {
				t.Errorf("GroupID = %d, want %d", got.GroupID, tc.wantGroupID)
			}

			if got.Usergroup == nil {
				t.Fatal("Usergroup is nil, want non-nil")
			}

			if got.Usergroup.GroupName != tc.wantName {
				t.Errorf("GroupName = %q, want %q", got.Usergroup.GroupName, tc.wantName)
			}

			if got.Usergroup.GroupType != tc.wantGroupType {
				t.Errorf("GroupType = %d, want %d", got.Usergroup.GroupType, tc.wantGroupType)
			}

			if got.Usergroup.LdapGroupDn != tc.wantLDAPDN {
				t.Errorf("LdapGroupDn = %q, want %q", got.Usergroup.LdapGroupDn, tc.wantLDAPDN)
			}
		})
	}
}

// TestIsGroupInSearchResults verifies search result matching.
func TestIsGroupInSearchResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		groupName string
		results   []*models.UserGroupSearchItem
		wantFound bool
		wantID    int64
	}{
		{
			name:      "empty results returns not found",
			groupName: "mygroup",
			results:   []*models.UserGroupSearchItem{},
			wantFound: false,
			wantID:    -1,
		},
		{
			name:      "nil item in slice is skipped",
			groupName: "mygroup",
			results:   []*models.UserGroupSearchItem{nil},
			wantFound: false,
			wantID:    -1,
		},
		{
			name:      "group not in list returns not found",
			groupName: "mygroup",
			results: []*models.UserGroupSearchItem{
				{ID: 1, GroupName: "othergroup"},
				{ID: 2, GroupName: "anothergroup"},
			},
			wantFound: false,
			wantID:    -1,
		},
		{
			name:      "group found in list returns id",
			groupName: "mygroup",
			results: []*models.UserGroupSearchItem{
				{ID: 1, GroupName: "othergroup"},
				{ID: 42, GroupName: "mygroup"},
			},
			wantFound: true,
			wantID:    42,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotFound, gotID := IsGroupInSearchResults(tc.groupName, tc.results)

			if gotFound != tc.wantFound {
				t.Errorf("found = %v, want %v", gotFound, tc.wantFound)
			}

			if gotID != tc.wantID {
				t.Errorf("id = %d, want %d", gotID, tc.wantID)
			}
		})
	}
}
