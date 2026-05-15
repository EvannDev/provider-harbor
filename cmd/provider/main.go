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

// Package main is the entry point for the Harbor Crossplane provider.
package main

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"

	changelogsv1alpha1 "github.com/crossplane/crossplane-runtime/v2/apis/changelogs/proto/v1alpha1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/feature"
	"github.com/crossplane/crossplane-runtime/v2/pkg/gate"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/customresourcesgate"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/statemetrics"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/EvannDev/provider-harbor/apis"
	template "github.com/EvannDev/provider-harbor/internal/controller"
	"github.com/EvannDev/provider-harbor/internal/version"
)

// leaseDurationSecs is the leader election lease duration in seconds.
const leaseDurationSecs = 60

// renewDeadlineSecs is the leader election renew deadline in seconds.
const renewDeadlineSecs = 50

// main is the entry point for the Harbor provider binary.
func main() {
	var (
		app            = kingpin.New(filepath.Base(os.Args[0]), "Template support for Crossplane.").DefaultEnvars()
		debug          = app.Flag("debug", "Run with debug logging.").Short('d').Bool()
		leaderElection = app.Flag("leader-election", "Use leader election for the controller manager.").Short('l').Default("false").Envar("LEADER_ELECTION").Bool()

		syncInterval            = app.Flag("sync", "How often all resources will be double-checked for drift from the desired state.").Short('s').Default("1h").Duration()
		pollInterval            = app.Flag("poll", "How often individual resources will be checked for drift from the desired state").Default("1m").Duration()
		pollStateMetricInterval = app.Flag("poll-state-metric", "State metric recording interval").Default("5s").Duration()

		maxReconcileRate = app.Flag("max-reconcile-rate", "The global maximum rate per second at which resources may checked for drift from the desired state.").Default("10").Int()

		enableManagementPolicies = app.Flag("enable-management-policies", "Enable support for Management Policies.").Default("true").Envar("ENABLE_MANAGEMENT_POLICIES").Bool()
		enableChangeLogs         = app.Flag("enable-changelogs", "Enable support for capturing change logs during reconciliation.").Default("false").Envar("ENABLE_CHANGE_LOGS").Bool()
		changelogsSocketPath     = app.Flag("changelogs-socket-path", "Path for changelogs socket (if enabled)").Default("/var/run/changelogs/changelogs.sock").Envar("CHANGELOGS_SOCKET_PATH").String()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	zapLogger := zap.New(zap.UseDevMode(*debug))
	log := logging.NewLogrLogger(zapLogger.WithName("provider-harbor"))

	if *debug {
		// The controller-runtime is *very* verbose even at info level, so we only
		// provide it a real logger when we're running in debug mode.
		ctrl.SetLogger(zapLogger)
	} else {
		// Setting the controller-runtime logger to a no-op logger by default. This
		// is not really needed, but otherwise we get a warning from the
		// controller-runtime.
		ctrl.SetLogger(zap.New(zap.WriteTo(io.Discard)))
	}

	mgr, err := newManager(*maxReconcileRate, syncInterval, *leaderElection)
	kingpin.FatalIfError(err, "Cannot create controller manager")

	kingpin.FatalIfError(apis.AddToScheme(mgr.GetScheme()), "Cannot add Template APIs to scheme")
	kingpin.FatalIfError(apiextensionsv1.AddToScheme(mgr.GetScheme()), "Cannot add CustomResourceDefinition to scheme")

	metricRecorder := managed.NewMRMetricRecorder()
	stateMetrics := statemetrics.NewMRStateMetrics()

	metrics.Registry.MustRegister(metricRecorder)
	metrics.Registry.MustRegister(stateMetrics)

	opts := buildControllerOptions(log, *maxReconcileRate, *pollInterval, *pollStateMetricInterval, metricRecorder, stateMetrics)

	if *enableManagementPolicies {
		opts.Features.Enable(feature.EnableBetaManagementPolicies)
		log.Info("Beta feature enabled", "flag", feature.EnableBetaManagementPolicies)
	}

	if *enableChangeLogs {
		opts.Features.Enable(feature.EnableAlphaChangeLogs)
		log.Info("Alpha feature enabled", "flag", feature.EnableAlphaChangeLogs)

		clo, clErr := buildChangeLogOptions(*changelogsSocketPath)
		kingpin.FatalIfError(clErr, "failed to create change logs client connection at %s", *changelogsSocketPath)

		opts.ChangeLogOptions = &clo
	}

	kingpin.FatalIfError(customresourcesgate.Setup(mgr, opts), "Cannot setup CRD gate controller")
	kingpin.FatalIfError(template.SetupGated(mgr, opts), "Cannot setup Template controllers")
	kingpin.FatalIfError(mgr.Start(ctrl.SetupSignalHandler()), "Cannot start controller manager")
}

// newManager creates the controller-runtime manager with leader election
// configured.
func newManager(maxReconcileRate int, syncInterval *time.Duration, leaderElection bool) (ctrl.Manager, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return nil, err
	}

	return ctrl.NewManager(ratelimiter.LimitRESTConfig(cfg, maxReconcileRate), ctrl.Options{
		// SyncPeriod in ctrl.Options has been removed since controller-runtime v0.16.0
		// The recommended way is to move it to cache.Options instead
		Cache: cache.Options{
			SyncPeriod: syncInterval,
		},

		// controller-runtime uses both ConfigMaps and Leases for leader
		// election by default. Leases expire after 15 seconds, with a
		// 10 seconds renewal deadline. We've observed leader loss due to
		// renewal deadlines being exceeded when under high load - i.e.
		// hundreds of reconciles per second and ~200rps to the API
		// server. Switching to Leases only and longer leases appears to
		// alleviate this.
		LeaderElection:             leaderElection,
		LeaderElectionID:           "crossplane-leader-election-provider-harbor",
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		LeaseDuration:              new(leaseDurationSecs * time.Second),
		RenewDeadline:              new(renewDeadlineSecs * time.Second),
	})
}

// buildControllerOptions assembles the controller.Options from parsed flags
// and pre-built metric recorders.
func buildControllerOptions(
	log logging.Logger,
	maxReconcileRate int,
	pollInterval time.Duration,
	pollStateMetricInterval time.Duration,
	mrMetrics *managed.MRMetricRecorder,
	stateMetrics *statemetrics.MRStateMetrics,
) controller.Options {
	return controller.Options{
		Logger:                  log,
		MaxConcurrentReconciles: maxReconcileRate,
		PollInterval:            pollInterval,
		GlobalRateLimiter:       ratelimiter.NewGlobal(maxReconcileRate),
		Features:                &feature.Flags{},
		Gate:                    new(gate.Gate[schema.GroupVersionKind]),
		MetricOptions: &controller.MetricOptions{
			PollStateMetricInterval: pollStateMetricInterval,
			MRMetrics:               mrMetrics,
			MRStateMetrics:          stateMetrics,
		},
	}
}

// buildChangeLogOptions dials the changelogs gRPC socket and returns the
// options.
func buildChangeLogOptions(socketPath string) (controller.ChangeLogOptions, error) {
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return controller.ChangeLogOptions{}, err
	}

	return controller.ChangeLogOptions{
		ChangeLogger: managed.NewGRPCChangeLogger(
			changelogsv1alpha1.NewChangeLogServiceClient(conn),
			managed.WithProviderVersion("provider-harbor:"+version.Version),
		),
	}, nil
}
