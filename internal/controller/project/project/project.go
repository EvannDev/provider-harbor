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

// Package project implements the Harbor Project managed resource controller.
package project

import (
	"context"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	projectv1alpha1 "github.com/EvannDev/provider-harbor/apis/project/v1alpha1"
	apisv1alpha1 "github.com/EvannDev/provider-harbor/apis/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/clients/common"
	projectclient "github.com/EvannDev/provider-harbor/internal/clients/project"
)

// Error message constants used throughout the controller.
const (
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errNewClient    = "cannot create Harbor client"
	errObserve      = "cannot observe Project"
	errCreate       = "cannot create Project"
	errUpdate       = "cannot update Project"
	errDelete       = "cannot delete Project"
)

// SetupGated adds a controller with safe-start support.
func SetupGated(mgr ctrl.Manager, opts controller.Options) error {
	opts.Gate.Register(func() {
		err := Setup(mgr, opts)
		if err != nil {
			panic(errors.Wrap(err, "cannot setup Project controller"))
		}
	}, projectv1alpha1.ProjectGroupVersionKind)

	return nil
}

// Setup adds a controller that reconciles Project managed resources.
func Setup(mgr ctrl.Manager, opts controller.Options) error {
	name := managed.ControllerName(projectv1alpha1.ProjectGroupKind)

	reconcilerOpts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*projectv1alpha1.Project](&connector{
			kube:         mgr.GetClient(),
			usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newServiceFn: projectclient.NewProjectsClient,
		}),
		managed.WithLogger(opts.Logger.WithValues("controller", name)),
		managed.WithPollInterval(opts.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	}

	if opts.Features.Enabled(feature.EnableBetaManagementPolicies) {
		reconcilerOpts = append(reconcilerOpts, managed.WithManagementPolicies())
	}

	if opts.Features.Enabled(feature.EnableAlphaChangeLogs) {
		reconcilerOpts = append(reconcilerOpts, managed.WithChangeLogger(opts.ChangeLogOptions.ChangeLogger))
	}

	if opts.MetricOptions != nil {
		reconcilerOpts = append(reconcilerOpts, managed.WithMetricRecorder(opts.MetricOptions.MRMetrics))
	}

	if opts.MetricOptions != nil && opts.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), opts.Logger, opts.MetricOptions.MRStateMetrics,
			&projectv1alpha1.ProjectList{}, opts.MetricOptions.PollStateMetricInterval,
		)

		err := mgr.Add(stateMetricsRecorder)
		if err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for ProjectList")
		}
	}

	reconciler := managed.NewReconciler(mgr, resource.ManagedKind(projectv1alpha1.ProjectGroupVersionKind), reconcilerOpts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(opts.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&projectv1alpha1.Project{}).
		Complete(ratelimiter.NewReconciler(name, reconciler, opts.GlobalRateLimiter))
}

// connector creates a Harbor API client from ProviderConfig credentials.
type connector struct {
	kube         client.Client
	usage        *resource.ProviderConfigUsageTracker
	newServiceFn func(common.Config) projectclient.ProjectsClient
}

// Connect implements managed.TypedExternalConnecter.
func (c *connector) Connect(ctx context.Context, proj *projectv1alpha1.Project) (managed.TypedExternalClient[*projectv1alpha1.Project], error) {
	err := c.usage.Track(ctx, proj)
	if err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	cfg, err := common.GetConfig(ctx, c.kube, proj)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{client: c.newServiceFn(*cfg)}, nil
}

// external implements Observe/Create/Update/Delete against the Harbor API.
type external struct {
	client projectclient.ProjectsClient
}

// projectName returns the external name, falling back to the CR name.
func projectName(proj *projectv1alpha1.Project) string {
	if externalName := meta.GetExternalName(proj); externalName != "" {
		return externalName
	}

	return proj.GetName()
}

// Observe checks whether the project exists and whether it is up-to-date.
func (e *external) Observe(ctx context.Context, proj *projectv1alpha1.Project) (managed.ExternalObservation, error) {
	name := projectName(proj)

	observed, err := e.client.GetProject(ctx, name)
	if err != nil {
		if isNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}

		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	proj.Status.AtProvider = projectclient.GenerateProjectObservation(observed)
	meta.SetExternalName(proj, observed.Name)
	proj.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: projectclient.IsProjectUpToDate(proj.Spec.ForProvider, observed),
	}, nil
}

// Create creates the project in Harbor.
func (e *external) Create(ctx context.Context, proj *projectv1alpha1.Project) (managed.ExternalCreation, error) {
	proj.Status.SetConditions(xpv1.Creating())

	name := projectName(proj)
	req := projectclient.ToProjectReq(name, proj.Spec.ForProvider)

	err := e.client.NewProject(ctx, req)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	meta.SetExternalName(proj, name)

	return managed.ExternalCreation{}, nil
}

// Update reconciles the project's state with the desired spec.
func (e *external) Update(ctx context.Context, proj *projectv1alpha1.Project) (managed.ExternalUpdate, error) {
	name := projectName(proj)
	req := projectclient.ToProjectReq(name, proj.Spec.ForProvider)

	err := e.client.UpdateProject(ctx, name, req)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	return managed.ExternalUpdate{}, nil
}

// Delete removes the project from Harbor.
func (e *external) Delete(ctx context.Context, proj *projectv1alpha1.Project) (managed.ExternalDelete, error) {
	proj.Status.SetConditions(xpv1.Deleting())

	err := e.client.DeleteProject(ctx, projectName(proj))
	if err != nil {
		if isNotFound(err) {
			return managed.ExternalDelete{}, nil
		}

		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	return managed.ExternalDelete{}, nil
}

// Disconnect is a no-op for stateless Harbor clients.
func (e *external) Disconnect(_ context.Context) error { return nil }

// isNotFound reports whether the error is a Harbor project-not-found response.
func isNotFound(err error) bool {
	return projectclient.IsProjectNotFound(err)
}
