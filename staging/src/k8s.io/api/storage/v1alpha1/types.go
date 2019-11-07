/*
Copyright 2017 The Kubernetes Authors.

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
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeAttachment captures the intent to attach or detach the specified volume
// to/from the specified node.
//
// VolumeAttachment objects are non-namespaced.
type VolumeAttachment struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Specification of the desired attach/detach volume behavior.
	// Populated by the Kubernetes system.
	Spec VolumeAttachmentSpec `json:"spec" protobuf:"bytes,2,opt,name=spec"`

	// Status of the VolumeAttachment request.
	// Populated by the entity completing the attach or detach
	// operation, i.e. the external-attacher.
	// +optional
	Status VolumeAttachmentStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VolumeAttachmentList is a collection of VolumeAttachment objects.
type VolumeAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Items is the list of VolumeAttachments
	Items []VolumeAttachment `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// VolumeAttachmentSpec is the specification of a VolumeAttachment request.
type VolumeAttachmentSpec struct {
	// Attacher indicates the name of the volume driver that MUST handle this
	// request. This is the name returned by GetPluginName().
	Attacher string `json:"attacher" protobuf:"bytes,1,opt,name=attacher"`

	// Source represents the volume that should be attached.
	Source VolumeAttachmentSource `json:"source" protobuf:"bytes,2,opt,name=source"`

	// The node that the volume should be attached to.
	NodeName string `json:"nodeName" protobuf:"bytes,3,opt,name=nodeName"`
}

// VolumeAttachmentSource represents a volume that should be attached.
// Right now only PersistenVolumes can be attached via external attacher,
// in future we may allow also inline volumes in pods.
// Exactly one member can be set.
type VolumeAttachmentSource struct {
	// Name of the persistent volume to attach.
	// +optional
	PersistentVolumeName *string `json:"persistentVolumeName,omitempty" protobuf:"bytes,1,opt,name=persistentVolumeName"`

	// inlineVolumeSpec contains all the information necessary to attach
	// a persistent volume defined by a pod's inline VolumeSource. This field
	// is populated only for the CSIMigration feature. It contains
	// translated fields from a pod's inline VolumeSource to a
	// PersistentVolumeSpec. This field is alpha-level and is only
	// honored by servers that enabled the CSIMigration feature.
	// +optional
	InlineVolumeSpec *v1.PersistentVolumeSpec `json:"inlineVolumeSpec,omitempty" protobuf:"bytes,2,opt,name=inlineVolumeSpec"`
}

// VolumeAttachmentStatus is the status of a VolumeAttachment request.
type VolumeAttachmentStatus struct {
	// Indicates the volume is successfully attached.
	// This field must only be set by the entity completing the attach
	// operation, i.e. the external-attacher.
	Attached bool `json:"attached" protobuf:"varint,1,opt,name=attached"`

	// Upon successful attach, this field is populated with any
	// information returned by the attach operation that must be passed
	// into subsequent WaitForAttach or Mount calls.
	// This field must only be set by the entity completing the attach
	// operation, i.e. the external-attacher.
	// +optional
	AttachmentMetadata map[string]string `json:"attachmentMetadata,omitempty" protobuf:"bytes,2,rep,name=attachmentMetadata"`

	// The last error encountered during attach operation, if any.
	// This field must only be set by the entity completing the attach
	// operation, i.e. the external-attacher.
	// +optional
	AttachError *VolumeError `json:"attachError,omitempty" protobuf:"bytes,3,opt,name=attachError,casttype=VolumeError"`

	// The last error encountered during detach operation, if any.
	// This field must only be set by the entity completing the detach
	// operation, i.e. the external-attacher.
	// +optional
	DetachError *VolumeError `json:"detachError,omitempty" protobuf:"bytes,4,opt,name=detachError,casttype=VolumeError"`
}

// VolumeError captures an error encountered during a volume operation.
type VolumeError struct {
	// Time the error was encountered.
	// +optional
	Time metav1.Time `json:"time,omitempty" protobuf:"bytes,1,opt,name=time"`

	// String detailing the error encountered during Attach or Detach operation.
	// This string maybe logged, so it should not contain sensitive
	// information.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,2,opt,name=message"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CSIStoragePool identifies one particular storage pool and
// stores the corresponding attributes. The spec is read-only.
type CSIStoragePool struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata. The name has no particular meaning and just has to
	// meet the usual requirements (length, characters, unique). To ensure that
	// there are no conflicts with other CSI drivers on the cluster, the recommendation
	// is to use sp-<uuid>.
	//
	// Objects are not namespaced.
	//
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Spec   CSIStoragePoolSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status CSIStoragePoolStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// CSIStoragePoolSpec contains the constant attributes of a CSIStoragePool.
type CSIStoragePoolSpec struct {
	// The CSI driver that provides access to the storage pool.
	// This must be the string returned by the CSI GetPluginName() call.
	DriverName string `json:"driverName" protobuf:"bytes,1,name=driverName"`
}

// CSIStoragePoolStatus contains runtime information about a CSIStoragePool.
//
// A pool might only be accessible from a subset of the nodes in the
// cluster. That subset can be identified either via NodeTopology or
// Nodes, but not both. If neither is set, the pool is assumed
// to be available in the entire cluster.
//
// It is expected to be extended with other
// attributes which do not depend on the storage class, like health of
// the pool. Therefore it has the list of
// `CSIStoragePoolByClass` instances instead of just the capacity
// and the storage class being in the spec.
type CSIStoragePoolStatus struct {
	// NodeTopology can be used to describe a storage pool that is available
	// only for nodes matching certain criteria.
	// +optional
	NodeTopology *v1.NodeSelector `json:"nodeTopology,omitempty" protobuf:"bytes,1,opt,name=nodeTopology"`

	// Nodes can be used to describe a storage pool that is available
	// only for certain nodes in the cluster.
	//
	// +listType=set
	// +optional
	Nodes []string `json:"nodes,omitempty" protobuf:"bytes,2,opt,name=nodes"`

	// Some information, like the actual usable capacity, may
	// depend on the storage class used for volumes.
	//
	// +patchMergeKey=storageClassName
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=storageClassName
	// +optional
	Classes []CSIStoragePoolByClass `patchStrategy:"merge" patchMergeKey:"storageClassName" json:"classes,omitempty" protobuf:"bytes,3,opt,name=classes"`
}

// CSIStoragePoolByClass contains information that applies to one storage
// pool of a CSI driver when using a certain storage class.
type CSIStoragePoolByClass struct {
	// The storage class name matches the name of some actual
	// `StorageClass`, in which case the information applies when
	// using that storage class for a volume. There are also two
	// special names:
	// - <ephemeral> for storage used by ephemeral inline volumes (which
	//   don't use a storage class)
	// - <fallback> for storage that is the same regardless of the storage class;
	//   it is applicable if there is no other, more specific entry
	StorageClassName string `json:"storageClassName" protobuf:"bytes,1,name=storageClassName"`

	// Capacity is the size of the largest volume that currently can
	// be created. This is a best-effort guess and even volumes
	// of that size might not get created successfully.
	// +optional
	Capacity *resource.Quantity `json:"capacity,omitempty" protobuf:"bytes,2,opt,name=capacity"`
}

const (
	// FallbackStorageClassName is used for a CSIStoragePool element which
	// applies when there isn't a more specific element for the
	// current storage class or ephemeral volume.
	FallbackStorageClassName = "<fallback>"

	// EphemeralStorageClassName is used for storage from which
	// ephemeral volumes are allocated.
	EphemeralStorageClassName = "<ephemeral>"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CSIStoragePoolList is a collection of CSIStoragePool objects.
type CSIStoragePoolList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Items is the list of CSIStoragePool objects.
	// +listType=set
	Items []CSIStoragePool `json:"items" protobuf:"bytes,2,rep,name=items"`
}
