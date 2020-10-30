package devfile

import (
	attributes "github.com/devfile/api/v2/pkg/attributes"
)

// DevfileHeader describes the structure of the devfile-specific top-level fields
// that are not part of the K8S API structures
type DevfileHeader struct {
	// Devfile schema version
	// +kubebuilder:validation:Pattern=^([2-9])\.([0-9]+)\.([0-9]+)(\-[0-9a-z-]+(\.[0-9a-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$
	SchemaVersion string `json:"schemaVersion" yaml:"schemaVersion"`

	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	// Optional metadata
	Metadata DevfileMetadata `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

type DevfileMetadata struct {
	// Optional devfile name
	// +optional
	Name string `json:"name,omitempty" yaml:"name,omitempty"`

	// Optional semver-compatible version
	// +optional
	// +kubebuilder:validation:Pattern=^([0-9]+)\.([0-9]+)\.([0-9]+)(\-[0-9a-z-]+(\.[0-9a-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$
	Version string `json:"version,omitempty" yaml:"version,omitempty"`

	// Map of implementation-dependant free-form YAML attributes. Deprecated, use the top-level attributes field instead.
	// +optional
	Attributes attributes.Attributes `json:"attributes,omitempty" yaml:"attributes,omitempty"`

	// Optional devfile display name
	// +optional
	DisplayName string `json:"displayName,omitempty" yaml:"displayName,omitempty"`

	// Optional devfile description
	// +optional
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Optional devfile tags
	// +optional
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// Optional devfile icon, can be a URI or a relative path in the project
	// +optional
	Icon string `json:"icon,omitempty" yaml:"icon,omitempty"`

	// Optional devfile global memory limit
	// +optional
	GlobalMemoryLimit string `json:"globalMemoryLimit,omitempty" yaml:"globalMemoryLimit,omitempty"`

	// Optional devfile project type
	// +optional
	ProjectType string `json:"projectType,omitempty" yaml:"projectType,omitempty"`

	// Optional devfile language
	// +optional
	Language string `json:"language,omitempty" yaml:"language,omitempty"`

	// Optional devfile website
	// +optional
	Website string `json:"website,omitempty" yaml:"website,omitempty"`
}
