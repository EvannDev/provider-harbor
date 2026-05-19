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

package fake

import (
	"context"

	"github.com/goharbor/go-client/pkg/sdk/v2.0/client/usergroup"

	iamclient "github.com/EvannDev/provider-harbor/internal/clients/iam"
)

// MockGroupsClient is a mock implementation of iam.GroupsClient.
type MockGroupsClient struct {
	// CreateUserGroupFn is called by CreateUserGroup when set.
	CreateUserGroupFn func(ctx context.Context, params *usergroup.CreateUserGroupParams) (*usergroup.CreateUserGroupCreated, error)
	// DeleteUserGroupFn is called by DeleteUserGroup when set.
	DeleteUserGroupFn func(ctx context.Context, params *usergroup.DeleteUserGroupParams) (*usergroup.DeleteUserGroupOK, error)
	// GetUserGroupFn is called by GetUserGroup when set.
	GetUserGroupFn func(ctx context.Context, params *usergroup.GetUserGroupParams) (*usergroup.GetUserGroupOK, error)
	// UpdateUserGroupFn is called by UpdateUserGroup when set.
	UpdateUserGroupFn func(ctx context.Context, params *usergroup.UpdateUserGroupParams) (*usergroup.UpdateUserGroupOK, error)
	// SearchUserGroupsFn is called by SearchUserGroups when set.
	SearchUserGroupsFn func(ctx context.Context, params *usergroup.SearchUserGroupsParams) (*usergroup.SearchUserGroupsOK, error)
}

// Ensure MockGroupsClient implements GroupsClient.
var _ iamclient.GroupsClient = &MockGroupsClient{}

// CreateUserGroup implements GroupsClient.CreateUserGroup.
func (m *MockGroupsClient) CreateUserGroup(ctx context.Context, params *usergroup.CreateUserGroupParams) (*usergroup.CreateUserGroupCreated, error) {
	if m.CreateUserGroupFn != nil {
		return m.CreateUserGroupFn(ctx, params)
	}

	return nil, errNotImplemented
}

// DeleteUserGroup implements GroupsClient.DeleteUserGroup.
func (m *MockGroupsClient) DeleteUserGroup(ctx context.Context, params *usergroup.DeleteUserGroupParams) (*usergroup.DeleteUserGroupOK, error) {
	if m.DeleteUserGroupFn != nil {
		return m.DeleteUserGroupFn(ctx, params)
	}

	return nil, errNotImplemented
}

// GetUserGroup implements GroupsClient.GetUserGroup.
func (m *MockGroupsClient) GetUserGroup(ctx context.Context, params *usergroup.GetUserGroupParams) (*usergroup.GetUserGroupOK, error) {
	if m.GetUserGroupFn != nil {
		return m.GetUserGroupFn(ctx, params)
	}

	return nil, errNotImplemented
}

// UpdateUserGroup implements GroupsClient.UpdateUserGroup.
func (m *MockGroupsClient) UpdateUserGroup(ctx context.Context, params *usergroup.UpdateUserGroupParams) (*usergroup.UpdateUserGroupOK, error) {
	if m.UpdateUserGroupFn != nil {
		return m.UpdateUserGroupFn(ctx, params)
	}

	return nil, errNotImplemented
}

// SearchUserGroups implements GroupsClient.SearchUserGroups.
func (m *MockGroupsClient) SearchUserGroups(ctx context.Context, params *usergroup.SearchUserGroupsParams) (*usergroup.SearchUserGroupsOK, error) {
	if m.SearchUserGroupsFn != nil {
		return m.SearchUserGroupsFn(ctx, params)
	}

	return nil, errNotImplemented
}
