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

package project

import (
	"context"
	"fmt"
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
	modelv2 "github.com/mittwald/goharbor-client/v5/apiv2/model"
	harborerrors "github.com/mittwald/goharbor-client/v5/apiv2/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	harborv1alpha1 "github.com/EvannDev/provider-harbor/apis/harbor/v1alpha1"
	apisv1alpha1 "github.com/EvannDev/provider-harbor/apis/v1alpha1"
	harborclient "github.com/EvannDev/provider-harbor/internal/clients/harbor"
	apiv2 "github.com/mittwald/goharbor-client/v5/apiv2"
)

const (
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errNewClient    = "cannot create Harbor client"
	errObserve      = "cannot observe Project"
	errCreate       = "cannot create Project"
	errUpdate       = "cannot update Project"
	errDelete       = "cannot delete Project"
)

// SetupGated adds a controller with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		err := Setup(mgr, o)
		if err != nil {
			panic(errors.Wrap(err, "cannot setup Project controller"))
		}
	}, harborv1alpha1.ProjectGroupVersionKind)

	return nil
}

// Setup adds a controller that reconciles Project managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(harborv1alpha1.ProjectGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*harborv1alpha1.Project](&connector{
			kube:  mgr.GetClient(),
			usage: resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	}

	if o.Features.Enabled(feature.EnableBetaManagementPolicies) {
		opts = append(opts, managed.WithManagementPolicies())
	}

	if o.Features.Enabled(feature.EnableAlphaChangeLogs) {
		opts = append(opts, managed.WithChangeLogger(o.ChangeLogOptions.ChangeLogger))
	}

	if o.MetricOptions != nil {
		opts = append(opts, managed.WithMetricRecorder(o.MetricOptions.MRMetrics))
	}

	if o.MetricOptions != nil && o.MetricOptions.MRStateMetrics != nil {
		stateMetricsRecorder := statemetrics.NewMRStateRecorder(
			mgr.GetClient(), o.Logger, o.MetricOptions.MRStateMetrics,
			&harborv1alpha1.ProjectList{}, o.MetricOptions.PollStateMetricInterval,
		)

		err := mgr.Add(stateMetricsRecorder)
		if err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for ProjectList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(harborv1alpha1.ProjectGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&harborv1alpha1.Project{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

// connector creates a Harbor API client from ProviderConfig credentials.
type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, cr *harborv1alpha1.Project) (managed.TypedExternalClient[*harborv1alpha1.Project], error) {
	if err := c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	cl, err := harborclient.NewClient(ctx, c.kube, cr.GetProviderConfigReference(), cr.GetNamespace())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{harbor: cl}, nil
}

// external implements Observe/Create/Update/Delete against the Harbor API.
type external struct {
	harbor apiv2.Client
}

// projectName returns the external name of the project, falling back to the CR name.
func projectName(cr *harborv1alpha1.Project) string {
	if n := meta.GetExternalName(cr); n != "" {
		return n
	}

	return cr.GetName()
}

func (e *external) Observe(ctx context.Context, cr *harborv1alpha1.Project) (managed.ExternalObservation, error) {
	name := projectName(cr)

	proj, err := e.harbor.GetProject(ctx, name)
	if err != nil {
		if isNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}

		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	// Populate observed state.
	id := proj.ProjectID
	repoCount := proj.RepoCount
	cr.Status.AtProvider = harborv1alpha1.ProjectObservation{
		ProjectID: &id,
		OwnerName: &proj.OwnerName,
		RepoCount: &repoCount,
	}
	meta.SetExternalName(cr, proj.Name)

	cr.Status.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, proj),
	}, nil
}

