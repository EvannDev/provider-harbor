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

// Package group reconciles Harbor Group managed resources.
package group

import (
	"context"
	"fmt"
	"strconv"

	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"

	v1alpha1 "github.com/EvannDev/provider-harbor/apis/iam/v1alpha1"
	apisv1alpha1 "github.com/EvannDev/provider-harbor/apis/v1alpha1"
	"github.com/EvannDev/provider-harbor/internal/clients/common"
	"github.com/EvannDev/provider-harbor/internal/clients/iam"
)

const (
	// errNotGroup is returned when the managed resource is not a Group.
	errNotGroup = "managed resource is not a Group custom resource"
	// errTrackPCUsage is returned when ProviderConfig usage tracking fails.
	errTrackPCUsage = "cannot track ProviderConfig usage"
	// errGetPC is returned when the ProviderConfig cannot be retrieved.
	errGetPC = "cannot get ProviderConfig"
	// errExternalNameNotSet is returned when the external name is missing.
	errExternalNameNotSet = "external name is not set for Group resource %s"
	// errCreate is returned when the Harbor API errors during Group creation.
	errCreate = "cannot create Group in Harbor"
	// errObserve is returned when the Harbor API errors during Group observation.
	errObserve = "cannot observe Group in Harbor"
	// errUpdate is returned when the Harbor API errors during Group update.
	errUpdate = "cannot update Group in Harbor"
	// errDelete is returned when the Harbor API errors during Group deletion.
	errDelete = "cannot delete Group in Harbor"
)

// SetupGated adds a controller for Group with safe-start CRD gate support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		err := Setup(mgr, o)
		if err != nil {
			panic(errors.Wrap(err, "cannot setup Group controller"))
		}
	}, v1alpha1.GroupGroupVersionKind)

	return nil
}

// Setup adds a controller that reconciles Group managed resources.
func Setup(mgr ctrl.Manager, options controller.Options) error {
	name := managed.ControllerName(v1alpha1.GroupGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithExternalConnector(&connector{
			kube:         mgr.GetClient(),
			usage:        resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newServiceFn: iam.NewGroupsClient,
		}),
		managed.WithLogger(options.Logger.WithValues("controller", name)),
		managed.WithPollInterval(options.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name) /*nolint:staticcheck // deprecated but not yet replaced*/)),
	}

	if options.Features.Enabled(feature.EnableBetaManagementPolicies) {
		opts = append(opts, managed.WithManagementPolicies())
	}

	if options.Features.Enabled(feature.EnableAlphaChangeLogs) {
		opts = append(opts, managed.WithChangeLogger(options.ChangeLogOptions.ChangeLogger))
	}

	if options.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(options.MetricOptions.MRMetrics))
	}

	if options.MetricOptions != nil && options.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), options.Logger, options.MetricOptions.MRStateMetrics, &v1alpha1.GroupList{}, options.MetricOptions.PollStateMetricInterval,
		)

		err := mgr.Add(stateMetricsRecorder)
		if err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for kind v1alpha1.GroupList")
		}
	}

	reconciler := managed.NewReconciler(mgr, resource.ManagedKind(v1alpha1.GroupGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(options.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.Group{}).
		Complete(ratelimiter.NewReconciler(name, reconciler, options.GlobalRateLimiter))
}

// connector produces an ExternalClient from ProviderConfig credentials.
type connector struct {
	kube         client.Client
	usage        *resource.ProviderConfigUsageTracker
	newServiceFn func(config common.Config) iam.GroupsClient
}

// Connect implements managed.ExternalConnecter.
func (c *connector) Connect(ctx context.Context, managedResource resource.Managed) (managed.ExternalClient, error) {
	groupResource, isValid := managedResource.(*v1alpha1.Group)
	if !isValid {
		return nil, errors.New(errNotGroup)
	}

	err := c.usage.Track(ctx, groupResource)
	if err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	modernManaged, isValid := managedResource.(resource.ModernManaged)
	if !isValid {
		return nil, errors.New("managed resource is not a ModernManaged")
	}

	cfg, err := common.GetConfig(ctx, c.kube, modernManaged)
	if err != nil || cfg == nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	svc := c.newServiceFn(*cfg)

	return &external{client: svc}, nil
}

// external reconciles Group resources against the Harbor API.
type external struct {
	// client is used to interact with the Harbor Group API.
	client iam.GroupsClient
}

// Create creates the external Group resource via the Harbor API.
func (c *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	groupResource, ok := mg.(*v1alpha1.Group)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotGroup)
	}

	groupResource.Status.SetConditions(xpv1.Creating())

	_, err := c.client.CreateUserGroup(ctx, iam.GenerateUserGroupCreateOpts(&groupResource.Spec.ForProvider))
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	// external name IS NOT set here because the API does not return it.

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

// Update reconciles the external Group resource with the desired spec.
func (c *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	groupResource, ok := mg.(*v1alpha1.Group)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotGroup)
	}

	groupId, err := getGroupID(groupResource)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	_, err = c.client.UpdateUserGroup(ctx, iam.GenerateUserGroupUpdateOpts(groupId, &groupResource.Spec.ForProvider))
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	return managed.ExternalUpdate{
		ConnectionDetails: managed.ConnectionDetails{},
	}, nil
}

// Delete removes the external Group resource via the Harbor API.
func (c *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	groupResource, ok := mg.(*v1alpha1.Group)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotGroup)
	}

	groupId, err := getGroupID(groupResource)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	groupResource.Status.SetConditions(xpv1.Deleting())

	_, err = c.client.DeleteUserGroup(ctx, iam.GenerateUserGroupDeleteOpts(groupId))
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	return managed.ExternalDelete{}, nil
}

// Disconnect is a no-op for stateless Harbor clients.
func (c *external) Disconnect(_ context.Context) error {
	return nil
}

// getGroupID retrieves the group ID from the external name of the resource.
func getGroupID(groupResource *v1alpha1.Group) (int64, error) {
	externalName := meta.GetExternalName(groupResource)
	if externalName == "" {
		return 0, fmt.Errorf(errExternalNameNotSet, groupResource.Name)
	}

	groupId, err := strconv.ParseInt(externalName, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "cannot parse external name as group ID")
	}

	return groupId, nil
}
