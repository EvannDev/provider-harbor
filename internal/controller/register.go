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

// Package controller registers all Harbor managed resource controllers.
package controller

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/EvannDev/provider-harbor/internal/controller/config"
	"github.com/EvannDev/provider-harbor/internal/controller/iam/group"
	robotaccountcontroller "github.com/EvannDev/provider-harbor/internal/controller/iam/robotaccount"
	projectcontroller "github.com/EvannDev/provider-harbor/internal/controller/project/project"
)

// SetupGated creates all Harbor controllers with safe-start support and
// adds them to the supplied manager.
func SetupGated(mgr ctrl.Manager, opts controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		config.Setup,
		group.SetupGated,
		projectcontroller.SetupGated,
		robotaccountcontroller.SetupGated,
	} {
		err := setup(mgr, opts)
		if err != nil {
			return err
		}
	}

	return nil
}
