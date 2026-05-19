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

// Package robotaccount reconciles Harbor RobotAccount managed resources.
package robotaccount

import (
	"context"
	"strconv"

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
	"github.com/goharbor/go-client/pkg/sdk/v2.0/models"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	iamv1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
	apisv1alpha1 "github.com/EvannDev/provider-harbor/apis/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/clients/common"
	iamclient "github.com/EvannDev/provider-harbor/internal/clients/iam"
)

// annotationRobotID persists the Harbor numeric robot ID as an annotation so
// findRobot can use GetRobotAccountByID on the first reconcile after Create.
// Status writes from Create are overwritten by UpdateCriticalAnnotations but
// annotation writes survive because UpdateCriticalAnnotations is client.Update.
const annotationRobotID = "harbor.crossplane.io/robot-id"

// errNamespaceNotResolved is returned by findRobot when the project namespace
// reference has not yet been resolved; the operation should be retried.
var errNamespaceNotResolved = errors.New("project namespace reference not yet resolved")

// Error message constants used throughout the controller.
const (
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errNewClient    = "cannot create Harbor client"
	errObserve      = "cannot observe RobotAccount"
	errCreate       = "cannot create RobotAccount"
	errUpdate       = "cannot update RobotAccount"
	errDelete       = "cannot delete RobotAccount"
)

// SetupGated adds a controller with safe-start support.
func SetupGated(mgr ctrl.Manager, opts controller.Options) error {
	opts.Gate.Register(func() {
		err := Setup(mgr, opts)
		if err != nil {
			panic(errors.Wrap(err, "cannot setup RobotAccount controller"))
		}
	}, iamv1alpha1.RobotAccountGroupVersionKind)

	return nil
}

// Setup adds a controller that reconciles RobotAccount managed resources.
func Setup(mgr ctrl.Manager, opts controller.Options) error {
	name := managed.ControllerName(iamv1alpha1.RobotAccountGroupKind)

	reconcilerOpts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*iamv1alpha1.RobotAccount](&connector{
			kube:         mgr.GetClient(),
			usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newServiceFn: iamclient.NewRobotAccountsClient,
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
			&iamv1alpha1.RobotAccountList{}, opts.MetricOptions.PollStateMetricInterval,
		)

		err := mgr.Add(stateMetricsRecorder)
		if err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for RobotAccountList")
		}
	}

	reconciler := managed.NewReconciler(mgr, resource.ManagedKind(iamv1alpha1.RobotAccountGroupVersionKind), reconcilerOpts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(opts.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&iamv1alpha1.RobotAccount{}).
		Complete(ratelimiter.NewReconciler(name, reconciler, opts.GlobalRateLimiter))
}

// connector creates a Harbor API client from ProviderConfig credentials.
type connector struct {
	kube         client.Client
	usage        *resource.ProviderConfigUsageTracker
	newServiceFn func(common.Config) iamclient.RobotAccountsClient
}

// Connect implements managed.TypedExternalConnecter.
func (c *connector) Connect(ctx context.Context, account *iamv1alpha1.RobotAccount) (managed.TypedExternalClient[*iamv1alpha1.RobotAccount], error) {
	err := c.usage.Track(ctx, account)
	if err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	cfg, err := common.GetConfig(ctx, c.kube, account)
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{client: c.newServiceFn(*cfg)}, nil
}

// external implements Observe/Create/Update/Delete against the Harbor API.
type external struct {
	client iamclient.RobotAccountsClient
}

// robotName returns the external name, falling back to the CR name.
func robotName(account *iamv1alpha1.RobotAccount) string {
	if externalName := meta.GetExternalName(account); externalName != "" {
		return externalName
	}

	return account.GetName()
}