func (e *external) Create(ctx context.Context, cr *harborv1alpha1.Project) (managed.ExternalCreation, error) {
	cr.Status.SetConditions(xpv1.Creating())

	name := projectName(cr)
	req := toProjectReq(name, cr.Spec.ForProvider)

	err := e.harbor.NewProject(ctx, req)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	meta.SetExternalName(cr, name)

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, cr *harborv1alpha1.Project) (managed.ExternalUpdate, error) {
	name := projectName(cr)

	proj, err := e.harbor.GetProject(ctx, name)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	applyParameters(cr.Spec.ForProvider, proj)

	if err := e.harbor.UpdateProject(ctx, proj, cr.Spec.ForProvider.StorageLimit); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, cr *harborv1alpha1.Project) (managed.ExternalDelete, error) {
	cr.Status.SetConditions(xpv1.Deleting())

	err := e.harbor.DeleteProject(ctx, projectName(cr))
	if err != nil {
		if isNotFound(err) {
			return managed.ExternalDelete{}, nil
		}

		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(_ context.Context) error { return nil }

// --- helpers ---

func isNotFound(err error) bool {
	var (
		notFound *harborerrors.ErrProjectNotFound
		mismatch *harborerrors.ErrProjectMismatch
	)

	return errors.As(err, &notFound) || errors.As(err, &mismatch)
}

// toProjectReq converts CR parameters into a Harbor ProjectReq for creation.
func toProjectReq(name string, p harborv1alpha1.ProjectParameters) *modelv2.ProjectReq {
	req := &modelv2.ProjectReq{
		ProjectName:  name,
		StorageLimit: p.StorageLimit,
	}
	if p.Metadata != nil {
		req.Metadata = toHarborMetadata(p.Metadata)
	}

	return req
}

// applyParameters writes desired state from ForProvider onto an existing Harbor Project object.
func applyParameters(p harborv1alpha1.ProjectParameters, proj *modelv2.Project) {
	if proj.Metadata == nil {
		proj.Metadata = &modelv2.ProjectMetadata{}
	}

	if p.Metadata != nil {
		meta := toHarborMetadata(p.Metadata)
		proj.Metadata.Public = meta.Public
		proj.Metadata.AutoScan = meta.AutoScan
		proj.Metadata.EnableContentTrust = meta.EnableContentTrust
		proj.Metadata.EnableContentTrustCosign = meta.EnableContentTrustCosign
		proj.Metadata.PreventVul = meta.PreventVul
		proj.Metadata.Severity = meta.Severity
		proj.Metadata.ReuseSysCVEAllowlist = meta.ReuseSysCVEAllowlist
	}
}

// toHarborMetadata converts typed booleans into the string-based ProjectMetadata Harbor expects.
func toHarborMetadata(m *harborv1alpha1.ProjectMetadataParameters) *modelv2.ProjectMetadata {
	meta := &modelv2.ProjectMetadata{}
	if m.Public != nil {
		meta.Public = boolToString(*m.Public)
	}

	if m.AutoScan != nil {
		meta.AutoScan = strPtr(boolToString(*m.AutoScan))
	}

	if m.EnableContentTrust != nil {
		meta.EnableContentTrust = strPtr(boolToString(*m.EnableContentTrust))
	}

	if m.EnableContentTrustCosign != nil {
		meta.EnableContentTrustCosign = strPtr(boolToString(*m.EnableContentTrustCosign))
	}

	if m.PreventVulnerable != nil {
		meta.PreventVul = strPtr(boolToString(*m.PreventVulnerable))
	}

	if m.Severity != nil {
		meta.Severity = m.Severity
	}

	if m.ReuseSysCVEAllowlist != nil {
		meta.ReuseSysCVEAllowlist = strPtr(boolToString(*m.ReuseSysCVEAllowlist))
	}

	return meta
}

// isUpToDate returns true if the observed Harbor project matches the desired spec.
func isUpToDate(p harborv1alpha1.ProjectParameters, proj *modelv2.Project) bool {
	if p.StorageLimit != nil {
		// Harbor returns the quota separately; we treat any non-nil desired value
		// as potentially needing a check — Update is idempotent, so false here is safe.
		return false
	}

	if p.Metadata == nil {
		return true
	}

	if proj.Metadata == nil {
		return false
	}

	m := p.Metadata

	pm := proj.Metadata
	if m.Public != nil && pm.Public != boolToString(*m.Public) {
		return false
	}

	if m.AutoScan != nil && derefStr(pm.AutoScan) != boolToString(*m.AutoScan) {
		return false
	}

	if m.EnableContentTrust != nil && derefStr(pm.EnableContentTrust) != boolToString(*m.EnableContentTrust) {
		return false
	}

	if m.EnableContentTrustCosign != nil && derefStr(pm.EnableContentTrustCosign) != boolToString(*m.EnableContentTrustCosign) {
		return false
	}

	if m.PreventVulnerable != nil && derefStr(pm.PreventVul) != boolToString(*m.PreventVulnerable) {
		return false
	}

	if m.Severity != nil && derefStr(pm.Severity) != *m.Severity {
		return false
	}

	if m.ReuseSysCVEAllowlist != nil && derefStr(pm.ReuseSysCVEAllowlist) != boolToString(*m.ReuseSysCVEAllowlist) {
		return false
	}

	return true
}

func boolToString(b bool) string { return strconv.FormatBool(b) }
func strPtr(s string) *string    { return &s }
func derefStr(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

// Ensure fmt is used (avoids import removal if logging is added later).
var _ = fmt.Sprintf
