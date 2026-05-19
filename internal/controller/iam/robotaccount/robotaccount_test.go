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

package robotaccount

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/goharbor/go-client/pkg/sdk/v2.0/models"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/gate"
	"k8s.io/apimachinery/pkg/runtime/schema"

	iamv1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
	iamclient "github.com/EvannDev/provider-harbor/internal/clients/iam"
	"github.com/EvannDev/provider-harbor/internal/fake"
)

const (
	// testRobotName is the short robot name used in tests.
	testRobotName = "myrobot"
	// testRobotFullName is the Harbor-prefixed robot name for tests.
	testRobotFullName = "robot$myproject+myrobot"
	// testProjectNS is the project namespace used in tests.
	testProjectNS = "myproject"
	// testRobotID is the numeric robot ID used in tests.
	testRobotID = int64(42)
	// testRobotIDStr is the string form of testRobotID.
	testRobotIDStr = "42"
	// testRobotSecret is the secret returned on robot creation.
	testRobotSecret = "super-secret"
	// testDescription is a robot description used in tests.
	testDescription = "my-robot-desc"
	// levelSystem is the system-level robot kind string.
	levelSystem = "system"
	// levelProject is the project-level robot kind string used
	// for both Level and Kind fields.
	levelProject = "project"
	// testOtherRobotName is an alternate robot name used in tests.
	testOtherRobotName = "other-robot"
	// actionPull is the pull access action used in tests.
	actionPull = "pull"
	// resourceRepository is the repository resource type used in tests.
	resourceRepository = "repository"
	// tcSuccess is the test case name for the success path.
	tcSuccess = "success"
	// tcAPIError is the test case name for the API error path.
	tcAPIError = "api error"
	// tcNSNotResolved is the test case name for the unresolved
	// namespace path.
	tcNSNotResolved = "namespace not resolved"
)

// errFakeAPI is a generic API error used in tests.
var errFakeAPI = errors.New("harbor api error")

// newTestAccount returns a minimal RobotAccount for testing.
func newTestAccount() *iamv1alpha1.RobotAccount {
	return &iamv1alpha1.RobotAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: testRobotName,
		},
		Spec: iamv1alpha1.RobotAccountSpec{
			ForProvider: iamv1alpha1.RobotAccountParameters{
				Level: levelSystem,
				Permissions: []iamv1alpha1.RobotAccountPermission{
					{
						Kind:      levelProject,
						Namespace: testProjectNS,
						Access: []iamv1alpha1.RobotAccountAccess{
							{Resource: resourceRepository, Action: actionPull},
						},
					},
				},
			},
		},
	}
}

// newTestRobot returns a minimal Harbor Robot for testing.
func newTestRobot() *models.Robot {
	robotID := testRobotID

	return &models.Robot{
		ID:          robotID,
		Name:        testRobotFullName,
		Description: testDescription,
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

// TestRobotName verifies the robotName helper returns the
// external name when set, or the CR name otherwise.
func TestRobotName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		account  *iamv1alpha1.RobotAccount
		wantName string
	}{
		{
			name: "uses CR name when no external name",
			account: &iamv1alpha1.RobotAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: testRobotName,
				},
			},
			wantName: testRobotName,
		},
		{
			name: "uses external name annotation",
			account: &iamv1alpha1.RobotAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: testRobotName,
					Annotations: map[string]string{
						"crossplane.io/external-name": testOtherRobotName,
					},
				},
			},
			wantName: testOtherRobotName,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := robotName(tc.account)

			if got != tc.wantName {
				t.Errorf(
					"robotName() = %q, want %q",
					got, tc.wantName,
				)
			}
		})
	}
}

