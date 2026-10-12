// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2 "github.com/crossplane/crossplane/apis/v2/core/v2"
)

// Owner of an App Hub application or workload.
type Owner struct {
	// Email address of the owner.
	Email string `json:"email"`

	// DisplayName of the owner. Defaults to the local part of the email.
	// +kubebuilder:validation:Optional
	DisplayName *string `json:"displayName,omitempty"`
}

// WorkloadAttributes are the App Hub attributes set on every registered workload.
type WorkloadAttributes struct {
	// Environment type: PRODUCTION, STAGING, TEST or DEVELOPMENT.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=PRODUCTION;STAGING;TEST;DEVELOPMENT
	Environment *string `json:"environment,omitempty"`

	// Criticality type: MISSION_CRITICAL, HIGH, MEDIUM or LOW.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum=MISSION_CRITICAL;HIGH;MEDIUM;LOW
	Criticality *string `json:"criticality,omitempty"`

	// +kubebuilder:validation:Optional
	BusinessOwners []Owner `json:"businessOwners,omitempty"`

	// +kubebuilder:validation:Optional
	DeveloperOwners []Owner `json:"developerOwners,omitempty"`

	// +kubebuilder:validation:Optional
	OperatorOwners []Owner `json:"operatorOwners,omitempty"`
}

// WorkloadRegistrationParameters select GKE workloads that App Hub has
// discovered and register them to an application. App Hub assigns discovered
// workloads opaque IDs, so they are selected by cluster, namespace and kind
// rather than named one by one.
type WorkloadRegistrationParameters struct {
	// Project hosting App Hub (the host project). Defaults to the projectID of
	// the ProviderConfig.
	// +kubebuilder:validation:Optional
	Project *string `json:"project,omitempty"`

	// Location of the application, for example us-east4.
	// +kubebuilder:validation:Required
	Location string `json:"location"`

	// ApplicationID of the App Hub application the workloads are registered to.
	// +kubebuilder:validation:Required
	ApplicationID string `json:"applicationId"`

	// Cluster is the name of the GKE cluster whose workloads are selected.
	// +kubebuilder:validation:Required
	Cluster string `json:"cluster"`

	// ClusterProject restricts the selection to workloads in this project
	// (project ID). Useful when several projects have a cluster of the same name.
	// +kubebuilder:validation:Optional
	ClusterProject *string `json:"clusterProject,omitempty"`

	// Namespaces whose workloads are selected.
	// +kubebuilder:validation:MinItems=1
	Namespaces []string `json:"namespaces"`

	// Kinds of workload selected: Deployment, StatefulSet, DaemonSet, CronJob.
	// Defaults to Deployment and StatefulSet.
	// +kubebuilder:validation:Optional
	Kinds []string `json:"kinds,omitempty"`

	// Exclude names workloads (by name, any namespace) that are never selected.
	// +kubebuilder:validation:Optional
	Exclude []string `json:"exclude,omitempty"`

	// Attributes set on each registered workload.
	// +kubebuilder:validation:Optional
	Attributes *WorkloadAttributes `json:"attributes,omitempty"`

	// AdoptDescriptions lists workload descriptions written by earlier tooling.
	// Workloads in the application carrying one of them are taken over by this
	// registration, as if it had created them.
	// +kubebuilder:validation:Optional
	AdoptDescriptions []string `json:"adoptDescriptions,omitempty"`
}

// WorkloadRegistrationObservation is the observed state.
type WorkloadRegistrationObservation struct {
	// Selected is the number of discovered workloads the selector matches.
	Selected int `json:"selected,omitempty"`

	// Registered lists the IDs of the workloads this registration owns.
	Registered []string `json:"registered,omitempty"`

	// Conflicts lists selected workloads already registered elsewhere.
	Conflicts []string `json:"conflicts,omitempty"`
}

// WorkloadRegistrationSpec defines the desired state of WorkloadRegistration.
type WorkloadRegistrationSpec struct {
	v2.ClusterManagedResourceSpec `json:",inline"`
	ForProvider                   WorkloadRegistrationParameters `json:"forProvider"`
}

// WorkloadRegistrationStatus defines the observed state of WorkloadRegistration.
type WorkloadRegistrationStatus struct {
	v2.ManagedResourceStatus `json:",inline"`
	AtProvider               WorkloadRegistrationObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// WorkloadRegistration registers the App Hub discovered workloads of a GKE
// cluster's namespaces to an application, and keeps the registration in step
// as workloads come and go.
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="APPLICATION",type="string",JSONPath=".spec.forProvider.applicationId"
// +kubebuilder:printcolumn:name="SELECTED",type="integer",JSONPath=".status.atProvider.selected"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:resource:scope=Cluster,categories={crossplane,managed,gcpobservability}
type WorkloadRegistration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WorkloadRegistrationSpec   `json:"spec"`
	Status            WorkloadRegistrationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkloadRegistrationList contains a list of WorkloadRegistrations.
type WorkloadRegistrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkloadRegistration `json:"items"`
}

// WorkloadRegistration type metadata.
var (
	WorkloadRegistration_Kind             = reflect.TypeOf(WorkloadRegistration{}).Name()
	WorkloadRegistration_GroupKind        = CRDGroupVersion.WithKind(WorkloadRegistration_Kind).GroupKind()
	WorkloadRegistration_KindAPIVersion   = WorkloadRegistration_Kind + "." + CRDGroupVersion.String()
	WorkloadRegistration_GroupVersionKind = CRDGroupVersion.WithKind(WorkloadRegistration_Kind)
)

func init() {
	SchemeBuilder.Register(&WorkloadRegistration{}, &WorkloadRegistrationList{})
}
