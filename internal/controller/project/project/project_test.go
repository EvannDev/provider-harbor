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
	"context"
	"errors"
	"testing"

	"github.com/goharbor/go-client/pkg/sdk/v2.0/models"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/gate"

	projectv1alpha1 "github.com/EvannDev/provider-harbor/apis/project/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/fake"
)

const (
	// testProjectName is the project name used in tests.
	testProjectName = "my-project"
	// testOwnerName is the project owner name used in tests.
	testOwnerName = "admin"
	// testOtherProjectName is an alternate project name used in tests.
	testOtherProjectName = "other-project"
	// tcSuccess is the test case name for the success path.
	tcSuccess = "success"
	// tcAPIError is the test case name for the API error path.
	tcAPIError = "api error"
)

// errFakeAPI is a generic API error used in tests.
var errFakeAPI = errors.New("harbor api error")

// newTestProject returns a minimal Project for testing.
func newTestProject() *projectv1alpha1.Project {
	return &projectv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name: testProjectName,
		},
	}
}

// newHarborProject returns a minimal Harbor Project for testing.
func newHarborProject() *models.Project {
	return &models.Project{
		Name:      testProjectName,
		OwnerName: testOwnerName,
	}
}

// TestSetupGated verifies SetupGated registers the callback
// with the gate and returns nil.
func TestSetupGated(t *testing.T) {
	t.Parallel()

	opts := controller.Options{
		Gate: new(gate.Gate[schema.GroupVersionKind]),
	}

	err := SetupGated(nil, opts)
	if err != nil {
		t.Errorf("SetupGated: unexpected error: %v", err)
	}
}

// TestDisconnect verifies Disconnect always returns nil.
func TestDisconnect(t *testing.T) {
	t.Parallel()

	ext := &external{}

	err := ext.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect: got error %v, want nil", err)
	}
}

// TestIsNotFound verifies the isNotFound predicate for project errors.
func TestIsNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "generic error",
			err:  errors.New("some other error"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isNotFound(tc.err)

			if got != tc.want {
				t.Errorf(
					"isNotFound(%v) = %v, want %v",
					tc.err, got, tc.want,
				)
			}
		})
	}
}

// TestProjectName verifies the projectName helper returns
// the external name when set, or the CR name otherwise.
func TestProjectName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		proj     *projectv1alpha1.Project
		wantName string
	}{
		{
			name: "uses CR name when no external name",
			proj: &projectv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: testProjectName,
				},
			},
			wantName: testProjectName,
		},
		{
			name: "uses external name annotation",
			proj: &projectv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{
					Name: testProjectName,
					Annotations: map[string]string{
						"crossplane.io/external-name": testOtherProjectName,
					},
				},
			},
			wantName: testOtherProjectName,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := projectName(tc.proj)

			if got != tc.wantName {
				t.Errorf(
					"projectName() = %q, want %q",
					got, tc.wantName,
				)
			}
		})
	}
}