// TestFindRobot verifies the four robot lookup strategies
// used by findRobot.
func TestFindRobot(t *testing.T) {
	t.Parallel()

	robot := newTestRobot()

	tests := []struct {
		name       string
		account    *iamv1alpha1.RobotAccount
		mockClient *fake.MockRobotAccountsClient
		wantRobot  *models.Robot
		wantError  bool
	}{
		{
			name: "via status ID",
			account: func() *iamv1alpha1.RobotAccount {
				acc := newTestAccount()
				id := testRobotID
				acc.Status.AtProvider.ID = &id

				return acc
			}(),
			mockClient: &fake.MockRobotAccountsClient{
				GetRobotAccountByIDFn: func(_ context.Context, id int64) (*models.Robot, error) {
					if id == testRobotID {
						return robot, nil
					}

					return nil, errFakeAPI
				},
			},
			wantRobot: robot,
			wantError: false,
		},
		{
			name: "via annotation ID",
			account: &iamv1alpha1.RobotAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: testRobotName,
					Annotations: map[string]string{
						annotationRobotID: testRobotIDStr,
					},
				},
				Spec: iamv1alpha1.RobotAccountSpec{
					ForProvider: iamv1alpha1.RobotAccountParameters{
						Level: levelSystem,
					},
				},
			},
			mockClient: &fake.MockRobotAccountsClient{
				GetRobotAccountByIDFn: func(_ context.Context, _ int64) (*models.Robot, error) {
					return robot, nil
				},
			},
			wantRobot: robot,
			wantError: false,
		},
		{
			name: "project-level with empty namespace",
			account: &iamv1alpha1.RobotAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: testRobotName,
				},
				Spec: iamv1alpha1.RobotAccountSpec{
					ForProvider: iamv1alpha1.RobotAccountParameters{
						Level: levelProject,
						Permissions: []iamv1alpha1.RobotAccountPermission{
							{Kind: levelProject, Namespace: ""},
						},
					},
				},
			},
			mockClient: &fake.MockRobotAccountsClient{},
			wantRobot:  nil,
			wantError:  true,
		},
		{
			name: "project-level with namespace resolved",
			account: &iamv1alpha1.RobotAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: testRobotName,
				},
				Spec: iamv1alpha1.RobotAccountSpec{
					ForProvider: iamv1alpha1.RobotAccountParameters{
						Level: levelProject,
						Permissions: []iamv1alpha1.RobotAccountPermission{
							{Kind: levelProject, Namespace: testProjectNS},
						},
					},
				},
			},
			mockClient: &fake.MockRobotAccountsClient{
				ListProjectRobotsV1Fn: func(_ context.Context, _ string) ([]*models.Robot, error) {
					return []*models.Robot{robot}, nil
				},
			},
			wantRobot: robot,
			wantError: false,
		},
		{
			name: "system-level by name",
			account: &iamv1alpha1.RobotAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name: testRobotName,
				},
				Spec: iamv1alpha1.RobotAccountSpec{
					ForProvider: iamv1alpha1.RobotAccountParameters{
						Level: levelSystem,
					},
				},
			},
			mockClient: &fake.MockRobotAccountsClient{
				GetRobotAccountByNameFn: func(_ context.Context, _ string) (*models.Robot, error) {
					return robot, nil
				},
			},
			wantRobot: robot,
			wantError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ext := &external{client: tc.mockClient}

			got, err := ext.findRobot(context.Background(), tc.account)

			if tc.wantError && err == nil {
				t.Error("findRobot: expected error, got nil")
			}

			if !tc.wantError && err != nil {
				t.Errorf("findRobot: unexpected error: %v", err)
			}

			if got != tc.wantRobot {
				t.Errorf(
					"findRobot: got %v, want %v",
					got, tc.wantRobot,
				)
			}
		})
	}
}

