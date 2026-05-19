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

package group

import (
	"context"
	"errors"
	"testing"

	"github.com/goharbor/go-client/pkg/sdk/v2.0/client/usergroup"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/models"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"

	v1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
	apisv1alpha1 "github.com/EvannDev/provider-harbor/apis/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/clients/common"
	iam "github.com/EvannDev/provider-harbor/internal/clients/iam"
	fakepkg "github.com/EvannDev/provider-harbor/internal/fake"
)

// mockGate is a test implementation of the controller.Gate interface.
type mockGate struct {
	registered bool
	callback   func()
	gvks       []schema.GroupVersionKind
}

// Register stores the callback and GVKs without invoking them.
func (m *mockGate) Register(callback func(), gvks ...schema.GroupVersionKind) {
	m.registered = true
	m.callback = callback
	m.gvks = append(m.gvks, gvks...)
}

// Set is a no-op for the mock gate.
func (m *mockGate) Set(_ schema.GroupVersionKind, _ bool) bool {
	return false
}

// makeGroup creates a Group resource for use in tests.
func makeGroup(name, externalName string, spec v1alpha1.GroupParameters) *v1alpha1.Group {
	g := &v1alpha1.Group{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: v1alpha1.GroupSpec{
			ForProvider: spec,
		},
	}

	if externalName != "" {
		meta.SetExternalName(g, externalName)
	}

	return g
}

// buildConnectScheme creates a scheme with all types needed by Connect tests.
func buildConnectScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()

	err := corev1.AddToScheme(s)
	if err != nil {
		t.Fatalf("cannot add corev1 scheme: %v", err)
	}

	err = apisv1alpha1.SchemeBuilder.AddToScheme(s)
	if err != nil {
		t.Fatalf("cannot add apisv1alpha1 scheme: %v", err)
	}

	err = v1alpha1.SchemeBuilder.AddToScheme(s)
	if err != nil {
		t.Fatalf("cannot add iamv1alpha1 scheme: %v", err)
	}

	return s
}

// errAPIFailure is a sentinel error used in tests.
var errAPIFailure = errors.New("api failure")

