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

	corev1 "k8s.io/api/core/v1"

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

// ClusterOperation is the Schema for the clusteroperations API
type ClusterOperation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterOperationSpec   `json:"spec,omitempty"`
	Status ClusterOperationStatus `json:"status,omitempty"`
}

type (
	ActionSource string
	ActionType   string
)

const (
	PlaybookActionType ActionType = "playbook"
	ShellActionType    ActionType = "shell"
)

const (
	BuiltinActionSource   ActionSource = "builtin"
	ConfigMapActionSource ActionSource = "configmap"
)

// ClusterOperationSpec defines the desired state of ClusterOperation
type ClusterOperationSpec struct {
	// Cluster the name of Cluster.kubeonkube.clay.io.
	// +required
	Cluster string `json:"cluster"`
	// HostsConfRef will be filled by operator when it performs backup.
	// +optional
	HostsConfRef *api.ConfigMapRef `json:"hostsConfRef,omitempty"`
	// VarsConfRef will be filled by operator when it performs backup.
	// +optional
	VarsConfRef *api.ConfigMapRef `json:"varsConfRef,omitempty"`
	// SSHAuthRef will be filled by operator when it performs backup.
	// +optional
	SSHAuthRef *api.SecretRef `json:"sshAuthRef,omitempty"`
	// +optional
	// EntrypointSHRef will be filled by operator when it renders entrypoint.sh.
	EntrypointSHRef *api.ConfigMapRef `json:"entrypointSHRef,omitempty"`
	// +required
	ActionType ActionType `json:"actionType"`
	// +required
	Action string `json:"action"`
	// +optional
	// +kubebuilder:default="builtin"
	ActionSource *ActionSource `json:"actionSource"`
	// +optional
	ActionSourceRef *api.ConfigMapRef `json:"actionSourceRef,omitempty"`
	// +optional
	ExtraArgs string `json:"extraArgs"`
	// +required
	Image string `json:"image"`
	// +optional
	PreHook []HookAction `json:"preHook,omitempty"`
	// +optional
	PostHook []HookAction `json:"postHook,omitempty"`
	// +optional
	Resources corev1.ResourceRequirements `json:"resources"`
	// +optional
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty"`
}

func (spec *ClusterOperationSpec) ConfigDataList() []*api.ConfigMapRef {
	result := []*api.ConfigMapRef{spec.HostsConfRef, spec.VarsConfRef, spec.EntrypointSHRef, spec.ActionSourceRef}
	for i := range spec.PreHook {
		result = append(result, spec.PreHook[i].ActionSourceRef)
	}
	for i := range spec.PostHook {
		result = append(result, spec.PostHook[i].ActionSourceRef)
	}
	return result
}

func (spec *ClusterOperationSpec) SecretDataList() []*api.SecretRef {
	return []*api.SecretRef{spec.SSHAuthRef}
}

type HookAction struct {
	// +required
	ActionType ActionType `json:"actionType"`
	// +required
	Action string `json:"action"`
	// +optional
	// +kubebuilder:default="builtin"
	ActionSource *ActionSource `json:"actionSource"`
	// +optional
	ActionSourceRef *api.ConfigMapRef `json:"actionSourceRef,omitempty"`
	// +optional
	ExtraArgs string `json:"extraArgs"`
}

type OpsStatus string

const (
	RunningStatus   OpsStatus = "Running"
	SucceededStatus OpsStatus = "Succeeded"
	FailedStatus    OpsStatus = "Failed"
)

// ClusterOperationStatus defines the observed state of ClusterOperation
type ClusterOperationStatus struct {
	// +optional
	Action string `json:"action"`
	// +optional
	JobRef *api.JobRef `json:"jobRef,omitempty"`
	// +optional
	Status OpsStatus `json:"status"`
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// +optional
	EndTime *metav1.Time `json:"endTime,omitempty"`
	// Digest is used to avoid the change of clusterOps by others. it will be filled by operator. Do Not change this value.
	// +optional
	Digest string `json:"digest,omitempty"`
	// HasModified indicates the spec has been modified by others after created.
	// +optional
	HasModified bool `json:"hasModified,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ClusterOperationList contains a list of ClusterOperation
type ClusterOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterOperation `json:"items"`
}