// Observe checks whether the robot account exists and whether it is up-to-date.
func (e *external) Observe(ctx context.Context, account *iamv1alpha1.RobotAccount) (managed.ExternalObservation, error) {
	robot, err := e.findRobot(ctx, account)
	if err != nil {
		if errors.Is(err, errNamespaceNotResolved) || iamclient.IsNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}

		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	account.Status.AtProvider = iamclient.GenerateRobotAccountObservation(robot)
	account.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: iamclient.IsRobotAccountUpToDate(account.Spec.ForProvider, robot),
	}, nil
}

// Create creates the robot account in Harbor.
func (e *external) Create(ctx context.Context, account *iamv1alpha1.RobotAccount) (managed.ExternalCreation, error) {
	account.Status.SetConditions(xpv1.Creating())

	err := iamclient.ValidatePermissions(account.Spec.ForProvider.Permissions)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	name := robotName(account)
	req := iamclient.ToRobotCreate(name, account.Spec.ForProvider)

	created, err := e.client.NewRobotAccount(ctx, req)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	ann := account.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}

	ann[annotationRobotID] = strconv.FormatInt(created.ID, 10)
	account.SetAnnotations(ann)

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{
			"name":   []byte(created.Name),
			"secret": []byte(created.Secret),
		},
	}, nil
}

// Update reconciles the robot account's state with the desired spec.
func (e *external) Update(ctx context.Context, account *iamv1alpha1.RobotAccount) (managed.ExternalUpdate, error) {
	err := iamclient.ValidatePermissions(account.Spec.ForProvider.Permissions)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	robot, err := e.findRobot(ctx, account)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	if robot == nil {
		return managed.ExternalUpdate{}, errors.New(errUpdate + ": robot not found")
	}

	iamclient.ApplyRobotAccountParameters(account.Spec.ForProvider, robot)

	err = e.client.UpdateRobotAccount(ctx, robot.ID, robot)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	return managed.ExternalUpdate{}, nil
}

// Delete removes the robot account from Harbor.
func (e *external) Delete(ctx context.Context, account *iamv1alpha1.RobotAccount) (managed.ExternalDelete, error) {
	account.Status.SetConditions(xpv1.Deleting())

	robot, err := e.findRobot(ctx, account)
	if err != nil {
		if errors.Is(err, errNamespaceNotResolved) || iamclient.IsNotFound(err) {
			return managed.ExternalDelete{}, nil
		}

		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	err = e.client.DeleteRobotAccountByID(ctx, robot.ID)
	if err != nil {
		if iamclient.IsNotFound(err) {
			return managed.ExternalDelete{}, nil
		}

		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	return managed.ExternalDelete{}, nil
}

// Disconnect is a no-op for stateless Harbor clients.
func (e *external) Disconnect(_ context.Context) error { return nil }

// findRobot locates the Harbor robot for this account using the best strategy:
//  1. ID-based lookup when an ID is stored in status (works for all types).
//  2. Annotation ID: set by Create, survives before status is written.
//  3. Project-level robots: scan GET /projects/{project}/robots.
//  4. System-level robots: plain GetRobotAccountByName.
//
// Returns errNamespaceNotResolved when the project namespace is not available.
func (e *external) findRobot(ctx context.Context, account *iamv1alpha1.RobotAccount) (*models.Robot, error) {
	if account.Status.AtProvider.ID != nil {
		return e.client.GetRobotAccountByID(ctx, *account.Status.AtProvider.ID)
	}

	if annotationID := account.GetAnnotations()[annotationRobotID]; annotationID != "" {
		id, parseErr := strconv.ParseInt(annotationID, 10, 64)
		if parseErr == nil {
			return e.client.GetRobotAccountByID(ctx, id)
		}
	}

	if account.Spec.ForProvider.Level == "project" {
		ns := iamclient.ProjectNamespace(account.Spec.ForProvider.Permissions)
		if ns == "" {
			return nil, errNamespaceNotResolved
		}

		return iamclient.FindProjectRobotByName(ctx, e.client, ns, robotName(account))
	}

	return e.client.GetRobotAccountByName(ctx, robotName(account))
}
