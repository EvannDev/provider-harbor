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
	"strings"

	modelv2 "github.com/mittwald/goharbor-client/v5/apiv2/model"
	harborerrors "github.com/mittwald/goharbor-client/v5/apiv2/pkg/errors"
)

// FetchProject returns the Harbor project with the given name, or (nil, nil)
// if no such project exists. The name is the external-name annotation value,
// which the provider sets to match the Harbor project name.
func (f *Framework) FetchProject(name string) (*modelv2.Project, error) {
	proj, err := f.Harbor.GetProject(context.Background(), name)
	if isProjectNotFound(err) {
		return nil, nil //nolint:nilnil // intentional: 404 is the natural absence sentinel
	}
	if err != nil {
		return nil, err
	}

	return proj, nil
}

// FetchRobotAccountByID returns the Harbor robot account with the given ID,
// or (nil, nil) if no such robot exists. The provider stores the robot ID in
// the status and external-name annotation after creation.
func (f *Framework) FetchRobotAccountByID(id int64) (*modelv2.Robot, error) {
	robot, err := f.Harbor.GetRobotAccountByID(context.Background(), id)
	if isRobotNotFound(err) {
		return nil, nil //nolint:nilnil // intentional: 404 is the natural absence sentinel
	}
	if err != nil {
		return nil, err
	}

	return robot, nil
}

// FindRobotByName returns the Harbor robot account whose name matches name
// (with or without the "robot$" prefix), or (nil, nil) if no such robot
// exists. Useful when the robot ID is not yet known.
func (f *Framework) FindRobotByName(name string) (*modelv2.Robot, error) {
	// Harbor stores robots with the "robot$" prefix.
	fullName := name
	if !strings.HasPrefix(name, "robot$") {
		fullName = "robot$" + name
	}

	robot, err := f.Harbor.GetRobotAccountByName(context.Background(), fullName)
	if isRobotNotFound(err) {
		return nil, nil //nolint:nilnil // intentional: absence is the natural "not found" sentinel
	}
	if err != nil {
		return nil, err
	}

	return robot, nil
}

func isProjectNotFound(err error) bool {
	if err == nil {
		return false
	}
	var notFound *harborerrors.ErrProjectNotFound
	var mismatch *harborerrors.ErrProjectMismatch
	return errors.As(err, &notFound) || errors.As(err, &mismatch)
}

func isRobotNotFound(err error) bool {
	if err == nil {
		return false
	}
	var unknown *harborerrors.ErrRobotAccountUnknownResource
	if errors.As(err, &unknown) {
		return true
	}

	return strings.Contains(err.Error(), "][404]")
}