// TestObserve verifies the Observe method handles
// not-found, namespace-unresolved, API errors, and
// up-to-date checks.
func TestObserve(t *testing.T) {
	t.Parallel()

	robot := newTestRobot()
	robotID := testRobotID

	tests := []struct {
		name         string
		account      *iamv1alpha1.RobotAccount
		mockClient   *fake.MockRobotAccountsClient
		wantExists   bool
		wantUpToDate bool
		wantError    bool
	}{
		{
			name: tcNSNotResolved,
			account: &iamv1alpha1.RobotAccount{
				ObjectMeta: metav1.ObjectMeta{Name: testRobotName},
				Spec: iamv1alpha1.RobotAccountSpec{
					ForProvider: iamv1alpha1.RobotAccountParameters{
						Level: levelProject,
						Permissions: []iamv1alpha1.RobotAccountPermission{
							{Kind: levelProject, Namespace: ""},
						},
					},
				},
			},
			mockClient:   &fake.MockRobotAccountsClient{},
			wantExists:   false,
			wantUpToDate: false,
			wantError:    false,
		},
		{
			name: "robot not found",
			account: &iamv1alpha1.RobotAccount{
				ObjectMeta: metav1.ObjectMeta{Name: testRobotName},
				Spec: iamv1alpha1.RobotAccountSpec{
					ForProvider: iamv1alpha1.RobotAccountParameters{
						Level: levelSystem,
					},
				},
			},
			mockClient: &fake.MockRobotAccountsClient{
				GetRobotAccountByNameFn: func(_ context.Context, _ string) (*models.Robot, error) {
					return nil, iamclient.ErrRobotNotFound
				},
			},
			wantExists:   false,
			wantUpToDate: false,
			wantError:    false,
		},
		{
			name: tcAPIError,
			account: func() *iamv1alpha1.RobotAccount {
				acc := newTestAccount()
				acc.Status.AtProvider.ID = &robotID

				return acc
			}(),
			mockClient: &fake.MockRobotAccountsClient{
				GetRobotAccountByIDFn: func(_ context.Context, _ int64) (*models.Robot, error) {
					return nil, errFakeAPI
				},
			},
			wantExists:   false,
			wantUpToDate: false,
			wantError:    true,
		},
		{
			name: "found and up to date",
			account: func() *iamv1alpha1.RobotAccount {
				acc := newTestAccount()
				acc.Spec.ForProvider.Description = testDescription
				acc.Status.AtProvider.ID = &robotID

				return acc
			}(),
			mockClient: &fake.MockRobotAccountsClient{
				GetRobotAccountByIDFn: func(_ context.Context, _ int64) (*models.Robot, error) {
					return &models.Robot{
						ID:          testRobotID,
						Name:        testRobotFullName,
						Description: testDescription,
						Permissions: []*models.RobotPermission{
							{
								Kind:      levelProject,
								Namespace: testProjectNS,
								Access: []*models.Access{
									{Resource: resourceRepository, Action: actionPull},
								},
							},
						},
					}, nil
				},
			},
			wantExists:   true,
			wantUpToDate: true,
			wantError:    false,
		},
		{
			name: "found and out of date",
			account: func() *iamv1alpha1.RobotAccount {
				acc := newTestAccount()
				acc.Spec.ForProvider.Description = "different-description"
				acc.Status.AtProvider.ID = &robotID

				return acc
			}(),
			mockClient: &fake.MockRobotAccountsClient{
				GetRobotAccountByIDFn: func(_ context.Context, _ int64) (*models.Robot, error) {
					return robot, nil
				},
			},
			wantExists:   true,
			wantUpToDate: false,
			wantError:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ext := &external{client: tc.mockClient}

			obs, err := ext.Observe(context.Background(), tc.account)

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

// TestCreate verifies the Create method validates permissions,
// creates the robot, and stores the annotation and connection
// details on success.
func TestCreate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		account           *iamv1alpha1.RobotAccount
		newRobotAccountFn func(context.Context, *models.RobotCreate) (*models.RobotCreated, error)
		wantError         bool
	}{
		{
			name: "validation error empty namespace",
			account: &iamv1alpha1.RobotAccount{
				ObjectMeta: metav1.ObjectMeta{Name: testRobotName},
				Spec: iamv1alpha1.RobotAccountSpec{
					ForProvider: iamv1alpha1.RobotAccountParameters{
						Level: levelSystem,
						Permissions: []iamv1alpha1.RobotAccountPermission{
							{Kind: levelProject, Namespace: ""},
						},
					},
				},
			},
			wantError: true,
		},
		{
			name:    tcAPIError,
			account: newTestAccount(),
			newRobotAccountFn: func(_ context.Context, _ *models.RobotCreate) (*models.RobotCreated, error) {
				return nil, errFakeAPI
			},
			wantError: true,
		},
		{
			name:    tcSuccess,
			account: newTestAccount(),
			newRobotAccountFn: func(_ context.Context, _ *models.RobotCreate) (*models.RobotCreated, error) {
				return &models.RobotCreated{
					ID:     testRobotID,
					Name:   testRobotFullName,
					Secret: testRobotSecret,
				}, nil
			},
			wantError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ext := &external{
				client: &fake.MockRobotAccountsClient{
					NewRobotAccountFn: tc.newRobotAccountFn,
				},
			}

			creation, err := ext.Create(context.Background(), tc.account)

			if tc.wantError && err == nil {
				t.Error("Create: expected error, got nil")
			}

			if !tc.wantError && err != nil {
				t.Errorf("Create: unexpected error: %v", err)
			}

			if !tc.wantError {
				checkCreateResult(t, tc.account, creation.ConnectionDetails)
			}
		})
	}
}

