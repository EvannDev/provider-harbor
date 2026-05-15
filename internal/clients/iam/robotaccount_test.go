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

package iam_test

import (
	"context"
	"errors"
	"testing"

	apiv2 "github.com/mittwald/goharbor-client/v5/apiv2"
	modelv2 "github.com/mittwald/goharbor-client/v5/apiv2/model"
	harborerrors "github.com/mittwald/goharbor-client/v5/apiv2/pkg/errors"

	v1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/clients/iam"
	"github.com/EvannDev/provider-harbor/internal/fake"
)

const (
	// robotMybot is the full Harbor robot name used in tests.
	robotMybot = "robot$mybot"
	// actionPull is the Harbor pull access action.
	actionPull = "pull"
	// nsMyProject is the test project namespace.
	nsMyProject = "myproject"
	// kindSystem is the system-level robot kind.
	kindSystem = "system"
	// resourceArtifact is the artifact resource type.
	resourceArtifact = "artifact"
	// botName is the short robot account name.
	botName = "mybot"
	// descMyBot is the first test robot description.
	descMyBot = "desc"
	// descMyBot2 is the second test robot description.
	descMyBot2 = "desc2"
	// descUpdated is the description used in update tests.
	descUpdated = "updated"
	// descPatched is the description used in patch tests.
	descPatched = "patched"
	// descWithPerms is the description used in permission tests.
	descWithPerms = "withperms"
	// nsTargetProject is the target project namespace used in tests.
	nsTargetProject = "targetproject"
	// robotFullName is the full robot name including project prefix.
	robotFullName = "robot$myproject+mybot"
	// resourceRepository is the repository resource type.
	resourceRepository = "repository"
	// kindProjectStr is the project-level robot kind.
	kindProjectStr = "project"
)

// TestNewRobotAccountsClient verifies that NewRobotAccountsClient returns
// the provided apiv2.Client as a RobotAccountsClient.
func TestNewRobotAccountsClient(t *testing.T) {
	t.Parallel()

	t.Run("returns client as RobotAccountsClient", func(t *testing.T) {
		t.Parallel()

		// apiv2.Client is a large interface; pass a nil value to confirm
		// the function returns exactly what it receives without wrapping.
		var nilClient apiv2.Client

		result := iam.NewRobotAccountsClient(nilClient)

		if result != nilClient {
			t.Errorf(
				"NewRobotAccountsClient: got %v, want %v",
				result,
				nilClient,
			)
		}
	})
}

// TestGenerateRobotAccountObservation verifies that
// GenerateRobotAccountObservation correctly maps a Harbor Robot into a
// RobotAccountObservation, and returns a zero value for nil input.
func TestGenerateRobotAccountObservation(t *testing.T) {
	t.Parallel()

	type testCase struct {
		// name is the sub-test name.
		name string
		// inputRobot is the robot passed to the function.
		inputRobot *modelv2.Robot
		// wantNilFields indicates whether all pointer fields should be nil.
		wantNilFields bool
		// wantID is the expected observation ID.
		wantID *int64
		// wantFullName is the expected observation FullName.
		wantFullName *string
		// wantExpiresAt is the expected observation ExpiresAt.
		wantExpiresAt *int64
	}

	cases := []testCase{
		{
			name:          "nil robot returns zero value",
			inputRobot:    nil,
			wantNilFields: true,
		},
		{
			name: "robot with all fields returns mapped observation",
			inputRobot: &modelv2.Robot{
				ID:        42,
				Name:      robotMybot,
				ExpiresAt: 9999,
			},
			wantNilFields: false,
			wantID:        new(int64(42)),
			wantFullName:  new(robotMybot),
			wantExpiresAt: new(int64(9999)),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			obs := iam.GenerateRobotAccountObservation(tc.inputRobot)

			if tc.wantNilFields {
				if obs.ID != nil || obs.FullName != nil || obs.ExpiresAt != nil {
					t.Errorf(
						"GenerateRobotAccountObservation: "+
							"expected all nil fields, got %+v",
						obs,
					)
				}

				return
			}

			if obs.ID == nil || *obs.ID != *tc.wantID {
				t.Errorf(
					"GenerateRobotAccountObservation: ID = %v, want %v",
					obs.ID,
					tc.wantID,
				)
			}

			if obs.FullName == nil || *obs.FullName != *tc.wantFullName {
				t.Errorf(
					"GenerateRobotAccountObservation: "+
						"FullName = %v, want %v",
					obs.FullName,
					tc.wantFullName,
				)
			}

			if obs.ExpiresAt == nil || *obs.ExpiresAt != *tc.wantExpiresAt {
				t.Errorf(
					"GenerateRobotAccountObservation: "+
						"ExpiresAt = %v, want %v",
					obs.ExpiresAt,
					tc.wantExpiresAt,
				)
			}
		})
	}
}

