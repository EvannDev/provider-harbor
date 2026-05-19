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

package common

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	crtfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"

	iamv1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
	apisv1alpha1 "github.com/EvannDev/provider-harbor/apis/v1alpha1"
)

// buildTestScheme creates a runtime scheme that includes both the core k8s
// types and the provider-harbor ProviderConfig types.
func buildTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	s := runtime.NewScheme()

	err := clientgoscheme.AddToScheme(s)
	if err != nil {
		t.Fatalf("cannot add client-go scheme: %v", err)
	}

	err = apisv1alpha1.SchemeBuilder.AddToScheme(s)
	if err != nil {
		t.Fatalf("cannot add apisv1alpha1 scheme: %v", err)
	}

	return s
}

// stubManaged wraps a v1alpha1.Group to provide a concrete ModernManaged
// for use in GetConfig tests.
type stubManaged struct {
	iamv1alpha1.Group
}

// TestNewHarborAPI verifies that NewHarborAPI creates a client correctly.
func TestNewHarborAPI(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         Config
		expectPanic bool
	}{
		{
			name: "valid URL with InsecureSkipTLSVerify true returns non-nil",
			cfg: Config{
				URL:                   "https://harbor.example.com",
				Username:              "admin",
				Password:              "password",
				InsecureSkipTLSVerify: true,
			},
			expectPanic: false,
		},
		{
			name: "valid URL with InsecureSkipTLSVerify false returns non-nil",
			cfg: Config{
				URL:                   "https://harbor.example.com",
				Username:              "admin",
				Password:              "password",
				InsecureSkipTLSVerify: false,
			},
			expectPanic: false,
		},
		{
			name: "invalid URL panics",
			cfg: Config{
				URL:      "://invalid-url",
				Username: "admin",
				Password: "password",
			},
			expectPanic: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.expectPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Error("NewHarborAPI() expected panic for invalid URL, got none")
					}
				}()
			}

			got := NewHarborAPI(tc.cfg)

			if !tc.expectPanic && got == nil {
				t.Error("NewHarborAPI() returned nil, want non-nil")
			}
		})
	}
}

// secretCredentialObjects returns a ProviderConfig and Secret for use in tests.
func secretCredentialObjects(credsJSON string) []runtime.Object {
	return []runtime.Object{
		&apisv1alpha1.ProviderConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-config",
				Namespace: "default",
			},
			Spec: apisv1alpha1.ProviderConfigSpec{
				URL:                   "https://harbor.example.com",
				InsecureSkipTLSVerify: credsJSON != `not-valid-json`,
				Credentials: apisv1alpha1.ProviderCredentials{
					Source: xpv1.CredentialsSourceSecret,
					CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
						SecretRef: &xpv1.SecretKeySelector{
							SecretReference: xpv1.SecretReference{
								Name:      "harbor-secret",
								Namespace: "default",
							},
							Key: "credentials",
						},
					},
				},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "harbor-secret",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"credentials": []byte(credsJSON),
			},
		},
	}
}

// stubManagedWithRef creates a stubManaged with a ProviderConfig reference.
func stubManagedWithRef(kind, name, namespace string) *stubManaged {
	mg := &stubManaged{}
	mg.Spec.ProviderConfigReference = &xpv1.ProviderConfigReference{
		Kind: kind,
		Name: name,
	}
	mg.Namespace = namespace

	return mg
}

// assertConfig checks that got matches the expected Config values.
func assertConfig(t *testing.T, got, want *Config) {
	t.Helper()

	if got == nil {
		t.Fatal("GetConfig() returned nil config")
	}

	if got.URL != want.URL {
		t.Errorf("URL = %q, want %q", got.URL, want.URL)
	}

	if got.Username != want.Username {
		t.Errorf("Username = %q, want %q", got.Username, want.Username)
	}

	if got.Password != want.Password {
		t.Errorf("Password = %q, want %q", got.Password, want.Password)
	}

	if got.InsecureSkipTLSVerify != want.InsecureSkipTLSVerify {
		t.Errorf("InsecureSkipTLSVerify = %v, want %v", got.InsecureSkipTLSVerify, want.InsecureSkipTLSVerify)
	}
}

// TestGetConfig verifies that GetConfig correctly constructs a Config.
func TestGetConfig(t *testing.T) {
	t.Parallel()

	validCredsJSON := `{"username":"testuser","password":"testpass"}`
	invalidCredsJSON := `not-valid-json`

	tests := []struct {
		name         string
		setupManaged func() *stubManaged
		setupObjects []runtime.Object
		wantErr      bool
		wantConfig   *Config
	}{
		{
			name: "nil ProviderConfigReference returns error",
			setupManaged: func() *stubManaged {
				return &stubManaged{}
			},
			wantErr: true,
		},
		{
			name:         "ProviderConfig kind with non-existing object returns error",
			setupManaged: func() *stubManaged { return stubManagedWithRef("ProviderConfig", "nonexistent", "default") },
			wantErr:      true,
		},
		{
			name:         "ClusterProviderConfig kind with non-existing object returns error",
			setupManaged: func() *stubManaged { return stubManagedWithRef(clusterProviderConfigKind, "nonexistent", "") },
			wantErr:      true,
		},
		{
			name:         "ProviderConfig exists but credentials JSON is invalid returns error",
			setupManaged: func() *stubManaged { return stubManagedWithRef("ProviderConfig", "my-config", "default") },
			setupObjects: secretCredentialObjects(invalidCredsJSON),
			wantErr:      true,
		},
		{
			name:         "ProviderConfig exists with valid JSON credentials returns correct Config",
			setupManaged: func() *stubManaged { return stubManagedWithRef("ProviderConfig", "my-config", "default") },
			setupObjects: secretCredentialObjects(validCredsJSON),
			wantConfig: &Config{
				URL:                   "https://harbor.example.com",
				Username:              "testuser",
				Password:              "testpass",
				InsecureSkipTLSVerify: true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := buildTestScheme(t)

			builder := crtfake.NewClientBuilder().WithScheme(s)

			for _, obj := range tc.setupObjects {
				builder = builder.WithRuntimeObjects(obj)
			}

			kube := builder.Build()
			mg := tc.setupManaged()

			got, err := GetConfig(context.Background(), kube, mg)

			if tc.wantErr {
				if err == nil {
					t.Error("GetConfig() expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("GetConfig() unexpected error: %v", err)

				return
			}

			if tc.wantConfig != nil {
				assertConfig(t, got, tc.wantConfig)
			}
		})
	}
}

// Ensure stubManaged satisfies the interface at compile time.
var _ interface {
	GetProviderConfigReference() *xpv1.ProviderConfigReference
	GetNamespace() string
} = &stubManaged{}

// GetProviderConfigReference delegates to the embedded ManagedResourceSpec.
func (s *stubManaged) GetProviderConfigReference() *xpv1.ProviderConfigReference {
	return s.Spec.ProviderConfigReference
}

// SetProviderConfigReference delegates to the embedded ManagedResourceSpec.
func (s *stubManaged) SetProviderConfigReference(r *xpv1.ProviderConfigReference) {
	s.Spec.ProviderConfigReference = r
}
