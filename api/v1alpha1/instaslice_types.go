/*
Copyright 2024.

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

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// Mig defines the MIG slice details
type Mig struct {
	Placements     []Placement `json:"placements,omitempty"`
	Profile        string      `json:"profile,omitempty"`
	Giprofileid    int         `json:"giprofileid"`
	CIProfileID    int         `json:"ciProfileid"`
	CIEngProfileID int         `json:"ciengprofileid"`
}

// Placement defines the placement details
type Placement struct {
	Size  int `json:"size"`
	Start int `json:"start"`
}

// AllocationDetails defines the allocation details
type AllocationDetails struct {
	Profile          string `json:"profile"`
	Start            uint32 `json:"start"`
	Size             uint32 `json:"size"`
	PodUUID          string `json:"podUUID"`
	GPUUUID          string `json:"gpuUUID"`
	Nodename         string `json:"nodename"`
	Allocationstatus string `json:"allocationStatus"`
	Giprofileid      int    `json:"giprofileid"`
	CIProfileID      int    `json:"ciProfileid"`
	CIEngProfileID   int    `json:"ciengprofileid"`
	Namespace        string `json:"namespace"`
	PodName          string `json:"podName"`
}

// PreparedDetails defines the prepared details
type PreparedDetails struct {
	Profile string `json:"profile"`
	Start   uint32 `json:"start"`
	Size    uint32 `json:"size"`
	Parent  string `json:"parent"`
	//Do we need POD UID here?
	PodUUID  string `json:"podUUID"`
	Giinfoid uint32 `json:"giinfo"`
	Ciinfoid uint32 `json:"ciinfo"`
}

// InstasliceSpec defines the desired state of Instaslice
type InstasliceSpec struct {
	MigGPUUUID map[string]string `json:"MigGPUUUID,omitempty"`
	// GPUID, Profile, start, podUUID
	Allocations map[string]AllocationDetails `json:"allocations,omitempty"`
	//Prepared :  GPUID, Profile, start
	Prepared     map[string]PreparedDetails `json:"prepared,omitempty"`
	Migplacement []Mig                      `json:"migplacement,omitempty"`
}

// InstasliceStatus defines the observed state of Instaslice
type InstasliceStatus struct {
	Processed string `json:"processed,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Instaslice is the Schema for the instaslices API
type Instaslice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InstasliceSpec   `json:"spec,omitempty"`
	Status InstasliceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// InstasliceList contains a list of Instaslice
type InstasliceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Instaslice `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Instaslice{}, &InstasliceList{})
}