// TestIsRobotAccountUpToDate verifies that IsRobotAccountUpToDate correctly
// detects drift across all compared fields and returns true when they match.
func TestIsRobotAccountUpToDate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		// name is the sub-test name.
		name string
		// params is the desired spec.
		params v1alpha1.RobotAccountParameters
		// robot is the observed Harbor robot.
		robot *modelv2.Robot
		// want is the expected return value.
		want bool
	}

	matchingAccess := []v1alpha1.RobotAccountAccess{
		{Resource: resourceRepository, Action: actionPull},
	}
	matchingRobotAccess := []*modelv2.Access{
		{Resource: resourceRepository, Action: actionPull},
	}
	matchingPerms := []v1alpha1.RobotAccountPermission{
		{Kind: kindProjectStr, Namespace: nsMyProject, Access: matchingAccess},
	}
	matchingRobotPerms := []*modelv2.RobotPermission{
		{Kind: kindProjectStr, Namespace: nsMyProject, Access: matchingRobotAccess},
	}

	cases := []testCase{
		{
			name: "description mismatch returns false",
			params: v1alpha1.RobotAccountParameters{
				Description: "old",
				Permissions: matchingPerms,
			},
			robot: &modelv2.Robot{
				Description: "new",
				Permissions: matchingRobotPerms,
			},
			want: false,
		},
		{
			name: "disable mismatch returns false",
			params: v1alpha1.RobotAccountParameters{
				Disable:     true,
				Permissions: matchingPerms,
			},
			robot: &modelv2.Robot{
				Disable:     false,
				Permissions: matchingRobotPerms,
			},
			want: false,
		},
		{
			name: "duration mismatch returns false when params.Duration set",
			params: v1alpha1.RobotAccountParameters{
				Duration:    new(int64(30)),
				Permissions: matchingPerms,
			},
			robot: &modelv2.Robot{
				Duration:    90,
				Permissions: matchingRobotPerms,
			},
			want: false,
		},
		{
			name: "nil duration skips duration check",
			params: v1alpha1.RobotAccountParameters{
				Duration:    nil,
				Permissions: matchingPerms,
			},
			robot: &modelv2.Robot{
				Duration:    90,
				Permissions: matchingRobotPerms,
			},
			want: true,
		},
		{
			name: "permission count mismatch returns false",
			params: v1alpha1.RobotAccountParameters{
				Permissions: []v1alpha1.RobotAccountPermission{},
			},
			robot: &modelv2.Robot{
				Permissions: matchingRobotPerms,
			},
			want: false,
		},
		{
			name: "permission kind mismatch returns false",
			params: v1alpha1.RobotAccountParameters{
				Permissions: []v1alpha1.RobotAccountPermission{
					{
						Kind:      kindSystem,
						Namespace: nsMyProject,
						Access:    matchingAccess,
					},
				},
			},
			robot: &modelv2.Robot{
				Permissions: matchingRobotPerms,
			},
			want: false,
		},
		{
			name: "permission namespace mismatch returns false",
			params: v1alpha1.RobotAccountParameters{
				Permissions: []v1alpha1.RobotAccountPermission{
					{
						Kind:      kindProjectStr,
						Namespace: "otherproject",
						Access:    matchingAccess,
					},
				},
			},
			robot: &modelv2.Robot{
				Permissions: matchingRobotPerms,
			},
			want: false,
		},
		{
			name: "access count mismatch returns false",
			params: v1alpha1.RobotAccountParameters{
				Permissions: []v1alpha1.RobotAccountPermission{
					{
						Kind:      kindProjectStr,
						Namespace: nsMyProject,
						Access:    []v1alpha1.RobotAccountAccess{},
					},
				},
			},
			robot: &modelv2.Robot{
				Permissions: matchingRobotPerms,
			},
			want: false,
		},
		{
			name: "access resource mismatch returns false",
			params: v1alpha1.RobotAccountParameters{
				Permissions: []v1alpha1.RobotAccountPermission{
					{
						Kind:      kindProjectStr,
						Namespace: nsMyProject,
						Access: []v1alpha1.RobotAccountAccess{
							{Resource: resourceArtifact, Action: actionPull},
						},
					},
				},
			},
			robot: &modelv2.Robot{
				Permissions: matchingRobotPerms,
			},
			want: false,
		},
		{
			name: "access action mismatch returns false",
			params: v1alpha1.RobotAccountParameters{
				Permissions: []v1alpha1.RobotAccountPermission{
					{
						Kind:      kindProjectStr,
						Namespace: nsMyProject,
						Access: []v1alpha1.RobotAccountAccess{
							{Resource: resourceRepository, Action: "push"},
						},
					},
				},
			},
			robot: &modelv2.Robot{
				Permissions: matchingRobotPerms,
			},
			want: false,
		},
		{
			name: "all fields match returns true",
			params: v1alpha1.RobotAccountParameters{
				Description: botName,
				Disable:     false,
				Duration:    new(int64(30)),
				Permissions: matchingPerms,
			},
			robot: &modelv2.Robot{
				Description: botName,
				Disable:     false,
				Duration:    30,
				Permissions: matchingRobotPerms,
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := iam.IsRobotAccountUpToDate(tc.params, tc.robot)

			if got != tc.want {
				t.Errorf(
					"IsRobotAccountUpToDate: got %v, want %v",
					got,
					tc.want,
				)
			}
		})
	}
}

