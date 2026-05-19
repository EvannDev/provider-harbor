//go:build e2e

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

package iam_test

import (
	"context"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	iamv1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
	iamclient "github.com/EvannDev/provider-harbor/internal/clients/iam"
	"github.com/EvannDev/provider-harbor/internal/test/e2e"
)

// TestInternalGroupCRUD creates an internal Harbor Group CR (without a preset
// external-name), waits for Ready, and verifies the group exists in Harbor with
// the correct name and type. It then renames the group and verifies Harbor
// reflects the change.
func TestInternalGroupCRUD(t *testing.T) {
	t.Parallel()

	f := e2e.New(t)

	const (
		crName    = "e2e-internal-group-crud"
		groupName = "e2e-internal-group-crud"
	)

	group := &iamv1alpha1.Group{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: testNamespace,
			// No external-name annotation: the controller discovers the group
			// by name search and sets it after the first successful Observe.
		},
		Spec: iamv1alpha1.GroupSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				ProviderConfigReference: &xpv1.ProviderConfigReference{
					Kind: "ClusterProviderConfig",
					Name: f.ProviderConfigName,
				},
			},
			ForProvider: iamv1alpha1.GroupParameters{
				Name: groupName,
				Type: iamclient.GroupTypeInternal,
			},
		},
	}

	f.CreateAndWaitForReady(t, group, defaultTimeout)
	e2e.AssertReady(t, group)
	e2e.AssertSynced(t, group)

	// The controller sets external-name to the Harbor group ID after first Observe.
	externalName := meta.GetExternalName(group)
	if externalName == "" {
		t.Fatalf("expected external-name to be populated after Ready")
	}

	if group.Status.AtProvider.ID == 0 {
		t.Fatalf("expected status.atProvider.id to be populated after Ready")
	}

	got, err := f.FetchUserGroupByID(group.Status.AtProvider.ID)
	if err != nil {
		t.Fatalf("fetching group from Harbor: %v", err)
	}
	if got == nil {
		t.Fatalf("group %q (id=%d) not found in Harbor after Ready", groupName, group.Status.AtProvider.ID)
	}
	if got.GroupName != groupName {
		t.Errorf("group name = %q, want %q", got.GroupName, groupName)
	}
	if group.Status.AtProvider.Type != iamclient.GroupTypeInternal {
		t.Errorf("status.atProvider.type = %q, want %q", group.Status.AtProvider.Type, iamclient.GroupTypeInternal)
	}

	// Update: rename the group.
	const updatedName = "e2e-internal-group-crud-renamed"
	if err := f.Kube.Get(context.Background(), kubeKey(testNamespace, crName), group); err != nil {
		t.Fatalf("re-fetching Group CR: %v", err)
	}
	group.Spec.ForProvider.Name = updatedName
	f.Update(t, group)
	if err := f.WaitForReady(context.Background(), group, defaultTimeout); err != nil {
		t.Fatalf("waiting for renamed Group to be Ready: %v\n  conditions: %s",
			err, e2e.SummariseConditions(group))
	}

	renamed, err := f.FetchUserGroupByID(group.Status.AtProvider.ID)
	if err != nil {
		t.Fatalf("fetching renamed group from Harbor: %v", err)
	}
	if renamed == nil {
		t.Fatalf("group (id=%d) not found in Harbor after rename", group.Status.AtProvider.ID)
	}
	if renamed.GroupName != updatedName {
		t.Errorf("renamed group name = %q, want %q", renamed.GroupName, updatedName)
	}
}

// TestOIDCGroupCRUD creates an OIDC Harbor Group CR, waits for Ready, and
// verifies the group exists in Harbor with the correct name and type.
func TestOIDCGroupCRUD(t *testing.T) {
	t.Parallel()

	f := e2e.New(t)

	const (
		crName    = "e2e-oidc-group-crud"
		groupName = "e2e-oidc-group-crud"
	)

	group := &iamv1alpha1.Group{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: testNamespace,
		},
		Spec: iamv1alpha1.GroupSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				ProviderConfigReference: &xpv1.ProviderConfigReference{
					Kind: "ClusterProviderConfig",
					Name: f.ProviderConfigName,
				},
			},
			ForProvider: iamv1alpha1.GroupParameters{
				Name: groupName,
				Type: iamclient.GroupTypeOIDC,
			},
		},
	}

	f.CreateAndWaitForReady(t, group, defaultTimeout)
	e2e.AssertReady(t, group)
	e2e.AssertSynced(t, group)

	externalName := meta.GetExternalName(group)
	if externalName == "" {
		t.Fatalf("expected external-name to be populated after Ready")
	}

	if group.Status.AtProvider.ID == 0 {
		t.Fatalf("expected status.atProvider.id to be populated after Ready")
	}

	got, err := f.FetchUserGroupByID(group.Status.AtProvider.ID)
	if err != nil {
		t.Fatalf("fetching group from Harbor: %v", err)
	}
	if got == nil {
		t.Fatalf("group %q (id=%d) not found in Harbor after Ready", groupName, group.Status.AtProvider.ID)
	}
	if got.GroupName != groupName {
		t.Errorf("group name = %q, want %q", got.GroupName, groupName)
	}
	if group.Status.AtProvider.Type != iamclient.GroupTypeOIDC {
		t.Errorf("status.atProvider.type = %q, want %q", group.Status.AtProvider.Type, iamclient.GroupTypeOIDC)
	}
}
