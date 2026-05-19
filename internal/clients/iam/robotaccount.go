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
	"net/http"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	goopenapiruntime "github.com/go-openapi/runtime"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/client/robot"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/models"

	v1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/clients/common"
)

// kindProject is the Harbor permission kind for project-scoped robots.
const kindProject = "project"

// ErrRobotNotFound is returned when the robot account does not exist.
var ErrRobotNotFound = errors.New("robot account not found")

// RobotAccountsClient defines the robot account operations needed by the
// controller.
type RobotAccountsClient interface {
	// GetRobotAccountByID fetches a robot account by its numeric ID.
	GetRobotAccountByID(ctx context.Context, id int64) (*models.Robot, error)
	// GetRobotAccountByName fetches a system-level robot account by name.
	GetRobotAccountByName(ctx context.Context, robotName string) (*models.Robot, error)
	// ListProjectRobotsV1 lists all robot accounts for a given project.
	ListProjectRobotsV1(ctx context.Context, projectName string) ([]*models.Robot, error)
	// NewRobotAccount creates a new robot account.
	NewRobotAccount(ctx context.Context, robot *models.RobotCreate) (*models.RobotCreated, error)
	// UpdateRobotAccount applies changes to an existing robot account by ID.
	UpdateRobotAccount(ctx context.Context, id int64, robotModel *models.Robot) error
	// DeleteRobotAccountByID removes a robot account by its numeric ID.
	DeleteRobotAccountByID(ctx context.Context, id int64) error
}

// robotAccountsClient wraps the Harbor v2 robot API.
type robotAccountsClient struct {
	api *robot.Client
}

// NewRobotAccountsClient returns a RobotAccountsClient backed by the
// official Harbor SDK.
func NewRobotAccountsClient(cfg common.Config) RobotAccountsClient {
	return &robotAccountsClient{api: common.NewHarborAPI(cfg).Robot}
}

// GetRobotAccountByID fetches a robot account by its numeric ID.
func (c *robotAccountsClient) GetRobotAccountByID(ctx context.Context, id int64) (*models.Robot, error) {
	resp, err := c.api.GetRobotByID(ctx, &robot.GetRobotByIDParams{RobotID: id})
	if err != nil {
		var notFound *robot.GetRobotByIDNotFound
		if errors.As(err, &notFound) {
			return nil, ErrRobotNotFound
		}

		return nil, errors.Wrap(err, "cannot get robot account by ID")
	}

	return resp.Payload, nil
}

// GetRobotAccountByName fetches a system-level robot account by name.
// Harbor system robot names use the prefix "robot$", so if the provided name
// does not already include it, both forms are checked.
func (c *robotAccountsClient) GetRobotAccountByName(ctx context.Context, robotName string) (*models.Robot, error) {
	q := "level=system"

	resp, err := c.api.ListRobot(ctx, &robot.ListRobotParams{Q: &q})
	if err != nil {
		return nil, errors.Wrap(err, "cannot list robot accounts")
	}

	suffix := "+" + robotName
	full := "robot$" + robotName

	for _, r := range resp.Payload {
		stripped := StripRobotPrefix(r.Name)
		if r.Name == full || stripped == robotName || strings.HasSuffix(stripped, suffix) {
			return r, nil
		}
	}

	return nil, ErrRobotNotFound
}

// ListProjectRobotsV1 lists all robot accounts for a given project.
func (c *robotAccountsClient) ListProjectRobotsV1(ctx context.Context, projectName string) ([]*models.Robot, error) {
	q := "level=project"

	resp, err := c.api.ListRobot(ctx, &robot.ListRobotParams{Q: &q})
	if err != nil {
		return nil, errors.Wrap(err, "cannot list project robot accounts")
	}

	var filtered []*models.Robot

	for _, r := range resp.Payload {
		for _, perm := range r.Permissions {
			if perm.Kind == kindProject && perm.Namespace == projectName {
				filtered = append(filtered, r)

				break
			}
		}
	}

	return filtered, nil
}

// NewRobotAccount creates a new robot account.
func (c *robotAccountsClient) NewRobotAccount(ctx context.Context, robotCreate *models.RobotCreate) (*models.RobotCreated, error) {
	resp, err := c.api.CreateRobot(ctx, &robot.CreateRobotParams{Robot: robotCreate})
	if err != nil {
		return nil, errors.Wrap(err, "cannot create robot account")
	}

	return resp.Payload, nil
}