// TestToRobotCreate verifies that ToRobotCreate produces a correctly
// populated RobotCreate model from the given name and parameters.
func TestToRobotCreate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		// name is the sub-test name.
		name string
		// robotName is the robot account name passed to the function.
		robotName string
		// params is the desired spec.
		params v1alpha1.RobotAccountParameters
		// wantDuration is the expected Duration field value.
		wantDuration int64
		// wantDescription is the expected Description field value.
		wantDescription string
		// wantLevel is the expected Level field value.
		wantLevel string
		// wantDisable is the expected Disable field value.
		wantDisable bool
	}

	cases := []testCase{
		{
			name:      "nil duration leaves Duration as zero",
			robotName: botName,
			params: v1alpha1.RobotAccountParameters{
				Description: descMyBot,
				Level:       kindProjectStr,
				Disable:     false,
				Duration:    nil,
				Permissions: []v1alpha1.RobotAccountPermission{},
			},
			wantDuration:    0,
			wantDescription: descMyBot,
			wantLevel:       kindProjectStr,
			wantDisable:     false,
		},
		{
			name:      "set duration is applied to request",
			robotName: "mybot2",
			params: v1alpha1.RobotAccountParameters{
				Description: descMyBot2,
				Level:       kindSystem,
				Disable:     true,
				Duration:    new(int64(60)),
				Permissions: []v1alpha1.RobotAccountPermission{},
			},
			wantDuration:    60,
			wantDescription: descMyBot2,
			wantLevel:       kindSystem,
			wantDisable:     true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := iam.ToRobotCreate(tc.robotName, tc.params)

			if req == nil {
				t.Fatal("ToRobotCreate: returned nil")
			}

			if req.Name != tc.robotName {
				t.Errorf(
					"ToRobotCreate: Name = %q, want %q",
					req.Name,
					tc.robotName,
				)
			}

			if req.Description != tc.wantDescription {
				t.Errorf(
					"ToRobotCreate: Description = %q, want %q",
					req.Description,
					tc.wantDescription,
				)
			}

			if req.Level != tc.wantLevel {
				t.Errorf(
					"ToRobotCreate: Level = %q, want %q",
					req.Level,
					tc.wantLevel,
				)
			}

			if req.Disable != tc.wantDisable {
				t.Errorf(
					"ToRobotCreate: Disable = %v, want %v",
					req.Disable,
					tc.wantDisable,
				)
			}

			if req.Duration != tc.wantDuration {
				t.Errorf(
					"ToRobotCreate: Duration = %v, want %v",
					req.Duration,
					tc.wantDuration,
				)
			}
		})
	}
}

