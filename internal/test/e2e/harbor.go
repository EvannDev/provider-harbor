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

package e2e

import (
	"context"
	"errors"
	"net/http"
	"strings"

	goopenapiruntime "github.com/go-openapi/runtime"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/client/project"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/client/robot"
	"github.com/goharbor/go-client/pkg/sdk/v2.0/models"
)

// FetchProject returns the Harbor project with the given name, or (nil, nil)
// if no such project exists.
func (f *Framework) FetchProject(name string) (*models.Project, error) {
	resp, err := f.Harbor.Project.GetProject(context.Background(), &project.GetProjectParams{ProjectNameOrID: name})
	if isProjectNotFound(err) {
		return nil, nil //nolint:nilnil // intentional: 404 is the natural absence sentinel
	}

	if err != nil {
		return nil, err
	}

	return resp.Payload, nil
}

// FetchRobotAccountByID returns the Harbor robot account with the given ID,
// or (nil, nil) if no such robot exists.
func (f *Framework) FetchRobotAccountByID(id int64) (*models.Robot, error) {
	resp, err := f.Harbor.Robot.GetRobotByID(context.Background(), &robot.GetRobotByIDParams{RobotID: id})
	if isRobotNotFound(err) {
		return nil, nil //nolint:nilnil // intentional: 404 is the natural absence sentinel
	}

	if err != nil {
		return nil, err
	}

	return resp.Payload, nil
}

// FindRobotByName returns the Harbor robot account whose name matches name
// (with or without the "robot$" prefix), or (nil, nil) if none is found.
func (f *Framework) FindRobotByName(name string) (*models.Robot, error) {
	fullName := name
	if !strings.HasPrefix(name, "robot$") {
		fullName = "robot$" + name
	}

	q := "level=system"

	resp, err := f.Harbor.Robot.ListRobot(context.Background(), &robot.ListRobotParams{Q: &q})
	if err != nil {
		return nil, err
	}

	for _, r := range resp.Payload {
		if r.Name == fullName {
			return r, nil
		}
	}

	return nil, nil //nolint:nilnil // intentional: absence is the natural "not found" sentinel
}

// isProjectNotFound reports whether err is a Harbor 404 for a project.
// GetProject has no typed NotFound response — unhandled codes become APIError.
func isProjectNotFound(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *goopenapiruntime.APIError
	if errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound {
		return true
	}

	return strings.Contains(err.Error(), "][404]")
}

// isRobotNotFound reports whether err is a Harbor 404 for a robot account.
func isRobotNotFound(err error) bool {
	if err == nil {
		return false
	}

	var notFound *robot.GetRobotByIDNotFound
	if errors.As(err, &notFound) {
		return true
	}

	var apiErr *goopenapiruntime.APIError
	if errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound {
		return true
	}

	return strings.Contains(err.Error(), "][404]")
}