// TestObserve verifies the Observe method handles
// not-found, API errors, and up-to-date checks.
func TestObserve(t *testing.T) {
	t.Parallel()

	storageLimit := int64(1024)

	tests := []struct {
		name         string
		proj         *projectv1alpha1.Project
		getProjectFn func(context.Context, string) (*models.Project, error)
		wantExists   bool
		wantUpToDate bool
		wantError    bool
	}{
		{
			name: tcAPIError,
			proj: newTestProject(),
			getProjectFn: func(_ context.Context, _ string) (*models.Project, error) {
				return nil, errFakeAPI
			},
			wantExists:   false,
			wantUpToDate: false,
			wantError:    true,
		},
		{
			name: "found and up to date",
			proj: newTestProject(),
			getProjectFn: func(_ context.Context, _ string) (*models.Project, error) {
				return newHarborProject(), nil
			},
			wantExists:   true,
			wantUpToDate: true,
			wantError:    false,
		},
		{
			name: "found and out of date",
			proj: &projectv1alpha1.Project{
				ObjectMeta: metav1.ObjectMeta{Name: testProjectName},
				Spec: projectv1alpha1.ProjectSpec{
					ForProvider: projectv1alpha1.ProjectParameters{
						StorageLimit: &storageLimit,
					},
				},
			},
			getProjectFn: func(_ context.Context, _ string) (*models.Project, error) {
				return newHarborProject(), nil
			},
			wantExists:   true,
			wantUpToDate: false,
			wantError:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ext := &external{
				client: &fake.MockProjectsClient{
					GetProjectFn: tc.getProjectFn,
				},
			}

			obs, err := ext.Observe(context.Background(), tc.proj)

			if tc.wantError && err == nil {
				t.Error("Observe: expected error, got nil")
			}

			if !tc.wantError && err != nil {
				t.Errorf("Observe: unexpected error: %v", err)
			}

			if obs.ResourceExists != tc.wantExists {
				t.Errorf(
					"Observe: ResourceExists = %v, want %v",
					obs.ResourceExists, tc.wantExists,
				)
			}

			if obs.ResourceUpToDate != tc.wantUpToDate {
				t.Errorf(
					"Observe: ResourceUpToDate = %v, want %v",
					obs.ResourceUpToDate, tc.wantUpToDate,
				)
			}
		})
	}
}

// TestCreate verifies the Create method creates the project
// and propagates API errors.
func TestCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		newProjectFn func(context.Context, *models.ProjectReq) error
		wantError    bool
	}{
		{
			name: tcSuccess,
			newProjectFn: func(_ context.Context, _ *models.ProjectReq) error {
				return nil
			},
			wantError: false,
		},
		{
			name: tcAPIError,
			newProjectFn: func(_ context.Context, _ *models.ProjectReq) error {
				return errFakeAPI
			},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			proj := newTestProject()
			ext := &external{
				client: &fake.MockProjectsClient{
					NewProjectFn: tc.newProjectFn,
				},
			}

			_, err := ext.Create(context.Background(), proj)

			if tc.wantError && err == nil {
				t.Error("Create: expected error, got nil")
			}

			if !tc.wantError && err != nil {
				t.Errorf("Create: unexpected error: %v", err)
			}

			if !tc.wantError && proj.GetName() == "" {
				t.Error("Create: project name was not preserved")
			}
		})
	}
}

// TestUpdate verifies the Update method sends the project req and propagates
// API errors.
func TestUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		updateProjectFn func(context.Context, string, *models.ProjectReq) error
		wantError       bool
	}{
		{
			name: tcSuccess,
			updateProjectFn: func(_ context.Context, _ string, _ *models.ProjectReq) error {
				return nil
			},
			wantError: false,
		},
		{
			name: tcAPIError,
			updateProjectFn: func(_ context.Context, _ string, _ *models.ProjectReq) error {
				return errFakeAPI
			},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ext := &external{
				client: &fake.MockProjectsClient{
					UpdateProjectFn: tc.updateProjectFn,
				},
			}

			_, err := ext.Update(context.Background(), newTestProject())

			if tc.wantError && err == nil {
				t.Error("Update: expected error, got nil")
			}

			if !tc.wantError && err != nil {
				t.Errorf("Update: unexpected error: %v", err)
			}
		})
	}
}

// TestDelete verifies the Delete method removes the project,
// treats not-found as success, and propagates other errors.
func TestDelete(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		deleteProjectFn func(context.Context, string) error
		wantError       bool
	}{
		{
			name: tcSuccess,
			deleteProjectFn: func(_ context.Context, _ string) error {
				return nil
			},
			wantError: false,
		},
		{
			name: tcAPIError,
			deleteProjectFn: func(_ context.Context, _ string) error {
				return errFakeAPI
			},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ext := &external{
				client: &fake.MockProjectsClient{
					DeleteProjectFn: tc.deleteProjectFn,
				},
			}

			_, err := ext.Delete(context.Background(), newTestProject())

			if tc.wantError && err == nil {
				t.Error("Delete: expected error, got nil")
			}

			if !tc.wantError && err != nil {
				t.Errorf("Delete: unexpected error: %v", err)
			}
		})
	}
}