// TestApplyRobotAccountParameters verifies that ApplyRobotAccountParameters
// applies all spec fields onto the target Robot object in-place.
func TestApplyRobotAccountParameters(t *testing.T) {
	t.Parallel()

	type testCase struct {
		// name is the sub-test name.
		name string
		// params is the desired spec.
		params v1alpha1.RobotAccountParameters
		// initialRobot is the Robot to mutate.
		initialRobot *modelv2.Robot
		// wantDescription is the expected description after applying.
		wantDescription string
		// wantDisable is the expected disable flag after applying.
		wantDisable bool
		// wantDuration is the expected duration after applying.
		wantDuration int64
	}

	cases := []testCase{
		{
			name: "nil duration does not overwrite robot duration",
			params: v1alpha1.RobotAccountParameters{
				Description: descUpdated,
				Disable:     true,
				Duration:    nil,
				Permissions: []v1alpha1.RobotAccountPermission{},
			},
			initialRobot: &modelv2.Robot{
				Duration: 99,
			},
			wantDescription: descUpdated,
			wantDisable:     true,
			wantDuration:    99,
		},
		{
			name: "set duration overwrites robot duration",
			params: v1alpha1.RobotAccountParameters{
				Description: descPatched,
				Disable:     false,
				Duration:    new(int64(14)),
				Permissions: []v1alpha1.RobotAccountPermission{},
			},
			initialRobot: &modelv2.Robot{
				Duration: 99,
			},
			wantDescription: descPatched,
			wantDisable:     false,
			wantDuration:    14,
		},
		{
			name: "permissions are applied",
			params: v1alpha1.RobotAccountParameters{
				Description: descWithPerms,
				Permissions: []v1alpha1.RobotAccountPermission{
					{
						Kind:      kindProjectStr,
						Namespace: "myns",
						Access: []v1alpha1.RobotAccountAccess{
							{Resource: resourceRepository, Action: actionPull},
						},
					},
				},
			},
			initialRobot:    &modelv2.Robot{},
			wantDescription: descWithPerms,
			wantDisable:     false,
			wantDuration:    0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			iam.ApplyRobotAccountParameters(tc.params, tc.initialRobot)

			if tc.initialRobot.Description != tc.wantDescription {
				t.Errorf(
					"ApplyRobotAccountParameters: "+
						"Description = %q, want %q",
					tc.initialRobot.Description,
					tc.wantDescription,
				)
			}

			if tc.initialRobot.Disable != tc.wantDisable {
				t.Errorf(
					"ApplyRobotAccountParameters: "+
						"Disable = %v, want %v",
					tc.initialRobot.Disable,
					tc.wantDisable,
				)
			}

			if tc.initialRobot.Duration != tc.wantDuration {
				t.Errorf(
					"ApplyRobotAccountParameters: "+
						"Duration = %v, want %v",
					tc.initialRobot.Duration,
					tc.wantDuration,
				)
			}
		})
	}
}

// checkHarborPermissionFields asserts that a converted Harbor permission
// matches the source CR permission's Kind, Namespace, and Access slice length.
func checkHarborPermissionFields(t *testing.T, idx int, harborPerm *modelv2.RobotPermission, perm v1alpha1.RobotAccountPermission) bool {
	t.Helper()

	if harborPerm.Kind != perm.Kind {
		t.Errorf(
			"ToHarborPermissions[%d]: Kind = %q, want %q",
			idx,
			harborPerm.Kind,
			perm.Kind,
		)
	}

	if harborPerm.Namespace != perm.Namespace {
		t.Errorf(
			"ToHarborPermissions[%d]: Namespace = %q, want %q",
			idx,
			harborPerm.Namespace,
			perm.Namespace,
		)
	}

	if len(harborPerm.Access) != len(perm.Access) {
		t.Errorf(
			"ToHarborPermissions[%d]: Access len = %d, want %d",
			idx,
			len(harborPerm.Access),
			len(perm.Access),
		)

		return false
	}

	return true
}

