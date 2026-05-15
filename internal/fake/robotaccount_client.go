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

package fake

import (
	"context"

	modelv2 "github.com/mittwald/goharbor-client/v5/apiv2/model"

	iamclient "github.com/EvannDev/provider-harbor/internal/clients/iam"
)

// MockRobotAccountsClient is a mock implementation of iam.RobotAccountsClient.
type MockRobotAccountsClient struct {
	// GetRobotAccountByIDFn is called by GetRobotAccountByID when set.
	GetRobotAccountByIDFn func(ctx context.Context, id int64) (*modelv2.Robot, error)
	// GetRobotAccountByNameFn is called by GetRobotAccountByName when set.
	GetRobotAccountByNameFn func(ctx context.Context, name string) (*modelv2.Robot, error)
	// ListProjectRobotsV1Fn is called by ListProjectRobotsV1 when set.
	ListProjectRobotsV1Fn func(ctx context.Context, project string) ([]*modelv2.Robot, error)
	// NewRobotAccountFn is called by NewRobotAccount when set.
	NewRobotAccountFn func(ctx context.Context, robot *modelv2.RobotCreate) (*modelv2.RobotCreated, error)
	// UpdateRobotAccountFn is called by UpdateRobotAccount when set.
	UpdateRobotAccountFn func(ctx context.Context, robot *modelv2.Robot) error
	// DeleteRobotAccountByIDFn is called by DeleteRobotAccountByID when set.
	DeleteRobotAccountByIDFn func(ctx context.Context, id int64) error
}

// Ensure MockRobotAccountsClient implements RobotAccountsClient.
var _ iamclient.RobotAccountsClient = &MockRobotAccountsClient{}

// GetRobotAccountByID implements RobotAccountsClient.GetRobotAccountByID.
func (m *MockRobotAccountsClient) GetRobotAccountByID(ctx context.Context, id int64) (*modelv2.Robot, error) {
	if m.GetRobotAccountByIDFn != nil {
		return m.GetRobotAccountByIDFn(ctx, id)
	}

	return nil, errNotImplemented
}

// GetRobotAccountByName implements RobotAccountsClient.GetRobotAccountByName.
func (m *MockRobotAccountsClient) GetRobotAccountByName(ctx context.Context, name string) (*modelv2.Robot, error) {
	if m.GetRobotAccountByNameFn != nil {
		return m.GetRobotAccountByNameFn(ctx, name)
	}

	return nil, errNotImplemented
}

// ListProjectRobotsV1 implements RobotAccountsClient.ListProjectRobotsV1.
func (m *MockRobotAccountsClient) ListProjectRobotsV1(ctx context.Context, project string) ([]*modelv2.Robot, error) {
	if m.ListProjectRobotsV1Fn != nil {
		return m.ListProjectRobotsV1Fn(ctx, project)
	}

	return nil, errNotImplemented
}

// NewRobotAccount implements RobotAccountsClient.NewRobotAccount.
func (m *MockRobotAccountsClient) NewRobotAccount(ctx context.Context, robot *modelv2.RobotCreate) (*modelv2.RobotCreated, error) {
	if m.NewRobotAccountFn != nil {
		return m.NewRobotAccountFn(ctx, robot)
	}

	return nil, errNotImplemented
}

// UpdateRobotAccount implements RobotAccountsClient.UpdateRobotAccount.
func (m *MockRobotAccountsClient) UpdateRobotAccount(ctx context.Context, robot *modelv2.Robot) error {
	if m.UpdateRobotAccountFn != nil {
		return m.UpdateRobotAccountFn(ctx, robot)
	}

	return errNotImplemented
}

// DeleteRobotAccountByID implements RobotAccountsClient.DeleteRobotAccountByID.
func (m *MockRobotAccountsClient) DeleteRobotAccountByID(ctx context.Context, id int64) error {
	if m.DeleteRobotAccountByIDFn != nil {
		return m.DeleteRobotAccountByIDFn(ctx, id)
	}

	return errNotImplemented
}