// TestObserve tests the Observe method of the external client.
func TestObserve(t *testing.T) {
	t.Parallel()

	defaultSpec := v1alpha1.GroupParameters{Name: "mygroup", Type: "internal"}

	tests := []struct {
		name    string
		client  *fakepkg.MockGroupsClient
		mg      resource.Managed
		want    managed.ExternalObservation
		wantErr bool
	}{
		{
			name:    "wrong resource type returns error",
			client:  &fakepkg.MockGroupsClient{},
			mg:      &v1alpha1.RobotAccount{},
			wantErr: true,
		},
		{
			name: "no external name, search returns empty, resource does not exist",
			client: &fakepkg.MockGroupsClient{
				SearchUserGroupsFn: func(_ context.Context, _ *usergroup.SearchUserGroupsParams) (*usergroup.SearchUserGroupsOK, error) {
					return &usergroup.SearchUserGroupsOK{
						Payload: []*models.UserGroupSearchItem{},
					}, nil
				},
			},
			mg:   makeGroup("test", "", defaultSpec),
			want: managed.ExternalObservation{ResourceExists: false},
		},
		{
			name: "no external name, search API error returns error",
			client: &fakepkg.MockGroupsClient{
				SearchUserGroupsFn: func(_ context.Context, _ *usergroup.SearchUserGroupsParams) (*usergroup.SearchUserGroupsOK, error) {
					return nil, errAPIFailure
				},
			},
			mg:      makeGroup("test", "", defaultSpec),
			wantErr: true,
		},
		{
			name: "no external name, group found on first page, resource exists",
			client: &fakepkg.MockGroupsClient{
				SearchUserGroupsFn: func(_ context.Context, _ *usergroup.SearchUserGroupsParams) (*usergroup.SearchUserGroupsOK, error) {
					return &usergroup.SearchUserGroupsOK{
						XTotalCount: 1,
						Payload: []*models.UserGroupSearchItem{
							{ID: 42, GroupName: "mygroup"},
						},
					}, nil
				},
				GetUserGroupFn: func(_ context.Context, params *usergroup.GetUserGroupParams) (*usergroup.GetUserGroupOK, error) {
					return &usergroup.GetUserGroupOK{
						Payload: &models.UserGroup{
							ID:        params.GroupID,
							GroupName: "mygroup",
							GroupType: 2,
						},
					}, nil
				},
			},
			mg: makeGroup("test", "", defaultSpec),
			want: managed.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: true,
			},
		},
		{
			name: "no external name, group found on second page, resource exists",
			client: &fakepkg.MockGroupsClient{
				SearchUserGroupsFn: func() func(_ context.Context, params *usergroup.SearchUserGroupsParams) (*usergroup.SearchUserGroupsOK, error) {
					call := 0

					return func(_ context.Context, _ *usergroup.SearchUserGroupsParams) (*usergroup.SearchUserGroupsOK, error) {
						call++
						if call == 1 {
							// First page: 100 items, none match
							items := make([]*models.UserGroupSearchItem, 100)
							for i := range items {
								items[i] = &models.UserGroupSearchItem{ID: int64(i + 1), GroupName: "other"}
							}

							return &usergroup.SearchUserGroupsOK{
								XTotalCount: 101,
								Payload:     items,
							}, nil
						}

						// Second page: group found
						return &usergroup.SearchUserGroupsOK{
							XTotalCount: 101,
							Payload: []*models.UserGroupSearchItem{
								{ID: 99, GroupName: "mygroup"},
							},
						}, nil
					}
				}(),
				GetUserGroupFn: func(_ context.Context, params *usergroup.GetUserGroupParams) (*usergroup.GetUserGroupOK, error) {
					return &usergroup.GetUserGroupOK{
						Payload: &models.UserGroup{
							ID:        params.GroupID,
							GroupName: "mygroup",
							GroupType: 2,
						},
					}, nil
				},
			},
			mg: makeGroup("test", "", defaultSpec),
			want: managed.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: true,
			},
		},
		{
			name: "external name set, get succeeds, up to date",
			client: &fakepkg.MockGroupsClient{
				GetUserGroupFn: func(_ context.Context, _ *usergroup.GetUserGroupParams) (*usergroup.GetUserGroupOK, error) {
					return &usergroup.GetUserGroupOK{
						Payload: &models.UserGroup{
							ID:        10,
							GroupName: "mygroup",
							GroupType: 2,
						},
					}, nil
				},
			},
			mg: makeGroup("test", "10", defaultSpec),
			want: managed.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: true,
			},
		},
		{
			name: "external name set, get succeeds, name differs, not up to date",
			client: &fakepkg.MockGroupsClient{
				GetUserGroupFn: func(_ context.Context, _ *usergroup.GetUserGroupParams) (*usergroup.GetUserGroupOK, error) {
					return &usergroup.GetUserGroupOK{
						Payload: &models.UserGroup{
							ID:        10,
							GroupName: "differentname",
							GroupType: 2,
						},
					}, nil
				},
			},
			mg: makeGroup("test", "10", defaultSpec),
			want: managed.ExternalObservation{
				ResourceExists:   true,
				ResourceUpToDate: false,
			},
		},
		{
			name:    "external name set but invalid returns error",
			client:  &fakepkg.MockGroupsClient{},
			mg:      makeGroup("test", "not-a-number", defaultSpec),
			wantErr: true,
		},
		{
			name: "external name set, get API error returns error",
			client: &fakepkg.MockGroupsClient{
				GetUserGroupFn: func(_ context.Context, _ *usergroup.GetUserGroupParams) (*usergroup.GetUserGroupOK, error) {
					return nil, errAPIFailure
				},
			},
			mg:      makeGroup("test", "10", defaultSpec),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := &external{client: tc.client}

			obs, err := e.Observe(context.Background(), tc.mg)

			if tc.wantErr {
				if err == nil {
					t.Error("Observe() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("Observe() unexpected error: %v", err)

				return
			}

			if obs.ResourceExists != tc.want.ResourceExists {
				t.Errorf("ResourceExists = %v, want %v", obs.ResourceExists, tc.want.ResourceExists)
			}

			if obs.ResourceUpToDate != tc.want.ResourceUpToDate {
				t.Errorf("ResourceUpToDate = %v, want %v", obs.ResourceUpToDate, tc.want.ResourceUpToDate)
			}
		})
	}
}

