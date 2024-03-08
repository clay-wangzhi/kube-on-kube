/*
Copyright 2024 Clay.

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

package v1alpha1

import (
	"kube-on-kube/api"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope="Cluster"
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=`.metadata.creationTimestamp`,name="Age",type=date

// Cluster is the Schema for the clusters API
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	// HostsConfRef stores hosts.yml.
	// +required
	HostsConfRef *api.ConfigMapRef `json:"hostsConfRef"`
	// VarsConfRef stores group_vars.yml.
	// +required
	VarsConfRef *api.ConfigMapRef `json:"varsConfRef"`
	// KubeConfRef stores cluster kubeconfig.
	// +optional
	KubeConfRef *api.ConfigMapRef `json:"kubeConfRef"`
	// SSHAuthRef stores ssh key and if it is empty ,then use sshpass.
	// +optional
	SSHAuthRef *api.SecretRef `json:"sshAuthRef"`
	// +optional
	PreCheckRef *api.ConfigMapRef `json:"preCheckRef"`
}

func (spec *ClusterSpec) ConfigDataList() []*api.ConfigMapRef {
	return []*api.ConfigMapRef{spec.HostsConfRef, spec.VarsConfRef, spec.KubeConfRef, spec.PreCheckRef}
}

func (spec *ClusterSpec) SecretDataList() []*api.SecretRef {
	return []*api.SecretRef{spec.SSHAuthRef}
}

type ClusterConditionType string

const (
	ClusterConditionCreating ClusterConditionType = "Running"

	ClusterConditionRunning ClusterConditionType = "Succeeded"

	ClusterConditionUpdating ClusterConditionType = "Failed"
)

type ClusterCondition struct {
	// ClusterOps refers to the name of ClusterOperation.
	// +required
	ClusterOps string `json:"clusterOps"`
	// +optional
	Status ClusterConditionType `json:"status"`
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// +optional
	EndTime *metav1.Time `json:"endTime,omitempty"`
}

// ClusterStatus defines the observed state of Cluster
type ClusterStatus struct {
	Conditions []ClusterCondition `json:"conditions"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}
