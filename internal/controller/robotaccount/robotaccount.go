package robotaccount

import (
	"context"
	"strconv"
	"strings"

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
	apiv2 "github.com/mittwald/goharbor-client/v5/apiv2"
	harborerrors "github.com/mittwald/goharbor-client/v5/apiv2/pkg/errors"
	modelv2 "github.com/mittwald/goharbor-client/v5/apiv2/model"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	harborv1alpha1 "github.com/EvannDev/provider-harbor/apis/harbor/v1alpha1"
	apisv1alpha1 "github.com/EvannDev/provider-harbor/apis/v1alpha1"
	harborclient "github.com/EvannDev/provider-harbor/internal/clients/harbor"
)

// annotationRobotID persists the Harbor numeric robot ID as an annotation so
// that findRobot can use GetRobotAccountByID on the very first reconcile after
// Create. Status writes from Create are overwritten by UpdateCriticalAnnotations
// (the reconciler does client.Update which replaces in-memory status with the
// server copy), but annotation writes survive because UpdateCriticalAnnotations
// itself is that same client.Update call.
const annotationRobotID = "harbor.crossplane.io/robot-id"

const (
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errNewClient    = "cannot create Harbor client"
	errObserve      = "cannot observe RobotAccount"
	errCreate       = "cannot create RobotAccount"
	errUpdate       = "cannot update RobotAccount"
	errDelete       = "cannot delete RobotAccount"
)

// SetupGated adds a controller with safe-start support.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	o.Gate.Register(func() {
		if err := Setup(mgr, o); err != nil {
			panic(errors.Wrap(err, "cannot setup RobotAccount controller"))
		}
	}, harborv1alpha1.RobotAccountGroupVersionKind)
	return nil
}

// Setup adds a controller that reconciles RobotAccount managed resources.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(harborv1alpha1.RobotAccountGroupKind)

	opts := []managed.ReconcilerOption{
		managed.WithTypedExternalConnector[*harborv1alpha1.RobotAccount](&connector{
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
			&harborv1alpha1.RobotAccountList{}, o.MetricOptions.PollStateMetricInterval,
		)
		if err := mgr.Add(stateMetricsRecorder); err != nil {
			return errors.Wrap(err, "cannot register MR state metrics recorder for RobotAccountList")
		}
	}

	r := managed.NewReconciler(mgr, resource.ManagedKind(harborv1alpha1.RobotAccountGroupVersionKind), opts...)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&harborv1alpha1.RobotAccount{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, cr *harborv1alpha1.RobotAccount) (managed.TypedExternalClient[*harborv1alpha1.RobotAccount], error) {
	if err := c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	cl, err := harborclient.NewClient(ctx, c.kube, cr.GetProviderConfigReference(), cr.GetNamespace())
	if err != nil {
		return nil, errors.Wrap(err, errNewClient)
	}

	return &external{harbor: cl}, nil
}

type external struct {
	harbor apiv2.Client
}

// robotName returns the external name, falling back to the CR name.
func robotName(cr *harborv1alpha1.RobotAccount) string {
	if n := meta.GetExternalName(cr); n != "" {
		return n
	}
	return cr.GetName()
}

func (e *external) Observe(ctx context.Context, cr *harborv1alpha1.RobotAccount) (managed.ExternalObservation, error) {
	robot, err := e.findRobot(ctx, cr)
	if err != nil {
		if isNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}
	if robot == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	setObservation(cr, robot)
	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, robot),
	}, nil
}