// checkCreateResult validates the Create side effects.
func checkCreateResult(
	t *testing.T,
	account *iamv1alpha1.RobotAccount,
	connDetails map[string][]byte,
) {
	t.Helper()

	wantAnnotation := strconv.FormatInt(testRobotID, 10)
	gotAnnotation := account.GetAnnotations()[annotationRobotID]

	if gotAnnotation != wantAnnotation {
		t.Errorf(
			"Create: annotation %s = %q, want %q",
			annotationRobotID, gotAnnotation, wantAnnotation,
		)
	}

	if string(connDetails["name"]) != testRobotFullName {
		t.Errorf(
			"Create: connection detail 'name' = %q, want %q",
			connDetails["name"], testRobotFullName,
		)
	}

	if string(connDetails["secret"]) != testRobotSecret {
		t.Errorf(
			"Create: connection detail 'secret' = %q, want %q",
			connDetails["secret"], testRobotSecret,
		)
	}
}

// TestUpdate verifies the Update method validates permissions,
// finds the robot, applies parameters, and propagates errors.
func TestUpdate(t *testing.T) {
	t.Parallel()

	robot := newTestRobot()
	robotID := testRobotID

	tests := []struct {
		name                  string
		account               *iamv1alpha1.RobotAccount
		getRobotAccountByIDFn func(context.Context, int64) (*models.Robot, error)
		updateRobotAccountFn  func(context.Context, int64, *models.Robot) error
		wantError             bool
	}{
		{
			name: "validation error",
			account: &iamv1alpha1.RobotAccount{
				ObjectMeta: metav1.ObjectMeta{Name: testRobotName},
				Spec: iamv1alpha1.RobotAccountSpec{
					ForProvider: iamv1alpha1.RobotAccountParameters{
						Level: levelSystem,
						Permissions: []iamv1alpha1.RobotAccountPermission{
							{Kind: levelProject, Namespace: ""},
						},
					},
				},
			},
			wantError: true,
		},
		{
			name: "find robot error",
			account: func() *iamv1alpha1.RobotAccount {
				acc := newTestAccount()
				acc.Status.AtProvider.ID = &robotID

				return acc
			}(),
			getRobotAccountByIDFn: func(_ context.Context, _ int64) (*models.Robot, error) {
				return nil, errFakeAPI
			},
			wantError: true,
		},
		{
			name: "update robot error",
			account: func() *iamv1alpha1.RobotAccount {
				acc := newTestAccount()
				acc.Status.AtProvider.ID = &robotID

				return acc
			}(),
			getRobotAccountByIDFn: func(_ context.Context, _ int64) (*models.Robot, error) {
				return robot, nil
			},
			updateRobotAccountFn: func(_ context.Context, _ int64, _ *models.Robot) error {
				return errFakeAPI
			},
			wantError: true,
		},
		{
			name: tcSuccess,
			account: func() *iamv1alpha1.RobotAccount {
				acc := newTestAccount()
				acc.Status.AtProvider.ID = &robotID

				return acc
			}(),
			getRobotAccountByIDFn: func(_ context.Context, _ int64) (*models.Robot, error) {
				return robot, nil
			},
			updateRobotAccountFn: func(_ context.Context, _ int64, _ *models.Robot) error {
				return nil
			},
			wantError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ext := &external{
				client: &fake.MockRobotAccountsClient{
					GetRobotAccountByIDFn: tc.getRobotAccountByIDFn,
					UpdateRobotAccountFn:  tc.updateRobotAccountFn,
				},
			}

			_, err := ext.Update(context.Background(), tc.account)

			if tc.wantError && err == nil {
				t.Error("Update: expected error, got nil")
			}

			if !tc.wantError && err != nil {
				t.Errorf("Update: unexpected error: %v", err)
			}
		})
	}
}

