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

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
)

// ProjectMetadataParameters configures Harbor project-level feature flags.
type ProjectMetadataParameters struct {
	// Public controls whether the project is publicly accessible.
	// +optional
	Public *bool `json:"public,omitempty"`

	// AutoScan enables automatic vulnerability scanning on image push.
	// +optional
	AutoScan *bool `json:"autoScan,omitempty"`

	// EnableContentTrust blocks unsigned image pulls when true.
	// +optional
	EnableContentTrust *bool `json:"enableContentTrust,omitempty"`

	// EnableContentTrustCosign blocks pulls of images without a cosign signature when true.
	// +optional
	EnableContentTrustCosign *bool `json:"enableContentTrustCosign,omitempty"`

	// PreventVulnerable blocks image pulls when vulnerabilities exceed Severity.
	// +optional
	PreventVulnerable *bool `json:"preventVulnerable,omitempty"`

	// Severity is the minimum vulnerability level that blocks image pulls.
	// Requires PreventVulnerable to be true.
	// +optional
	// +kubebuilder:validation:Enum=none;low;medium;high;critical
	Severity *string `json:"severity,omitempty"`

	// ReuseSysCVEAllowlist makes the project inherit the system CVE allowlist.
	// +optional
	ReuseSysCVEAllowlist *bool `json:"reuseSysCVEAllowlist,omitempty"`
}

// ProjectParameters defines the desired state of a Harbor Project.
type ProjectParameters struct {
	// StorageLimit is the storage quota for the project in bytes.
	// Use -1 for unlimited.
	// +optional
	StorageLimit *int64 `json:"storageLimit,omitempty"`

	// Metadata configures project-level feature flags.
	// +optional
	Metadata *ProjectMetadataParameters `json:"metadata,omitempty"`
}

// ProjectObservation holds the fields observed from Harbor after reconciliation.
type ProjectObservation struct {
	// ProjectID is the internal numeric ID assigned by Harbor.
	// +optional
	ProjectID *int32 `json:"projectID,omitempty"`

	// OwnerName is the username of the project owner.
	// +optional
	OwnerName *string `json:"ownerName,omitempty"`

	// RepoCount is the number of repositories in the project.
	// +optional
	RepoCount *int64 `json:"repoCount,omitempty"`
}

// ProjectSpec defines the desired state of a Project.
type ProjectSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`

	ForProvider ProjectParameters `json:"forProvider"`
}

// ProjectStatus represents the observed state of a Project.
type ProjectStatus struct {
	xpv1.ResourceStatus `json:",inline"`

	AtProvider ProjectObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A Project is a Harbor project managed resource.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,harbor}
type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectSpec   `json:"spec"`
	Status ProjectStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ProjectList contains a list of Project.
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Project `json:"items"`
}

var (
	ProjectKind             = reflect.TypeFor[Project]().Name()
	ProjectGroupKind        = schema.GroupKind{Group: Group, Kind: ProjectKind}.String()
	ProjectKindAPIVersion   = ProjectKind + "." + SchemeGroupVersion.String()
	ProjectGroupVersionKind = SchemeGroupVersion.WithKind(ProjectKind)
)

func init() {
	SchemeBuilder.Register(&Project{}, &ProjectList{})
}