// UpdateRobotAccount applies changes to an existing robot account by ID.
func (c *robotAccountsClient) UpdateRobotAccount(ctx context.Context, id int64, robotModel *models.Robot) error {
	_, err := c.api.UpdateRobot(ctx, &robot.UpdateRobotParams{RobotID: id, Robot: robotModel})

	return errors.Wrap(err, "cannot update robot account")
}

// DeleteRobotAccountByID removes a robot account by its numeric ID.
func (c *robotAccountsClient) DeleteRobotAccountByID(ctx context.Context, id int64) error {
	_, err := c.api.DeleteRobot(ctx, &robot.DeleteRobotParams{RobotID: id})
	if err != nil {
		var notFound *robot.DeleteRobotNotFound
		if errors.As(err, &notFound) {
			return ErrRobotNotFound
		}

		var apiErr *goopenapiruntime.APIError
		if errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound {
			return ErrRobotNotFound
		}

		return errors.Wrap(err, "cannot delete robot account")
	}

	return nil
}

// IsNotFound returns true if the error indicates the robot account does not
// exist.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrRobotNotFound)
}

// GenerateRobotAccountObservation converts a Harbor Robot into a
// RobotAccountObservation.
func GenerateRobotAccountObservation(robotModel *models.Robot) v1alpha1.RobotAccountObservation {
	if robotModel == nil {
		return v1alpha1.RobotAccountObservation{}
	}

	id := robotModel.ID
	expiresAt := robotModel.ExpiresAt

	return v1alpha1.RobotAccountObservation{
		ID:        &id,
		FullName:  &robotModel.Name,
		ExpiresAt: &expiresAt,
	}
}

// IsRobotAccountUpToDate returns true if the observed Harbor robot matches
// the desired spec.
func IsRobotAccountUpToDate(params v1alpha1.RobotAccountParameters, robotModel *models.Robot) bool {
	if params.Description != robotModel.Description || params.Disable != robotModel.Disable {
		return false
	}

	if params.Duration != nil && robotModel.Duration != nil && *params.Duration != *robotModel.Duration {
		return false
	}

	return arePermissionsUpToDate(params.Permissions, robotModel.Permissions)
}

// arePermissionsUpToDate checks if the spec permissions match the observed
// permissions.
func arePermissionsUpToDate(perms []v1alpha1.RobotAccountPermission, robotPerms []*models.RobotPermission) bool {
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
func isSinglePermissionUpToDate(perm v1alpha1.RobotAccountPermission, robotPerm *models.RobotPermission) bool {
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
func ToRobotCreate(name string, params v1alpha1.RobotAccountParameters) *models.RobotCreate {
	req := &models.RobotCreate{
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
func ApplyRobotAccountParameters(params v1alpha1.RobotAccountParameters, robotModel *models.Robot) {
	robotModel.Description = params.Description
	robotModel.Disable = params.Disable
	robotModel.Permissions = ToHarborPermissions(params.Permissions)

	if params.Duration != nil {
		robotModel.Duration = params.Duration
	}
}

// ToHarborPermissions converts CR permission list to Harbor model permissions.
func ToHarborPermissions(perms []v1alpha1.RobotAccountPermission) []*models.RobotPermission {
	out := make([]*models.RobotPermission, len(perms))

	for idx, perm := range perms {
		access := make([]*models.Access, len(perm.Access))
		for jdx, acc := range perm.Access {
			access[jdx] = &models.Access{
				Resource: acc.Resource,
				Action:   acc.Action,
			}
		}

		out[idx] = &models.RobotPermission{
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

// FindProjectRobotByName uses ListProjectRobotsV1 to find a project-scoped
// robot by its short name.
func FindProjectRobotByName(ctx context.Context, cl RobotAccountsClient, projectName, name string) (*models.Robot, error) {
	robots, err := cl.ListProjectRobotsV1(ctx, projectName)
	if err != nil {
		return nil, err
	}

	suffix := "+" + name

	for _, r := range robots {
		stripped := StripRobotPrefix(r.Name)
		if strings.HasSuffix(stripped, suffix) || stripped == name {
			return r, nil
		}
	}

	return nil, ErrRobotNotFound
}