// checkHarborAccessFields asserts that each Access entry within a converted
// Harbor permission matches the corresponding source CR access entry.
func checkHarborAccessFields(t *testing.T, idx int, harborPerm *modelv2.RobotPermission, perm v1alpha1.RobotAccountPermission) {
	t.Helper()

	for jdx, acc := range perm.Access {
		harborAcc := harborPerm.Access[jdx]

		if harborAcc.Resource != acc.Resource {
			t.Errorf(
				"ToHarborPermissions[%d].Access[%d]: Resource = %q, want %q",
				idx,
				jdx,
				harborAcc.Resource,
				acc.Resource,
			)
		}

		if harborAcc.Action != acc.Action {
			t.Errorf(
				"ToHarborPermissions[%d].Access[%d]: Action = %q, want %q",
				idx,
				jdx,
				harborAcc.Action,
				acc.Action,
			)
		}
	}
}

// TestToHarborPermissions verifies that ToHarborPermissions converts
// CR permission slices to Harbor model permissions correctly.
func TestToHarborPermissions(t *testing.T) {
	t.Parallel()

	type testCase struct {
		// name is the sub-test name.
		name string
		// inputPerms is the list of CR permissions to convert.
		inputPerms []v1alpha1.RobotAccountPermission
		// wantCount is the expected number of converted permissions.
		wantCount int
	}

	cases := []testCase{
		{
			name:       "empty slice returns empty result",
			inputPerms: []v1alpha1.RobotAccountPermission{},
			wantCount:  0,
		},
		{
			name: "single permission with multiple accesses is mapped",
			inputPerms: []v1alpha1.RobotAccountPermission{
				{
					Kind:      kindProjectStr,
					Namespace: nsMyProject,
					Access: []v1alpha1.RobotAccountAccess{
						{Resource: resourceRepository, Action: actionPull},
						{Resource: resourceArtifact, Action: "read"},
					},
				},
			},
			wantCount: 1,
		},
		{
			name: "multiple permissions are all mapped",
			inputPerms: []v1alpha1.RobotAccountPermission{
				{
					Kind:      kindProjectStr,
					Namespace: "ns1",
					Access: []v1alpha1.RobotAccountAccess{
						{Resource: resourceRepository, Action: actionPull},
					},
				},
				{
					Kind:      kindSystem,
					Namespace: "*",
					Access: []v1alpha1.RobotAccountAccess{
						{Resource: "audit-log", Action: "list"},
					},
				},
			},
			wantCount: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := iam.ToHarborPermissions(tc.inputPerms)

			if len(result) != tc.wantCount {
				t.Fatalf(
					"ToHarborPermissions: len = %d, want %d",
					len(result),
					tc.wantCount,
				)
			}

			for idx, perm := range tc.inputPerms {
				harborPerm := result[idx]

				if !checkHarborPermissionFields(t, idx, harborPerm, perm) {
					continue
				}

				checkHarborAccessFields(t, idx, harborPerm, perm)
			}
		})
	}
}

