package harbor

import (
	"context"
	"crypto/tls"
	"net/http"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	apiv2 "github.com/mittwald/goharbor-client/v5/apiv2"
	"github.com/mittwald/goharbor-client/v5/apiv2/pkg/config"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apisv1alpha1 "github.com/EvannDev/provider-harbor/apis/v1alpha1"
)

// NewClient builds an authenticated Harbor API client from a ProviderConfig reference.
//
// It reads the ProviderConfig (or ClusterProviderConfig) named by ref, extracts the
// Harbor URL and credentials from the referenced Kubernetes Secret, and returns a
// ready-to-use apiv2.Client that every controller can call directly.
func NewClient(ctx context.Context, kube client.Client, ref *xpv1.ProviderConfigReference, namespace string) (apiv2.Client, error) {
	if ref == nil {
		return nil, errors.New("providerConfigRef is not set")
	}
	spec, err := getProviderConfigSpec(ctx, kube, *ref, namespace)
	if err != nil {
		return nil, err
	}

	raw, err := resource.CommonCredentialExtractor(ctx, spec.Credentials.Source, kube, spec.Credentials.CommonCredentialSelectors)
	if err != nil {
		return nil, err
	}

	creds, err := ParseCredentials(raw)
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{}
	if spec.InsecureSkipTLSVerify {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // user opt-in
		}
	}

	// The goharbor-client library appends "/v2.0" to the URL to form the base
	// path. Harbor's API lives at /api/v2.0, so we ensure the URL ends with
	// "/api" so the final base path is /api/v2.0.
	apiURL := strings.TrimRight(spec.URL, "/")
	if !strings.HasSuffix(apiURL, "/api") && !strings.HasSuffix(apiURL, "/v2.0") {
		apiURL += "/api"
	}

	return apiv2.NewRESTClientForHostWithClient(apiURL, creds.Username, creds.Password, config.Defaults(), httpClient)
}

// getProviderConfigSpec fetches the ProviderConfig or ClusterProviderConfig
// identified by ref and returns its Spec.
func getProviderConfigSpec(ctx context.Context, kube client.Client, ref xpv1.ProviderConfigReference, namespace string) (apisv1alpha1.ProviderConfigSpec, error) {
	switch ref.Kind {
	case "ClusterProviderConfig":
		cpc := &apisv1alpha1.ClusterProviderConfig{}
		if err := kube.Get(ctx, types.NamespacedName{Name: ref.Name}, cpc); err != nil {
			return apisv1alpha1.ProviderConfigSpec{}, err
		}
		return cpc.Spec, nil
	default: // "ProviderConfig" or unset
		pc := &apisv1alpha1.ProviderConfig{}
		if err := kube.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, pc); err != nil {
			return apisv1alpha1.ProviderConfigSpec{}, err
		}
		return pc.Spec, nil
	}
}