// TestCreate tests the Create method of the external client.
func TestCreate(t *testing.T) {
	t.Parallel()

	defaultSpec := v1alpha1.GroupParameters{Name: "mygroup", Type: "internal"}

	tests := []struct {
		name    string
		client  *fakepkg.MockGroupsClient
		mg      resource.Managed
		wantErr bool
	}{
		{
			name:    "wrong resource type returns error",
			client:  &fakepkg.MockGroupsClient{},
			mg:      &v1alpha1.RobotAccount{},
			wantErr: true,
		},
		{
			name: "create succeeds returns empty connection details",
			client: &fakepkg.MockGroupsClient{
				CreateUserGroupFn: func(_ context.Context, _ *usergroup.CreateUserGroupParams) (*usergroup.CreateUserGroupCreated, error) {
					return &usergroup.CreateUserGroupCreated{}, nil
				},
			},
			mg: makeGroup("test", "", defaultSpec),
		},
		{
			name: "create API error returns error",
			client: &fakepkg.MockGroupsClient{
				CreateUserGroupFn: func(_ context.Context, _ *usergroup.CreateUserGroupParams) (*usergroup.CreateUserGroupCreated, error) {
					return nil, errAPIFailure
				},
			},
			mg:      makeGroup("test", "", defaultSpec),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := &external{client: tc.client}

			creation, err := e.Create(context.Background(), tc.mg)

			if tc.wantErr {
				if err == nil {
					t.Error("Create() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("Create() unexpected error: %v", err)

				return
			}

			if creation.ConnectionDetails == nil {
				t.Error("Create() ConnectionDetails is nil, want empty map")
			}
		})
	}
}

// TestUpdate tests the Update method of the external client.
func TestUpdate(t *testing.T) {
	t.Parallel()

	defaultSpec := v1alpha1.GroupParameters{Name: "mygroup", Type: "internal"}

	tests := []struct {
		name    string
		client  *fakepkg.MockGroupsClient
		mg      resource.Managed
		wantErr bool
	}{
		{
			name:    "wrong resource type returns error",
			client:  &fakepkg.MockGroupsClient{},
			mg:      &v1alpha1.RobotAccount{},
			wantErr: true,
		},
		{
			name:    "external name not set returns error",
			client:  &fakepkg.MockGroupsClient{},
			mg:      makeGroup("test", "", defaultSpec),
			wantErr: true,
		},
		{
			name:    "external name invalid returns error",
			client:  &fakepkg.MockGroupsClient{},
			mg:      makeGroup("test", "not-a-number", defaultSpec),
			wantErr: true,
		},
		{
			name: "update succeeds returns empty connection details",
			client: &fakepkg.MockGroupsClient{
				UpdateUserGroupFn: func(_ context.Context, _ *usergroup.UpdateUserGroupParams) (*usergroup.UpdateUserGroupOK, error) {
					return &usergroup.UpdateUserGroupOK{}, nil
				},
			},
			mg: makeGroup("test", "10", defaultSpec),
		},
		{
			name: "update API error returns error",
			client: &fakepkg.MockGroupsClient{
				UpdateUserGroupFn: func(_ context.Context, _ *usergroup.UpdateUserGroupParams) (*usergroup.UpdateUserGroupOK, error) {
					return nil, errAPIFailure
				},
			},
			mg:      makeGroup("test", "10", defaultSpec),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := &external{client: tc.client}

			update, err := e.Update(context.Background(), tc.mg)

			if tc.wantErr {
				if err == nil {
					t.Error("Update() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("Update() unexpected error: %v", err)

				return
			}

			if update.ConnectionDetails == nil {
				t.Error("Update() ConnectionDetails is nil, want empty map")
			}
		})
	}
}

// TestDelete tests the Delete method of the external client.
func TestDelete(t *testing.T) {
	t.Parallel()

	defaultSpec := v1alpha1.GroupParameters{Name: "mygroup", Type: "internal"}

	tests := []struct {
		name    string
		client  *fakepkg.MockGroupsClient
		mg      resource.Managed
		wantErr bool
	}{
		{
			name:    "wrong resource type returns error",
			client:  &fakepkg.MockGroupsClient{},
			mg:      &v1alpha1.RobotAccount{},
			wantErr: true,
		},
		{
			name:    "external name not set returns error",
			client:  &fakepkg.MockGroupsClient{},
			mg:      makeGroup("test", "", defaultSpec),
			wantErr: true,
		},
		{
			name:    "external name invalid returns error",
			client:  &fakepkg.MockGroupsClient{},
			mg:      makeGroup("test", "not-a-number", defaultSpec),
			wantErr: true,
		},
		{
			name: "delete succeeds",
			client: &fakepkg.MockGroupsClient{
				DeleteUserGroupFn: func(_ context.Context, _ *usergroup.DeleteUserGroupParams) (*usergroup.DeleteUserGroupOK, error) {
					return &usergroup.DeleteUserGroupOK{}, nil
				},
			},
			mg: makeGroup("test", "10", defaultSpec),
		},
		{
			name: "delete API error returns error",
			client: &fakepkg.MockGroupsClient{
				DeleteUserGroupFn: func(_ context.Context, _ *usergroup.DeleteUserGroupParams) (*usergroup.DeleteUserGroupOK, error) {
					return nil, errAPIFailure
				},
			},
			mg:      makeGroup("test", "10", defaultSpec),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			e := &external{client: tc.client}

			_, err := e.Delete(context.Background(), tc.mg)

			if tc.wantErr {
				if err == nil {
					t.Error("Delete() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("Delete() unexpected error: %v", err)
			}
		})
	}
}

// TestDisconnect tests that Disconnect always returns nil.
func TestDisconnect(t *testing.T) {
	t.Parallel()

	e := &external{client: &fakepkg.MockGroupsClient{}}

	err := e.Disconnect(context.Background())
	if err != nil {
		t.Errorf("Disconnect() unexpected error: %v", err)
	}
}

// TestConnect tests the Connect method of the connector.
func TestConnect(t *testing.T) {
	t.Parallel()

	defaultSpec := v1alpha1.GroupParameters{Name: "mygroup", Type: iam.GroupTypeInternal}

	tests := []struct {
		name        string
		buildClient func(s *runtime.Scheme) *connector
		mg          resource.Managed
		wantErr     bool
		wantNonNil  bool
	}{
		{
			name: "wrong resource type returns error",
			buildClient: func(s *runtime.Scheme) *connector {
				fakeClient := kfake.NewClientBuilder().WithScheme(s).Build()

				return &connector{
					kube:         fakeClient,
					usage:        resource.NewProviderConfigUsageTracker(fakeClient, &apisv1alpha1.ProviderConfigUsage{}),
					newServiceFn: func(_ common.Config) iam.GroupsClient { return &fakepkg.MockGroupsClient{} },
				}
			},
			mg:      &v1alpha1.RobotAccount{},
			wantErr: true,
		},
		{
			name: "ProviderConfig not found returns error",
			buildClient: func(s *runtime.Scheme) *connector {
				group := &v1alpha1.Group{
					ObjectMeta: metav1.ObjectMeta{Name: "mygroup", Namespace: "default"},
					Spec: v1alpha1.GroupSpec{
						ManagedResourceSpec: xpv2.ManagedResourceSpec{
							ProviderConfigReference: &xpv1.ProviderConfigReference{
								Kind: "ProviderConfig",
								Name: "default",
							},
						},
						ForProvider: defaultSpec,
					},
				}
				fakeClient := kfake.NewClientBuilder().WithScheme(s).WithObjects(group).Build()

				return &connector{
					kube:         fakeClient,
					usage:        resource.NewProviderConfigUsageTracker(fakeClient, &apisv1alpha1.ProviderConfigUsage{}),
					newServiceFn: func(_ common.Config) iam.GroupsClient { return &fakepkg.MockGroupsClient{} },
				}
			},
			mg: &v1alpha1.Group{
				ObjectMeta: metav1.ObjectMeta{Name: "mygroup", Namespace: "default"},
				Spec: v1alpha1.GroupSpec{
					ManagedResourceSpec: xpv2.ManagedResourceSpec{
						ProviderConfigReference: &xpv1.ProviderConfigReference{
							Kind: "ProviderConfig",
							Name: "default",
						},
					},
					ForProvider: defaultSpec,
				},
			},
			wantErr: true,
		},
		{
			name: "success with ProviderConfig and Secret",
			buildClient: func(s *runtime.Scheme) *connector {
				secretData := []byte(`{"username":"admin","password":"secret"}`)
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "harbor-creds", Namespace: "default"},
					Data:       map[string][]byte{"credentials": secretData},
				}
				providerConfig := &apisv1alpha1.ProviderConfig{
					ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
					Spec: apisv1alpha1.ProviderConfigSpec{
						URL: "http://localhost:9000",
						Credentials: apisv1alpha1.ProviderCredentials{
							Source: xpv1.CredentialsSourceSecret,
							CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
								SecretRef: &xpv1.SecretKeySelector{
									SecretReference: xpv1.SecretReference{
										Name:      "harbor-creds",
										Namespace: "default",
									},
									Key: "credentials",
								},
							},
						},
					},
				}
				group := &v1alpha1.Group{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mygroup",
						Namespace: "default",
						UID:       "test-uid-1234",
					},
					Spec: v1alpha1.GroupSpec{
						ManagedResourceSpec: xpv2.ManagedResourceSpec{
							ProviderConfigReference: &xpv1.ProviderConfigReference{
								Kind: "ProviderConfig",
								Name: "default",
							},
						},
						ForProvider: defaultSpec,
					},
				}
				fakeClient := kfake.NewClientBuilder().WithScheme(s).WithObjects(secret, providerConfig, group).Build()

				return &connector{
					kube:         fakeClient,
					usage:        resource.NewProviderConfigUsageTracker(fakeClient, &apisv1alpha1.ProviderConfigUsage{}),
					newServiceFn: func(_ common.Config) iam.GroupsClient { return &fakepkg.MockGroupsClient{} },
				}
			},
			mg: &v1alpha1.Group{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mygroup",
					Namespace: "default",
					UID:       "test-uid-1234",
				},
				Spec: v1alpha1.GroupSpec{
					ManagedResourceSpec: xpv2.ManagedResourceSpec{
						ProviderConfigReference: &xpv1.ProviderConfigReference{
							Kind: "ProviderConfig",
							Name: "default",
						},
					},
					ForProvider: defaultSpec,
				},
			},
			wantNonNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := buildConnectScheme(t)
			c := tc.buildClient(s)

			ec, err := c.Connect(context.Background(), tc.mg)

			if tc.wantErr {
				if err == nil {
					t.Error("Connect() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("Connect() unexpected error: %v", err)

				return
			}

			if tc.wantNonNil && ec == nil {
				t.Error("Connect() returned nil ExternalClient, want non-nil")
			}
		})
	}
}

