package api

// Info is what the service says about itself. A client checks APIVersion
// against Version and refuses a service that speaks another.
type Info struct {
	// APIVersion is the major version of this contract the service speaks.
	APIVersion string `json:"api_version"`

	// ServiceVersion is the build of the service answering. It is there for
	// people and logs; nothing decides anything on it.
	ServiceVersion string `json:"service_version"`

	// MicrosandboxVersion is the version of the msb the service runs. It has
	// to equal the SDK's, because the two share one database under MSB_HOME
	// and versions that differ break it, so a service whose two differ is
	// not Ready.
	MicrosandboxVersion string `json:"microsandbox_version"`

	// Ready is whether the service has checked the runtime and adopted
	// again every run it held before it last started. Until it has, a call
	// that creates, starts, stops, kills, restarts or deletes a run is 503
	// unavailable, and Reason says what is missing.
	Ready  bool   `json:"ready"`
	Reason string `json:"reason,omitempty"`

	// Architecture is the service's GOARCH, which is every guest's too: a
	// microVM runs the host's instruction set, so an image with no variant
	// for it cannot run here.
	Architecture string `json:"architecture"`
}
