/*
Copyright 2022.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	BackupNotStarted = "NotStarted"
	BackupInProgress = "InProgress"
	BackupCompleted  = "Completed"
	BackupError      = "Error"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PVCBackupSpec defines the desired state of PVCBackup
type PVCBackupSpec struct {
	// The namespace where the the source PVC is located
	//+kubebuilder:validation:Required
	SourceNamespace string `json:"sourceNamespace"`
	// Source PVC name
	//+kubebuilder:validation:Required
	SourcePVC string `json:"sourcePVC"`
	// The namespace where the the destination PVC is created
	//+kubebuilder:validation:Required
	DestinationNamespace string `json:"destinationNamespace"`
	// Destination PVC name
	//+kubebuilder:validation:Required
	DestinationPVC string `json:"destinationPVC"`
}

// PVCBackupStatus defines the observed state of PVCBackup
type PVCBackupStatus struct {
	// PVC backup status
	BackupStatus string `json:"backupStatus,omitempty"`
	// The date and time when the backup has been completed
	BackupDateAndTime string `json:"backupDateAndTime,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="SOURCE_NAMESPACE",type="string",JSONPath=".spec.sourceNamespace"
//+kubebuilder:printcolumn:name="SOURCE_PVC",type="string",JSONPath=".spec.sourcePVC"
//+kubebuilder:printcolumn:name="DESTINATION_NAMESPACE",type="string",JSONPath=".spec.destinationNamespace"
//+kubebuilder:printcolumn:name="DESTINATION_PVC",type="string",JSONPath=".spec.destinationPVC"
//+kubebuilder:printcolumn:name="BACKUP_STATUS",type="string",JSONPath=".status.backupStatus"

// PVCBackup is the Schema for the pvcbackups API
type PVCBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PVCBackupSpec   `json:"spec,omitempty"`
	Status PVCBackupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PVCBackupList contains a list of PVCBackup
type PVCBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PVCBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PVCBackup{}, &PVCBackupList{})
}
