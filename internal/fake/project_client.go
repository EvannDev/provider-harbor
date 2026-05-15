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

	projectclient "github.com/EvannDev/provider-harbor/internal/clients/project"
)

// MockProjectsClient is a mock implementation of project.ProjectsClient.
type MockProjectsClient struct {
	// GetProjectFn is called by GetProject when set.
	GetProjectFn func(ctx context.Context, name string) (*modelv2.Project, error)
	// NewProjectFn is called by NewProject when set.
	NewProjectFn func(ctx context.Context, req *modelv2.ProjectReq) error
	// UpdateProjectFn is called by UpdateProject when set.
	UpdateProjectFn func(ctx context.Context, proj *modelv2.Project, limit *int64) error
	// DeleteProjectFn is called by DeleteProject when set.
	DeleteProjectFn func(ctx context.Context, name string) error
}

// Ensure MockProjectsClient implements ProjectsClient.
var _ projectclient.ProjectsClient = &MockProjectsClient{}

// GetProject implements ProjectsClient.GetProject.
func (m *MockProjectsClient) GetProject(ctx context.Context, name string) (*modelv2.Project, error) {
	if m.GetProjectFn != nil {
		return m.GetProjectFn(ctx, name)
	}

	return nil, errNotImplemented
}

// NewProject implements ProjectsClient.NewProject.
func (m *MockProjectsClient) NewProject(ctx context.Context, req *modelv2.ProjectReq) error {
	if m.NewProjectFn != nil {
		return m.NewProjectFn(ctx, req)
	}

	return errNotImplemented
}

// UpdateProject implements ProjectsClient.UpdateProject.
func (m *MockProjectsClient) UpdateProject(ctx context.Context, proj *modelv2.Project, limit *int64) error {
	if m.UpdateProjectFn != nil {
		return m.UpdateProjectFn(ctx, proj, limit)
	}

	return errNotImplemented
}

// DeleteProject implements ProjectsClient.DeleteProject.
func (m *MockProjectsClient) DeleteProject(ctx context.Context, name string) error {
	if m.DeleteProjectFn != nil {
		return m.DeleteProjectFn(ctx, name)
	}

	return errNotImplemented
}
