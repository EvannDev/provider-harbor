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

// Package common provides shared Harbor API client utilities.
package common

import (
	"context"
	"encoding/json"
	"net/url"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/goharbor/go-client/pkg/harbor"
	v2client "github.com/goharbor/go-client/pkg/sdk/v2.0/client"

	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apisv1alpha1 "github.com/EvannDev/provider-harbor/apis/v1alpha1"
)

// clusterProviderConfigKind is the kind for ClusterProviderConfig resources.
const clusterProviderConfigKind = "ClusterProviderConfig"

// Config holds Harbor connection credentials for creating an API client.
type Config struct {
	// URL is the Harbor instance base URL.
	URL string
	// Username is the Harbor account username.
	Username string
	// Password is the Harbor account password.
	Password string
	// InsecureSkipTLSVerify disables TLS certificate verification.
	InsecureSkipTLSVerify bool
}

// harborCredentials holds credentials parsed from a Kubernetes Secret.
// The Secret value must be a JSON object with "username" and "password" keys.
type harborCredentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// NewHarborAPI creates an authenticated Harbor v2 API client from a Config.
func NewHarborAPI(cfg Config) *v2client.HarborAPI {
	parsedURL, err := url.Parse(cfg.URL)
	if err != nil {
		panic(errors.Wrap(err, "cannot parse Harbor URL"))
	}

	var transport = harbor.InsecureTransport
	if !cfg.InsecureSkipTLSVerify {
		transport = nil
	}

	harborCfg := &harbor.Config{
		URL:       parsedURL,
		Transport: transport,
		AuthInfo:  httptransport.BasicAuth(cfg.Username, cfg.Password),
	}

	return v2client.New(harborCfg.ToV2Config())
}

// GetConfig constructs a Config from the ProviderConfig referenced
// by the managed resource.
func GetConfig(ctx context.Context, kube client.Client, managedResource resource.ModernManaged) (*Config, error) {
	providerConfigRef := managedResource.GetProviderConfigReference()
	if providerConfigRef == nil {
		return nil, errors.New("providerConfigRef is not set")
	}

	spec, err := getProviderConfigSpec(ctx, kube, providerConfigRef.Name, providerConfigRef.Kind, managedResource.GetNamespace())
	if err != nil {
		return nil, err
	}

	raw, err := resource.CommonCredentialExtractor(ctx, spec.Credentials.Source, kube, spec.Credentials.CommonCredentialSelectors)
	if err != nil {
		return nil, err
	}

	var creds harborCredentials

	err = json.Unmarshal(raw, &creds)
	if err != nil {
		return nil, errors.Wrap(err, "cannot parse Harbor credentials")
	}

	return &Config{
		URL:                   spec.URL,
		Username:              creds.Username,
		Password:              creds.Password,
		InsecureSkipTLSVerify: spec.InsecureSkipTLSVerify,
	}, nil
}

// getProviderConfigSpec retrieves the ProviderConfigSpec for the named config.
func getProviderConfigSpec(ctx context.Context, kube client.Client, name, kind, namespace string) (apisv1alpha1.ProviderConfigSpec, error) {
	switch kind {
	case clusterProviderConfigKind:
		clusterConfig := &apisv1alpha1.ClusterProviderConfig{}

		err := kube.Get(ctx, types.NamespacedName{Name: name}, clusterConfig)
		if err != nil {
			return apisv1alpha1.ProviderConfigSpec{}, errors.Wrap(err, "cannot get ClusterProviderConfig")
		}

		return clusterConfig.Spec, nil
	default:
		providerConfig := &apisv1alpha1.ProviderConfig{}

		err := kube.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, providerConfig)
		if err != nil {
			return apisv1alpha1.ProviderConfigSpec{}, errors.Wrap(err, "cannot get ProviderConfig")
		}

		return providerConfig.Spec, nil
	}
}
