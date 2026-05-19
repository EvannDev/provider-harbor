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
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	iamv1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
	projectv1alpha1 "github.com/EvannDev/provider-harbor/apis/project/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/test/e2e"
)

const (
	testNamespace  = "crossplane-system"
	defaultTimeout = 2 * time.Minute
)

// TestSystemRobotAccountCRUD creates a system-level Harbor robot account CR,
// waits for Ready, verifies the robot exists in Harbor with the expected name
// and that the connection secret contains the token.
func TestSystemRobotAccountCRUD(t *testing.T) {
	t.Parallel()

	f := e2e.New(t)

	const (
		crName     = "e2e-system-robot-crud"
		robotName  = "e2e-system-robot-crud"
		secretName = "e2e-system-robot-crud-secret"
	)

	duration := int64(-1) // never expires
	robot := &iamv1alpha1.RobotAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				"crossplane.io/external-name": robotName,
			},
		},
		Spec: iamv1alpha1.RobotAccountSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				ProviderConfigReference: &xpv1.ProviderConfigReference{
					Kind: "ClusterProviderConfig",
					Name: f.ProviderConfigName,
				},
				WriteConnectionSecretToReference: &xpv1.LocalSecretReference{
					Name: secretName,
				},
			},
			ForProvider: iamv1alpha1.RobotAccountParameters{
				Level:       "system",
				Description: "E2E-managed system robot",
				Duration:    &duration,
				Permissions: []iamv1alpha1.RobotAccountPermission{
					{
						Kind:      "project",
						Namespace: "*",
						Access: []iamv1alpha1.RobotAccountAccess{
							{Resource: "repository", Action: "pull"},
						},
					},
				},
			},
		},
	}

	f.CreateAndWaitForReady(t, robot, defaultTimeout)
	e2e.AssertReady(t, robot)
	e2e.AssertSynced(t, robot)

	// The provider stores the Harbor robot ID in the external-name annotation.
	externalName := meta.GetExternalName(robot)
	if externalName == "" {
		t.Fatalf("expected external-name to be populated after Ready")
	}

	// Verify the robot exists in Harbor via the status ID.
	if robot.Status.AtProvider.ID == nil {
		t.Fatalf("expected status.atProvider.id to be populated after Ready")
	}

	got, err := f.FetchRobotAccountByID(*robot.Status.AtProvider.ID)
	if err != nil {
		t.Fatalf("fetching robot account from Harbor: %v", err)
	}
	if got == nil {
		t.Fatalf("robot account %q (id=%d) not found in Harbor after Ready",
			robotName, *robot.Status.AtProvider.ID)
	}
	if got.Description != "E2E-managed system robot" {
		t.Errorf("robot description = %q, want %q", got.Description, "E2E-managed system robot")
	}

	// Verify the connection secret was created.
	secret := &corev1.Secret{}
	if err := f.Kube.Get(context.Background(), kubeKey(testNamespace, secretName), secret); err != nil {
		t.Errorf("connection secret %s/%s not found: %v", testNamespace, secretName, err)
	}
}

// TestProjectRobotAccountCRUD creates a Project CR, then a project-scoped
// robot account that references it, waits for both to be Ready, and verifies
// the robot exists in Harbor under the correct project.
func TestProjectRobotAccountCRUD(t *testing.T) {
	t.Parallel()

	f := e2e.New(t)

	const (
		projectCRName = "e2e-iam-project"
		projectName   = "e2e-iam-project"
		robotCRName   = "e2e-project-robot-crud"
		robotName     = "e2e-project-robot-crud"
		secretName    = "e2e-project-robot-crud-secret"
	)

	// Create a project for the robot to be scoped to.
	proj := &projectv1alpha1.Project{
		ObjectMeta: metav1.ObjectMeta{
			Name:      projectCRName,
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
				StorageLimit: new(int64(-1)),
			},
		},
	}

	f.CreateAndWaitForReady(t, proj, defaultTimeout)

	duration := int64(-1)
	robot := &iamv1alpha1.RobotAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      robotCRName,
			Namespace: testNamespace,
			Annotations: map[string]string{
				"crossplane.io/external-name": robotName,
			},
		},
		Spec: iamv1alpha1.RobotAccountSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				ProviderConfigReference: &xpv1.ProviderConfigReference{
					Kind: "ClusterProviderConfig",
					Name: f.ProviderConfigName,
				},
				WriteConnectionSecretToReference: &xpv1.LocalSecretReference{
					Name: secretName,
				},
			},
			ForProvider: iamv1alpha1.RobotAccountParameters{
				Level:       "project",
				Description: "E2E-managed project robot",
				Duration:    &duration,
				Permissions: []iamv1alpha1.RobotAccountPermission{
					{
						Kind:      "project",
						Namespace: projectName,
						Access: []iamv1alpha1.RobotAccountAccess{
							{Resource: "repository", Action: "pull"},
							{Resource: "repository", Action: "push"},
						},
					},
				},
			},
		},
	}

	f.CreateAndWaitForReady(t, robot, defaultTimeout)
	e2e.AssertReady(t, robot)
	e2e.AssertSynced(t, robot)

	if robot.Status.AtProvider.ID == nil {
		t.Fatalf("expected status.atProvider.id to be populated after Ready")
	}

	got, err := f.FetchRobotAccountByID(*robot.Status.AtProvider.ID)
	if err != nil {
		t.Fatalf("fetching robot account from Harbor: %v", err)
	}
	if got == nil {
		t.Fatalf("robot account %q not found in Harbor after Ready", robotName)
	}
	if got.Description != "E2E-managed project robot" {
		t.Errorf("robot description = %q, want %q", got.Description, "E2E-managed project robot")
	}
}
