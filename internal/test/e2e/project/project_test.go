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

package project_test

import (
	"context"
	"testing"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	projectv1alpha1 "github.com/EvannDev/provider-harbor/apis/project/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/test/e2e"
)

const (
	testNamespace  = "crossplane-system"
	defaultTimeout = 2 * time.Minute
)

// TestProjectCRUD creates a Harbor Project CR, waits for Ready, verifies the
// project exists in Harbor with the expected name, updates the storage limit,
// and verifies the update is reflected.
func TestProjectCRUD(t *testing.T) {
	t.Parallel()

	f := e2e.New(t)

	const (
		crName      = "e2e-project-crud"
		projectName = "e2e-project-crud"
	)

	proj := &projectv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				"crossplane.io/external-name": projectName,
			},
		},
		Spec: projectv1alpha1.ProjectSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				ProviderConfigReference: &xpv1.ProviderConfigReference{
					Kind: "ClusterProviderConfig",
					Name: f.ProviderConfigName,
				},
			},
			ForProvider: projectv1alpha1.ProjectParameters{
				StorageLimit: ptr.To(int64(-1)),
			},
		},
	}

	f.CreateAndWaitForReady(t, proj, defaultTimeout)
	e2e.AssertReady(t, proj)
	e2e.AssertSynced(t, proj)
	e2e.AssertExternalName(t, proj, projectName)

	got, err := f.FetchProject(projectName)
	if err != nil {
		t.Fatalf("fetching project from Harbor: %v", err)
	}
	if got == nil {
		t.Fatalf("project %q not found in Harbor after Ready", projectName)
	}
	if got.Name != projectName {
		t.Errorf("Harbor project name = %q, want %q", got.Name, projectName)
	}

	// Re-fetch the CR to get the current resourceVersion before updating.
	if err := f.Kube.Get(context.Background(), kubeKey(testNamespace, crName), proj); err != nil {
		t.Fatalf("re-fetching project CR: %v", err)
	}

	// Update: set a 1 GiB storage limit.
	proj.Spec.ForProvider.StorageLimit = ptr.To(int64(1073741824))
	f.Update(t, proj)

	if err := f.WaitForReady(context.Background(), proj, defaultTimeout); err != nil {
		t.Fatalf("waiting for project to be Ready after update: %v\n  conditions: %s",
			err, e2e.SummariseConditions(proj))
	}

	updated, err := f.FetchProject(projectName)
	if err != nil {
		t.Fatalf("fetching updated project from Harbor: %v", err)
	}
	if updated == nil {
		t.Fatalf("project %q not found in Harbor after update", projectName)
	}
}
