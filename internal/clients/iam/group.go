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
	"context"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/client/usergroup"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/models"

	v1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/clients/common"
	"github.com/EvannDev/provider-harbor/internal/helpers"
)

const (
	// GroupTypeLDAP is the string value for LDAP group type in Harbor.
	GroupTypeLDAP = "ldap"
	// GroupTypeInternal is the string value for internal group type in Harbor.
	GroupTypeInternal = "internal"
	// GroupTypeOIDC is the string value for OIDC group type in Harbor.
	GroupTypeOIDC = "oidc"

	// groupTypeLDAPInt is the Harbor integer code for LDAP groups.
	groupTypeLDAPInt int64 = 1
	// groupTypeInternalInt is the Harbor integer code for internal groups.
	groupTypeInternalInt int64 = 2
	// groupTypeOIDCInt is the Harbor integer code for OIDC groups.
	groupTypeOIDCInt int64 = 3
)

// ErrGroupNotFound is returned when the group does not exist in Harbor.
var ErrGroupNotFound = errors.New("group not found")

// GroupsClient defines operations for Harbor user group management.
type GroupsClient interface {
	CreateUserGroup(ctx context.Context, params *usergroup.CreateUserGroupParams) (*usergroup.CreateUserGroupCreated, error)
	DeleteUserGroup(ctx context.Context, params *usergroup.DeleteUserGroupParams) (*usergroup.DeleteUserGroupOK, error)
	GetUserGroup(ctx context.Context, params *usergroup.GetUserGroupParams) (*usergroup.GetUserGroupOK, error)
	UpdateUserGroup(ctx context.Context, params *usergroup.UpdateUserGroupParams) (*usergroup.UpdateUserGroupOK, error)
	SearchUserGroups(ctx context.Context, params *usergroup.SearchUserGroupsParams) (*usergroup.SearchUserGroupsOK, error)
}

// NewGroupsClient creates a GroupsClient from the provided Harbor config.
func NewGroupsClient(cfg common.Config) GroupsClient {
	return common.NewHarborAPI(cfg).Usergroup
}

// IsGroupNotFound reports whether err indicates the group does not exist.
func IsGroupNotFound(err error) bool {
	return errors.Is(err, ErrGroupNotFound)
}

// groupTypeToInt converts a string group type to its Harbor integer code.
func groupTypeToInt(groupType string) int64 {
	switch groupType {
	case GroupTypeLDAP:
		return groupTypeLDAPInt
	case GroupTypeOIDC:
		return groupTypeOIDCInt
	default:
		return groupTypeInternalInt
	}
}

// groupTypeToString converts a Harbor integer group type to its string form.
func groupTypeToString(groupType int64) string {
	switch groupType {
	case groupTypeLDAPInt:
		return GroupTypeLDAP
	case groupTypeOIDCInt:
		return GroupTypeOIDC
	default:
		return GroupTypeInternal
	}
}

// GenerateGroupObservation converts a Harbor UserGroup into a GroupObservation.
func GenerateGroupObservation(userGroup *models.UserGroup) v1alpha1.GroupObservation {
	if userGroup == nil {
		return v1alpha1.GroupObservation{}
	}

	obs := v1alpha1.GroupObservation{
		ID:   userGroup.ID,
		Name: userGroup.GroupName,
		Type: groupTypeToString(userGroup.GroupType),
	}

	if userGroup.LdapGroupDn != "" {
		obs.LDAPGroupDN = &userGroup.LdapGroupDn
	}

	return obs
}

// GenerateUserGroupGetOpts creates GetUserGroupParams for the given group ID.
func GenerateUserGroupGetOpts(groupID int64) *usergroup.GetUserGroupParams {
	return &usergroup.GetUserGroupParams{GroupID: groupID}
}

// GenerateUserGroupSearchOpts creates SearchUserGroupsParams
// for the given group name.
func GenerateUserGroupSearchOpts(spec v1alpha1.GroupParameters, paginationArgs *common.PaginationArgs) *usergroup.SearchUserGroupsParams {
	if paginationArgs == nil {
		return &usergroup.SearchUserGroupsParams{
			Groupname: spec.Name,
		}
	}

	return &usergroup.SearchUserGroupsParams{
		Groupname: spec.Name,
		Page:      paginationArgs.Page,
		PageSize:  paginationArgs.PageSize,
	}
}

// GenerateUserGroupDeleteOpts creates DeleteUserGroupParams for the given
// group ID.
func GenerateUserGroupDeleteOpts(groupID int64) *usergroup.DeleteUserGroupParams {
	return &usergroup.DeleteUserGroupParams{GroupID: groupID}
}

// GenerateUserGroupCreateOpts converts GroupParameters to a Harbor
// UserGroup model for creation.
func GenerateUserGroupCreateOpts(params *v1alpha1.GroupParameters) *usergroup.CreateUserGroupParams {
	if params == nil {
		return &usergroup.CreateUserGroupParams{}
	}

	userGroup := &models.UserGroup{
		GroupName: params.Name,
		GroupType: groupTypeToInt(params.Type),
	}

	if params.LDAPGroupDN != nil {
		userGroup.LdapGroupDn = *params.LDAPGroupDN
	}

	return &usergroup.CreateUserGroupParams{Usergroup: userGroup}
}

// GenerateUserGroupUpdateOpts converts GroupParameters to a Harbor
// UserGroup model for updating.
func GenerateUserGroupUpdateOpts(groupID int64, params *v1alpha1.GroupParameters) *usergroup.UpdateUserGroupParams {
	if params == nil {
		return &usergroup.UpdateUserGroupParams{}
	}

	userGroup := &models.UserGroup{
		GroupName: params.Name,
		GroupType: groupTypeToInt(params.Type),
	}

	if params.LDAPGroupDN != nil {
		userGroup.LdapGroupDn = *params.LDAPGroupDN
	}

	return &usergroup.UpdateUserGroupParams{
		GroupID:   groupID,
		Usergroup: userGroup}
}

// LateInitializeGroup fills empty Group spec fields from the API observation.
// Groups have no server-assigned spec fields; this is a no-op.
func LateInitializeGroup(spec *v1alpha1.GroupParameters, observation *v1alpha1.GroupObservation) {
	if spec == nil || observation == nil {
		return
	}
}

// IsGroupLateInitialized reports whether the spec changed during late init.
// Groups do not late-initialize any fields, so this always returns false.
func IsGroupLateInitialized(_, _ *v1alpha1.GroupParameters) bool {
	return false
}

// IsGroupUpToDate reports whether the spec matches the observed Harbor state.
func IsGroupUpToDate(spec *v1alpha1.GroupParameters, observation *v1alpha1.GroupObservation) bool {
	if spec == nil {
		return true
	}

	if observation == nil {
		return false
	}

	return spec.Name == observation.Name &&
		spec.Type == observation.Type &&
		helpers.IsComparablePtrEqualComparablePtr(spec.LDAPGroupDN, observation.LDAPGroupDN)
}

// IsGroupInSearchResults checks if a group with the given name
// exists in the search results.
// It returns the group id if found, or -1 if not found.
func IsGroupInSearchResults(groupName string, searchResponse []*models.UserGroupSearchItem) (found bool, groupID int64) {
	for _, group := range searchResponse {
		if group != nil && group.GroupName == groupName {
			return true, group.ID
		}
	}

	return false, -1
}