// TestValidatePermissions verifies that ValidatePermissions returns nil
// for valid permissions and an error with the index for project
// permissions with an empty namespace.
func TestValidatePermissions(t *testing.T) {
	t.Parallel()

	type testCase struct {
		// name is the sub-test name.
		name string
		// inputPerms is the list of permissions to validate.
		inputPerms []v1alpha1.RobotAccountPermission
		// wantErr indicates whether an error is expected.
		wantErr bool
		// wantErrIndex is the index expected to appear in the error string.
		wantErrIndex int
	}

	cases := []testCase{
		{
			name:       "empty permissions returns nil",
			inputPerms: []v1alpha1.RobotAccountPermission{},
			wantErr:    false,
		},
		{
			name: "project kind with non-empty namespace returns nil",
			inputPerms: []v1alpha1.RobotAccountPermission{
				{Kind: kindProjectStr, Namespace: nsMyProject},
			},
			wantErr: false,
		},
		{
			name: "project kind with empty namespace returns error",
			inputPerms: []v1alpha1.RobotAccountPermission{
				{Kind: kindProjectStr, Namespace: "valid"},
				{Kind: kindProjectStr, Namespace: ""},
			},
			wantErr:      true,
			wantErrIndex: 1,
		},
		{
			name: "system kind with empty namespace returns nil",
			inputPerms: []v1alpha1.RobotAccountPermission{
				{Kind: kindSystem, Namespace: ""},
			},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := iam.ValidatePermissions(tc.inputPerms)

			if tc.wantErr && err == nil {
				t.Fatalf("ValidatePermissions: expected error, got nil")
			}

			if !tc.wantErr && err != nil {
				t.Fatalf(
					"ValidatePermissions: unexpected error: %v",
					err,
				)
			}
		})
	}
}

// TestProjectNamespace verifies that ProjectNamespace returns the first
// resolved project namespace, or an empty string when none exists.
func TestProjectNamespace(t *testing.T) {
	t.Parallel()

	type testCase struct {
		// name is the sub-test name.
		name string
		// inputPerms is the list of permissions to inspect.
		inputPerms []v1alpha1.RobotAccountPermission
		// wantNamespace is the expected return value.
		wantNamespace string
	}

	cases := []testCase{
		{
			name: "no project kind returns empty string",
			inputPerms: []v1alpha1.RobotAccountPermission{
				{Kind: kindSystem, Namespace: "some"},
			},
			wantNamespace: "",
		},
		{
			name: "project kind with empty namespace returns empty string",
			inputPerms: []v1alpha1.RobotAccountPermission{
				{Kind: kindProjectStr, Namespace: ""},
			},
			wantNamespace: "",
		},
		{
			name: "project kind with namespace returns namespace value",
			inputPerms: []v1alpha1.RobotAccountPermission{
				{Kind: kindSystem, Namespace: "sys"},
				{Kind: kindProjectStr, Namespace: nsTargetProject},
			},
			wantNamespace: nsTargetProject,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := iam.ProjectNamespace(tc.inputPerms)

			if got != tc.wantNamespace {
				t.Errorf(
					"ProjectNamespace: got %q, want %q",
					got,
					tc.wantNamespace,
				)
			}
		})
	}
}

// TestStripRobotPrefix verifies that StripRobotPrefix removes the
// "robot$" prefix and leaves strings without the prefix unchanged.
func TestStripRobotPrefix(t *testing.T) {
	t.Parallel()

	type testCase struct {
		// name is the sub-test name.
		name string
		// fullName is the input string.
		fullName string
		// wantResult is the expected stripped result.
		wantResult string
	}

	cases := []testCase{
		{
			name:       "removes robot$ prefix",
			fullName:   robotMybot,
			wantResult: botName,
		},
		{
			name:       "leaves string without prefix unchanged",
			fullName:   botName,
			wantResult: botName,
		},
		{
			name:       "leaves empty string unchanged",
			fullName:   "",
			wantResult: "",
		},
		{
			name:       "handles nested project prefix",
			fullName:   robotFullName,
			wantResult: nsMyProject + "+" + botName,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := iam.StripRobotPrefix(tc.fullName)

			if got != tc.wantResult {
				t.Errorf(
					"StripRobotPrefix(%q): got %q, want %q",
					tc.fullName,
					got,
					tc.wantResult,
				)
			}
		})
	}
}