// TestSetupGatedRegistersGroupGVK verifies that SetupGated registers
// the Group GVK with the controller gate without requiring a live manager.
func TestSetupGatedRegistersGroupGVK(t *testing.T) {
	t.Parallel()

	g := &mockGate{}
	o := controller.DefaultOptions()
	o.Gate = g

	err := SetupGated(nil, o)
	if err != nil {
		t.Fatalf("SetupGated() unexpected error: %v", err)
	}

	if !g.registered {
		t.Fatal("SetupGated() did not call Gate.Register")
	}

	if g.callback == nil {
		t.Fatal("SetupGated() registered a nil callback")
	}

	if len(g.gvks) != 1 {
		t.Fatalf("SetupGated() registered %d GVKs, want 1", len(g.gvks))
	}

	if diff := cmp.Diff(v1alpha1.GroupGroupVersionKind, g.gvks[0]); diff != "" {
		t.Fatalf("SetupGated() GVK mismatch (-want +got):\n%s", diff)
	}
}

// TestSetupRequiresNonNilManager verifies Setup panics with nil manager,
// covering the initial Setup statements before manager access.
func TestSetupRequiresNonNilManager(t *testing.T) {
	t.Parallel()

	defer func() {
		if r := recover(); r == nil {
			t.Error("Setup() expected panic with nil manager, got none")
		}
	}()

	_ = Setup(nil, controller.DefaultOptions())
}

// TestSetupGatedCallbackInvokesSetup verifies the closure registered by
// SetupGated forwards to Setup. Since Setup requires a non-nil manager,
// invoking the callback with nil panics — we recover and confirm the path.
func TestSetupGatedCallbackInvokesSetup(t *testing.T) {
	t.Parallel()

	g := &mockGate{}
	o := controller.DefaultOptions()
	o.Gate = g

	_ = SetupGated(nil, o)

	panicked := false

	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()

		g.callback()
	}()

	if !panicked {
		t.Error("SetupGated callback expected panic with nil manager, got none")
	}
}