// findRobot locates the Harbor robot for this CR using the best available
// strategy:
//  1. ID-based lookup when an ID is stored in status (works for all types).
//  2. For project-level robots: scan GET /projects/{project}/robots.
//     GET /robots only lists system-level robots on most Harbor versions,
//     so we use the project-specific endpoint instead.
//  3. For system-level robots: plain GetRobotAccountByName (uses GET /robots).
//
// Returns nil, nil when the namespace is not yet resolved.
func (e *external) findRobot(ctx context.Context, cr *harborv1alpha1.RobotAccount) (*modelv2.Robot, error) {
	// 1. Status ID: available after the first successful Observe.
	if cr.Status.AtProvider.ID != nil {
		return e.harbor.GetRobotAccountByID(ctx, *cr.Status.AtProvider.ID)
	}

	// 2. Annotation ID: set by Create and survives before status is written.
	//    GET /robots/{id} works for any robot type regardless of level.
	if ann := cr.GetAnnotations()[annotationRobotID]; ann != "" {
		if id, err := strconv.ParseInt(ann, 10, 64); err == nil {
			return e.harbor.GetRobotAccountByID(ctx, id)
		}
	}

	// 3. Project-level robots: use GET /projects/{project}/robots.
	//    V2 project robots created via POST /robots do not appear in
	//    GET /robots on most Harbor versions without a level filter.
	if cr.Spec.ForProvider.Level == "project" {
		ns := projectNamespace(cr.Spec.ForProvider.Permissions)
		if ns == "" {
			return nil, nil
		}
		return findProjectRobotByName(ctx, e.harbor, ns, robotName(cr))
	}

	// 4. System-level robots: plain name lookup via GET /robots.
	return e.harbor.GetRobotAccountByName(ctx, robotName(cr))
}

// findProjectRobotByName uses GET /projects/{project}/robots to find a
// project-scoped robot by its short name. Harbor stores the full name as
// robot$<project>+<name>; we match against the short name suffix.
func findProjectRobotByName(ctx context.Context, cl apiv2.Client, project, name string) (*modelv2.Robot, error) {
	robots, err := cl.ListProjectRobotsV1(ctx, project)
	if err != nil {
		return nil, err
	}
	suffix := "+" + name
	for _, r := range robots {
		if strings.HasSuffix(stripRobotPrefix(r.Name), suffix) || stripRobotPrefix(r.Name) == name {
			return r, nil
		}
	}
	return nil, &harborerrors.ErrRobotAccountUnknownResource{}
}

// projectNamespace returns the first resolved project namespace from a
// permission list, or "" if none is available.
func projectNamespace(perms []harborv1alpha1.RobotAccountPermission) string {
	for _, p := range perms {
		if p.Kind == "project" && p.Namespace != "" {
			return p.Namespace
		}
	}
	return ""
}

// setObservation updates the CR status from a Harbor robot. It does NOT touch
// the external-name annotation: for project robots Harbor returns a name like
// robot$<project>+<name>, and overwriting external-name with that value would
// corrupt subsequent lookups and cause a double-prefix on deletion.
func setObservation(cr *harborv1alpha1.RobotAccount, robot *modelv2.Robot) {
	id := robot.ID
	expiresAt := robot.ExpiresAt
	cr.Status.AtProvider = harborv1alpha1.RobotAccountObservation{
		ID:        &id,
		FullName:  &robot.Name,
		ExpiresAt: &expiresAt,
	}
	cr.Status.SetConditions(xpv1.Available())
}

func (e *external) Create(ctx context.Context, cr *harborv1alpha1.RobotAccount) (managed.ExternalCreation, error) {
	cr.Status.SetConditions(xpv1.Creating())

	if err := validatePermissions(cr.Spec.ForProvider.Permissions); err != nil {
		return managed.ExternalCreation{}, err
	}

	name := robotName(cr)
	req := toRobotCreate(name, cr.Spec.ForProvider)

	created, err := e.harbor.NewRobotAccount(ctx, req)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	// Store the robot ID as an annotation so findRobot can call
	// GetRobotAccountByID on the very next reconcile, before the status
	// subresource has been written. Annotation changes made here survive
	// because the reconciler persists them via UpdateCriticalAnnotations
	// (client.Update). Do NOT write to cr.Status — that is overwritten by
	// the same client.Update response before Status().Update() runs.
	ann := cr.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}
	ann[annotationRobotID] = strconv.FormatInt(created.ID, 10)
	cr.SetAnnotations(ann)

	return managed.ExternalCreation{
		ConnectionDetails: managed.ConnectionDetails{
			"name":   []byte(created.Name),
			"secret": []byte(created.Secret),
		},
	}, nil
}