// TestFindProjectRobotByName verifies that FindProjectRobotByName correctly
// finds a robot by exact name, by suffix, and propagates errors from the
// client.
func TestFindProjectRobotByName(t *testing.T) {
	t.Parallel()

	type testCase struct {
		// name is the sub-test name.
		name string
		// listFn is the mock ListProjectRobotsV1 implementation.
		listFn func(ctx context.Context, project string) ([]*modelv2.Robot, error)
		// searchProject is the project name to search in.
		searchProject string
		// searchName is the robot short name to look for.
		searchName string
		// wantErr indicates whether an error is expected.
		wantErr bool
		// wantNotFound indicates whether the error should be ErrRobotAccountUnknownResource.
		wantNotFound bool
		// wantRobotName is the expected robot name (full name).
		wantRobotName string
	}

	listError := errors.New("list failed")

	cases := []testCase{
		{
			name: "list error is propagated",
			listFn: func(_ context.Context, _ string) ([]*modelv2.Robot, error) {
				return nil, listError
			},
			searchProject: nsMyProject,
			searchName:    botName,
			wantErr:       true,
			wantNotFound:  false,
		},
		{
			name: "no robots returns ErrRobotAccountUnknownResource",
			listFn: func(_ context.Context, _ string) ([]*modelv2.Robot, error) {
				return []*modelv2.Robot{}, nil
			},
			searchProject: nsMyProject,
			searchName:    botName,
			wantErr:       true,
			wantNotFound:  true,
		},
		{
			name: "robot found by exact stripped name",
			listFn: func(_ context.Context, _ string) ([]*modelv2.Robot, error) {
				return []*modelv2.Robot{
					{Name: robotMybot},
				}, nil
			},
			searchProject: nsMyProject,
			searchName:    botName,
			wantErr:       false,
			wantRobotName: robotMybot,
		},
		{
			name: "robot found by suffix match",
			listFn: func(_ context.Context, _ string) ([]*modelv2.Robot, error) {
				return []*modelv2.Robot{
					{Name: robotFullName},
				}, nil
			},
			searchProject: nsMyProject,
			searchName:    botName,
			wantErr:       false,
			wantRobotName: robotFullName,
		},
		{
			name: "robot not found despite having robots",
			listFn: func(_ context.Context, _ string) ([]*modelv2.Robot, error) {
				return []*modelv2.Robot{
					{Name: "robot$otherbot"},
					{Name: "robot$anotherbot"},
				}, nil
			},
			searchProject: nsMyProject,
			searchName:    botName,
			wantErr:       true,
			wantNotFound:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mockClient := &fake.MockRobotAccountsClient{
				ListProjectRobotsV1Fn: tc.listFn,
			}
			ctx := context.Background()

			robot, err := iam.FindProjectRobotByName(
				ctx,
				mockClient,
				tc.searchProject,
				tc.searchName,
			)

			if tc.wantErr && err == nil {
				t.Fatalf(
					"FindProjectRobotByName: expected error, got nil",
				)
			}

			if !tc.wantErr && err != nil {
				t.Fatalf(
					"FindProjectRobotByName: unexpected error: %v",
					err,
				)
			}

			if tc.wantNotFound {
				var unknownErr *harborerrors.ErrRobotAccountUnknownResource

				if !errors.As(err, &unknownErr) {
					t.Errorf(
						"FindProjectRobotByName: "+
							"expected ErrRobotAccountUnknownResource, got %T: %v",
						err,
						err,
					)
				}
			}

			if !tc.wantErr && robot != nil && robot.Name != tc.wantRobotName {
				t.Errorf(
					"FindProjectRobotByName: "+
						"robot.Name = %q, want %q",
					robot.Name,
					tc.wantRobotName,
				)
			}
		})
	}
}

// TestIsNotFound verifies that IsNotFound correctly identifies not-found
// errors from both typed errors and status-code strings.
func TestIsNotFound(t *testing.T) {
	t.Parallel()

	type testCase struct {
		// name is the sub-test name.
		name string
		// inputErr is the error to check.
		inputErr error
		// want is the expected return value.
		want bool
	}

	cases := []testCase{
		{
			name:     "ErrRobotAccountUnknownResource pointer returns true",
			inputErr: &harborerrors.ErrRobotAccountUnknownResource{},
			want:     true,
		},
		{
			name:     "error string containing ][404] returns true",
			inputErr: errors.New("something ][404] bad"),
			want:     true,
		},
		{
			name:     "unrelated error returns false",
			inputErr: errors.New("some other error"),
			want:     false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := iam.IsNotFound(tc.inputErr)

			if got != tc.want {
				t.Errorf(
					"IsNotFound: got %v, want %v",
					got,
					tc.want,
				)
			}
		})
	}
}
