// MIT License
//
// Copyright (c) 2025 André Jesus and vHive team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package k8s

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "vhive.io", Version: "v1"}
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// addKnownTypes adds the list of known types to the scheme.
// This function is called by the generated code to register the types with the scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&NodeSnapshotCache{},
		&NodeSnapshotCacheList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,path=nodesnapshotcaches,singular=nodesnapshotcache
type NodeSnapshotCache struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec NodeSnapshotCacheSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:generate=true
type NodeSnapshotCacheSpec struct {
	NodeName  string   `json:"nodeName,omitempty"`
	Snapshots []string `json:"snapshots,omitempty"`
}

// +kubebuilder:object:root=true
type NodeSnapshotCacheList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeSnapshotCache `json:"items"`
}
