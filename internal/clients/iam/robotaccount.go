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

// Package iam provides Harbor IAM client utilities and helpers.
package iam

import (
	"context"
	"strings"

	apiv2 "github.com/mittwald/goharbor-client/v5/apiv2"
	modelv2 "github.com/mittwald/goharbor-client/v5/apiv2/model"
	harborerrors "github.com/mittwald/goharbor-client/v5/apiv2/pkg/errors"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"

	v1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
)

// kindProject is the Harbor permission kind for project-scoped robots.
const kindProject = "project"

// RobotAccountsClient defines the robot account operations needed by the
// controller.
type RobotAccountsClient interface {
	// GetRobotAccountByID fetches a robot account by its numeric ID.
	GetRobotAccountByID(ctx context.Context, id int64) (*modelv2.Robot, error)
	// GetRobotAccountByName fetches a system-level robot account by name.
	GetRobotAccountByName(ctx context.Context, robotName string) (*modelv2.Robot, error)
	// ListProjectRobotsV1 lists all robot accounts for a given project.
	ListProjectRobotsV1(ctx context.Context, projectName string) ([]*modelv2.Robot, error)
	// NewRobotAccount creates a new robot account.
	NewRobotAccount(ctx context.Context, robot *modelv2.RobotCreate) (*modelv2.RobotCreated, error)
	// UpdateRobotAccount applies changes to an existing robot account.
	UpdateRobotAccount(ctx context.Context, robot *modelv2.Robot) error
	// DeleteRobotAccountByID removes a robot account by its numeric ID.
	DeleteRobotAccountByID(ctx context.Context, id int64) error
}

// NewRobotAccountsClient returns the apiv2.Client as a RobotAccountsClient.
// apiv2.Client implements all RobotAccountsClient methods directly.
func NewRobotAccountsClient(cl apiv2.Client) RobotAccountsClient {
	return cl
}

// GenerateRobotAccountObservation converts a Harbor Robot into a
// RobotAccountObservation.
func GenerateRobotAccountObservation(robot *modelv2.Robot) v1alpha1.RobotAccountObservation {
	if robot == nil {
		return v1alpha1.RobotAccountObservation{}
	}

	id := robot.ID
	expiresAt := robot.ExpiresAt

	return v1alpha1.RobotAccountObservation{
		ID:        &id,
		FullName:  &robot.Name,
		ExpiresAt: &expiresAt,
	}
}

// IsRobotAccountUpToDate returns true if the observed Harbor robot matches
// the desired spec.
func IsRobotAccountUpToDate(params v1alpha1.RobotAccountParameters, robot *modelv2.Robot) bool {
	if params.Description != robot.Description || params.Disable != robot.Disable {
		return false
	}

	if params.Duration != nil && *params.Duration != robot.Duration {
		return false
	}

	return arePermissionsUpToDate(params.Permissions, robot.Permissions)
}

// arePermissionsUpToDate checks if the spec permissions match the observed
// permissions.
func arePermissionsUpToDate(perms []v1alpha1.RobotAccountPermission, robotPerms []*modelv2.RobotPermission) bool {
	if len(perms) != len(robotPerms) {
		return false
	}

	for idx, perm := range perms {
		if !isSinglePermissionUpToDate(perm, robotPerms[idx]) {
			return false
		}
	}

	return true
}

// isSinglePermissionUpToDate checks a single permission entry against its
// observed counterpart.
func isSinglePermissionUpToDate(perm v1alpha1.RobotAccountPermission, robotPerm *modelv2.RobotPermission) bool {
	if perm.Kind != robotPerm.Kind || perm.Namespace != robotPerm.Namespace {
		return false
	}

	if len(perm.Access) != len(robotPerm.Access) {
		return false
	}

	for idx, access := range perm.Access {
		if access.Resource != robotPerm.Access[idx].Resource || access.Action != robotPerm.Access[idx].Action {
			return false
		}
	}

	return true
}

// ToRobotCreate converts CR parameters into a Harbor RobotCreate request.
func ToRobotCreate(name string, params v1alpha1.RobotAccountParameters) *modelv2.RobotCreate {
	req := &modelv2.RobotCreate{
		Name:        name,
		Description: params.Description,
		Level:       params.Level,
		Disable:     params.Disable,
		Permissions: ToHarborPermissions(params.Permissions),
	}

	if params.Duration != nil {
		req.Duration = *params.Duration
	}

	return req
}

// ApplyRobotAccountParameters applies desired spec fields onto an existing
// Harbor Robot object.
func ApplyRobotAccountParameters(params v1alpha1.RobotAccountParameters, robot *modelv2.Robot) {
	robot.Description = params.Description
	robot.Disable = params.Disable
	robot.Permissions = ToHarborPermissions(params.Permissions)

	if params.Duration != nil {
		robot.Duration = *params.Duration
	}
}

// ToHarborPermissions converts CR permission list to Harbor model permissions.
func ToHarborPermissions(perms []v1alpha1.RobotAccountPermission) []*modelv2.RobotPermission {
	out := make([]*modelv2.RobotPermission, len(perms))

	for idx, perm := range perms {
		access := make([]*modelv2.Access, len(perm.Access))
		for jdx, acc := range perm.Access {
			access[jdx] = &modelv2.Access{
				Resource: acc.Resource,
				Action:   acc.Action,
			}
		}

		out[idx] = &modelv2.RobotPermission{
			Kind:      perm.Kind,
			Namespace: perm.Namespace,
			Access:    access,
		}
	}

	return out
}

// ValidatePermissions returns an error if any project-scoped permission has
// an empty namespace, meaning the Project reference has not been resolved yet.
func ValidatePermissions(perms []v1alpha1.RobotAccountPermission) error {
	for idx, perm := range perms {
		if perm.Kind == kindProject && perm.Namespace == "" {
			return errors.Errorf("permissions[%d]: project namespace is empty — "+
				"the referenced Project may not be ready yet; will retry", idx)
		}
	}

	return nil
}

// ProjectNamespace returns the first resolved project namespace from a
// permission list, or "" if none is available.
func ProjectNamespace(perms []v1alpha1.RobotAccountPermission) string {
	for _, perm := range perms {
		if perm.Kind == kindProject && perm.Namespace != "" {
			return perm.Namespace
		}
	}

	return ""
}

// StripRobotPrefix removes the "robot$" prefix Harbor adds to robot names.
func StripRobotPrefix(fullName string) string {
	return strings.TrimPrefix(fullName, "robot$")
}

// FindProjectRobotByName uses GET /projects/{project}/robots to find a
// project-scoped robot by its short name.
func FindProjectRobotByName(ctx context.Context, cl RobotAccountsClient, project, name string) (*modelv2.Robot, error) {
	robots, err := cl.ListProjectRobotsV1(ctx, project)
	if err != nil {
		return nil, err
	}

	suffix := "+" + name

	for _, robot := range robots {
		stripped := StripRobotPrefix(robot.Name)
		if strings.HasSuffix(stripped, suffix) || stripped == name {
			return robot, nil
		}
	}

	return nil, &harborerrors.ErrRobotAccountUnknownResource{}
}

// IsNotFound returns true if the error indicates the robot account does not
// exist.
func IsNotFound(err error) bool {
	var unknown *harborerrors.ErrRobotAccountUnknownResource
	if errors.As(err, &unknown) {
		return true
	}

	return strings.Contains(err.Error(), "][404]")
}
