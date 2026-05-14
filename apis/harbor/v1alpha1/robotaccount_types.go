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

// RobotAccountAccess defines a single action allowed on a resource.
type RobotAccountAccess struct {
	// Resource is the Harbor resource type (e.g. repository, artifact, tag).
	Resource string `json:"resource"`
	// Action is the allowed action (e.g. pull, push, delete, read, create, list).
	Action string `json:"action"`
}

// RobotAccountPermission grants a set of actions on a project namespace.
type RobotAccountPermission struct {
	// Kind is the scope kind: "project" or "system".
	// +kubebuilder:validation:Enum=project;system
	Kind string `json:"kind"`

	// Namespace is the project name this permission applies to.
	// Use "*" for all projects (system-level robots only).
	// +crossplane:generate:reference:type=Project
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// NamespaceRef is a reference to a Project managed resource whose
	// external name will be used as the namespace value.
	// +optional
	NamespaceRef *xpv1.NamespacedReference `json:"namespaceRef,omitempty"`

	// NamespaceSelector selects a Project managed resource by labels.
	// +optional
	NamespaceSelector *xpv1.NamespacedSelector `json:"namespaceSelector,omitempty"`

	// Access is the list of resource/action pairs granted.
	Access []RobotAccountAccess `json:"access"`
}

// RobotAccountParameters defines the desired state of a Harbor robot account.
type RobotAccountParameters struct {
	// Description is an optional human-readable description of the robot.
	// +optional
	Description string `json:"description,omitempty"`

	// Duration is the token validity period in days. Use -1 for no expiry.
	// +optional
	// +kubebuilder:default=-1
	Duration *int64 `json:"duration,omitempty"`

	// Level defines the scope of the robot: "system" or "project".
	// +kubebuilder:validation:Enum=system;project
	// +kubebuilder:default=system
	Level string `json:"level"`

	// Disable disables the robot account without deleting it.
	// +optional
	Disable bool `json:"disable,omitempty"`

	// Permissions is the list of project/system permissions granted to this robot.
	Permissions []RobotAccountPermission `json:"permissions"`
}

// RobotAccountObservation holds fields reported by Harbor after reconciliation.
type RobotAccountObservation struct {
	// ID is the internal numeric ID assigned by Harbor.
	// +optional
	ID *int64 `json:"id,omitempty"`

	// FullName is the full robot account name as returned by Harbor (e.g. robot$name).
	// +optional
	FullName *string `json:"fullName,omitempty"`

	// ExpiresAt is the Unix timestamp when the token expires (-1 = never).
	// +optional
	ExpiresAt *int64 `json:"expiresAt,omitempty"`
}

// RobotAccountSpec defines the desired state of a RobotAccount.
type RobotAccountSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`

	ForProvider RobotAccountParameters `json:"forProvider"`
}

// RobotAccountStatus represents the observed state of a RobotAccount.
type RobotAccountStatus struct {
	xpv1.ResourceStatus `json:",inline"`

	AtProvider RobotAccountObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true

// A RobotAccount is a Harbor robot account managed resource.
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,categories={crossplane,managed,harbor}
type RobotAccount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RobotAccountSpec   `json:"spec"`
	Status RobotAccountStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// RobotAccountList contains a list of RobotAccount.
type RobotAccountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []RobotAccount `json:"items"`
}

var (
	RobotAccountKind             = reflect.TypeFor[RobotAccount]().Name()
	RobotAccountGroupKind        = schema.GroupKind{Group: Group, Kind: RobotAccountKind}.String()
	RobotAccountKindAPIVersion   = RobotAccountKind + "." + SchemeGroupVersion.String()
	RobotAccountGroupVersionKind = SchemeGroupVersion.WithKind(RobotAccountKind)
)

func init() {
	SchemeBuilder.Register(&RobotAccount{}, &RobotAccountList{})
}