func (e *external) Update(ctx context.Context, cr *harborv1alpha1.RobotAccount) (managed.ExternalUpdate, error) {
	if err := validatePermissions(cr.Spec.ForProvider.Permissions); err != nil {
		return managed.ExternalUpdate{}, err
	}

	robot, err := e.findRobot(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}
	if robot == nil {
		return managed.ExternalUpdate{}, errors.New(errUpdate + ": robot not found")
	}

	applyParameters(cr.Spec.ForProvider, robot)

	if err := e.harbor.UpdateRobotAccount(ctx, robot); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, cr *harborv1alpha1.RobotAccount) (managed.ExternalDelete, error) {
	cr.Status.SetConditions(xpv1.Deleting())

	robot, err := e.findRobot(ctx, cr)
	if err != nil {
		if isNotFound(err) {
			return managed.ExternalDelete{}, nil
		}
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}
	if robot == nil {
		// Namespace not resolved; nothing to delete.
		return managed.ExternalDelete{}, nil
	}

	if err := e.harbor.DeleteRobotAccountByID(ctx, robot.ID); err != nil {
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
	var unknown *harborerrors.ErrRobotAccountUnknownResource
	if errors.As(err, &unknown) {
		return true
	}
	// The goharbor-client does not map all 404 responses to ErrRobotAccountUnknownResource
	// (e.g. GetRobotByID, DeleteRobot). Fall back to the swagger error message pattern.
	return strings.Contains(err.Error(), "][404]")
}

// validatePermissions returns an error if any project-scoped permission has an
// empty namespace, which means the Project reference has not been resolved yet.
func validatePermissions(perms []harborv1alpha1.RobotAccountPermission) error {
	for i, p := range perms {
		if p.Kind == "project" && p.Namespace == "" {
			return errors.Errorf("permissions[%d]: project namespace is empty — "+
				"the referenced Project may not be ready yet; will retry", i)
		}
	}
	return nil
}

// stripRobotPrefix removes the "robot$" prefix Harbor adds to robot names.
func stripRobotPrefix(fullName string) string {
	return strings.TrimPrefix(fullName, "robot$")
}

func toRobotCreate(name string, p harborv1alpha1.RobotAccountParameters) *modelv2.RobotCreate {
	req := &modelv2.RobotCreate{
		Name:        name,
		Description: p.Description,
		Level:       p.Level,
		Disable:     p.Disable,
		Permissions: toHarborPermissions(p.Permissions),
	}
	if p.Duration != nil {
		req.Duration = *p.Duration
	}
	return req
}

func applyParameters(p harborv1alpha1.RobotAccountParameters, robot *modelv2.Robot) {
	robot.Description = p.Description
	robot.Disable = p.Disable
	robot.Permissions = toHarborPermissions(p.Permissions)
	if p.Duration != nil {
		robot.Duration = *p.Duration
	}
}

func toHarborPermissions(perms []harborv1alpha1.RobotAccountPermission) []*modelv2.RobotPermission {
	out := make([]*modelv2.RobotPermission, len(perms))
	for i, p := range perms {
		access := make([]*modelv2.Access, len(p.Access))
		for j, a := range p.Access {
			access[j] = &modelv2.Access{
				Resource: a.Resource,
				Action:   a.Action,
			}
		}
		out[i] = &modelv2.RobotPermission{
			Kind:      p.Kind,
			Namespace: p.Namespace,
			Access:    access,
		}
	}
	return out
}

func isUpToDate(p harborv1alpha1.RobotAccountParameters, robot *modelv2.Robot) bool {
	if p.Description != robot.Description {
		return false
	}
	if p.Disable != robot.Disable {
		return false
	}
	if p.Duration != nil && *p.Duration != robot.Duration {
		return false
	}
	if len(p.Permissions) != len(robot.Permissions) {
		return false
	}
	for i, perm := range p.Permissions {
		rp := robot.Permissions[i]
		if perm.Kind != rp.Kind || perm.Namespace != rp.Namespace {
			return false
		}
		if len(perm.Access) != len(rp.Access) {
			return false
		}
		for j, a := range perm.Access {
			if a.Resource != rp.Access[j].Resource || a.Action != rp.Access[j].Action {
				return false
			}
		}
	}
	return true
}
