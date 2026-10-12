// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2 "github.com/crossplane/crossplane/apis/v2/core/v2"
)

// ScopeParameters set which log and trace scopes a project's observability
// scope uses. Every project has exactly one scope, _Default, so this resource
// manages settings of an existing object rather than creating one; the
// external name is the scope ID and defaults to _Default.
type ScopeParameters struct {
	// Project whose scope is managed. Defaults to the projectID of the
	// ProviderConfig.
	// +kubebuilder:validation:Optional
	Project *string `json:"project,omitempty"`

	// Location of the scope. Only global is supported by the API.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default=global
	Location string `json:"location,omitempty"`

	// LogScope is the full resource name of the default log scope, for example
	// //logging.googleapis.com/projects/my-project/locations/global/logScopes/my-scope
	// +kubebuilder:validation:Optional
	LogScope *string `json:"logScope,omitempty"`

	// TraceScope is the resource name of the default trace scope, for example
	// projects/my-project/locations/global/traceScopes/my-scope
	// +kubebuilder:validation:Optional
	TraceScope *string `json:"traceScope,omitempty"`
}

// ScopeObservation is the observed state of the scope.
type ScopeObservation struct {
	Name       string `json:"name,omitempty"`
	LogScope   string `json:"logScope,omitempty"`
	TraceScope string `json:"traceScope,omitempty"`
}

// ScopeSpec defines the desired state of Scope.
type ScopeSpec struct {
	v2.ClusterManagedResourceSpec `json:",inline"`
	ForProvider                   ScopeParameters `json:"forProvider"`
}

// ScopeStatus defines the observed state of Scope.
type ScopeStatus struct {
	v2.ManagedResourceStatus `json:",inline"`
	AtProvider               ScopeObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// Scope manages a project's Observability scope: the default log and trace
// scopes used by the Cloud console (Logs Explorer, Trace Explorer and
// Application Monitoring). Deleting it resets the scope to the project's own
// _Default log scope and no trace scope.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="EXTERNAL-NAME",type="string",JSONPath=".metadata.annotations.crossplane\\.io/external-name"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,gcpobservability}
type Scope struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ScopeSpec   `json:"spec"`
	Status            ScopeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ScopeList contains a list of Scopes.
type ScopeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Scope `json:"items"`
}

// Scope type metadata.
var (
	Scope_Kind             = reflect.TypeOf(Scope{}).Name()
	Scope_GroupKind        = CRDGroupVersion.WithKind(Scope_Kind).GroupKind()
	Scope_KindAPIVersion   = Scope_Kind + "." + CRDGroupVersion.String()
	Scope_GroupVersionKind = CRDGroupVersion.WithKind(Scope_Kind)
)

func init() {
	SchemeBuilder.Register(&Scope{}, &ScopeList{})
}
