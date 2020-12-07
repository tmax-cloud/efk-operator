/*


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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type InputPlugin struct {
	Path    string `json:"path,omitempty"`
	Pattern string `json:"pattern,omitempty"`
	Tag     string `json:"tag,omitempty"`
}

type FilterPlugin struct {
	ParserName string `json:"parserName,omitempty"`
	Regex      string `json:"regex,omitempty"`
	Tag        string `json:"tag,omitempty"`
}

type OutputPlugin struct {
	IndexName string `json:"indexName,omitempty"`
	Tag       string `json:"tag,omitempty"`
}

// FluentBitConfigurationSpec defines the desired state of FluentBitConfiguration
type FluentBitConfigurationSpec struct {
	InputPlugins []InputPlugin `json:"inputPlugins" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,2,rep,name=inputPlugins"`

	// +optional
	FilterPlugins []FilterPlugin `json:"filterPlugins" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,2,rep,name=filterPlugins"`

	OutputPlugins []OutputPlugin `json:"outputPlugins" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,2,rep,name=outputPlugins"`
}

// FluentBitConfigurationStatus defines the observed state of FluentBitConfiguration
type FluentBitConfigurationStatus struct {
	Phase       string `json:"phase,omitempty"`
	LogRootPath string `json:"logRootPath,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=fbc

// FluentBitConfiguration is the Schema for the fluentbitconfigurations API
type FluentBitConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   FluentBitConfigurationSpec   `json:"spec,omitempty"`
	Status FluentBitConfigurationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// FluentBitConfigurationList contains a list of FluentBitConfiguration
type FluentBitConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []FluentBitConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&FluentBitConfiguration{}, &FluentBitConfigurationList{})
}