// TestDelete verifies the Delete method removes the robot,
// treats not-found as success, and propagates other errors.
func TestDelete(t *testing.T) {
	t.Parallel()

	robot := newTestRobot()
	robotID := testRobotID

	tests := []struct {
		name                     string
		account                  *iamv1alpha1.RobotAccount
		getRobotAccountByIDFn    func(context.Context, int64) (*models.Robot, error)
		getRobotAccountByNameFn  func(context.Context, string) (*models.Robot, error)
		deleteRobotAccountByIDFn func(context.Context, int64) error
		wantError                bool
	}{
		{
			name: tcNSNotResolved,
			account: &iamv1alpha1.RobotAccount{
				ObjectMeta: metav1.ObjectMeta{Name: testRobotName},
				Spec: iamv1alpha1.RobotAccountSpec{
					ForProvider: iamv1alpha1.RobotAccountParameters{
						Level: levelProject,
						Permissions: []iamv1alpha1.RobotAccountPermission{
							{Kind: levelProject, Namespace: ""},
						},
					},
				},
			},
			wantError: false,
		},
		{
			name: "robot not found is idempotent",
			account: &iamv1alpha1.RobotAccount{
				ObjectMeta: metav1.ObjectMeta{Name: testRobotName},
				Spec: iamv1alpha1.RobotAccountSpec{
					ForProvider: iamv1alpha1.RobotAccountParameters{
						Level: levelSystem,
					},
				},
			},
			getRobotAccountByNameFn: func(_ context.Context, _ string) (*models.Robot, error) {
				return nil, iamclient.ErrRobotNotFound
			},
			wantError: false,
		},
		{
			name: "delete not found is idempotent",
			account: func() *iamv1alpha1.RobotAccount {
				acc := newTestAccount()
				acc.Status.AtProvider.ID = &robotID

				return acc
			}(),
			getRobotAccountByIDFn: func(_ context.Context, _ int64) (*models.Robot, error) {
				return robot, nil
			},
			deleteRobotAccountByIDFn: func(_ context.Context, _ int64) error {
				return iamclient.ErrRobotNotFound
			},
			wantError: false,
		},
		{
			name: "delete api error",
			account: func() *iamv1alpha1.RobotAccount {
				acc := newTestAccount()
				acc.Status.AtProvider.ID = &robotID

				return acc
			}(),
			getRobotAccountByIDFn: func(_ context.Context, _ int64) (*models.Robot, error) {
				return robot, nil
			},
			deleteRobotAccountByIDFn: func(_ context.Context, _ int64) error {
				return errFakeAPI
			},
			wantError: true,
		},
		{
			name: tcSuccess,
			account: func() *iamv1alpha1.RobotAccount {
				acc := newTestAccount()
				acc.Status.AtProvider.ID = &robotID

				return acc
			}(),
			getRobotAccountByIDFn: func(_ context.Context, _ int64) (*models.Robot, error) {
				return robot, nil
			},
			deleteRobotAccountByIDFn: func(_ context.Context, _ int64) error {
				return nil
			},
			wantError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ext := &external{
				client: &fake.MockRobotAccountsClient{
					GetRobotAccountByIDFn:    tc.getRobotAccountByIDFn,
					GetRobotAccountByNameFn:  tc.getRobotAccountByNameFn,
					DeleteRobotAccountByIDFn: tc.deleteRobotAccountByIDFn,
				},
			}

			_, err := ext.Delete(context.Background(), tc.account)

			if tc.wantError && err == nil {
				t.Error("Delete: expected error, got nil")
			}

			if !tc.wantError && err != nil {
				t.Errorf("Delete: unexpected error: %v", err)
			}
		})
	}
}
