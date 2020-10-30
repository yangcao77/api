package v1alpha2

// Component that allows the developer to declare and configure a volume into their devworkspace
type VolumeComponent struct {
	BaseComponent `json:",inline" yaml:",inline"`
	Volume        `json:",inline" yaml:",inline"`
}

// Volume that should be mounted to a component container
type Volume struct {
	// +optional
	// Size of the volume
	Size string `json:"size,omitempty" yaml:"size,omitempty"`

	// +optional
	// Ephemeral volumes are not stored persistently across restarts. Defaults
	// to false
	Ephemeral bool `json:"ephemeral,omitempty" yaml:"ephemeral,omitempty"`
}
