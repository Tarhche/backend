package runTask

import (
	"github.com/khanzadimahdi/testproject/domain"
)

// Request represents a request to run a task
type Request struct {
	UUID           string                 `json:"uuid"`
	Name           string                 `json:"name"`
	Image          string                 `json:"image"`
	AutoRemove     bool                   `json:"auto_remove"`
	PortBindings   map[uint][]PortBinding `json:"port_bindings"`
	RestartPolicy  string                 `json:"restart_policy"`
	RestartCount   uint                   `json:"restart_count"`
	HealthCheck    string                 `json:"health_check"`
	AttachStdin    bool                   `json:"attach_stdin"`
	AttachStdout   bool                   `json:"attach_stdout"`
	AttachStderr   bool                   `json:"attach_stderr"`
	Environment    []string               `json:"environment"`
	Command        []string               `json:"command"`
	Entrypoint     []string               `json:"entrypoint"`
	Mounts         []Mount                `json:"mounts"`
	ResourceLimits ResourceLimits         `json:"resource_limits"`
}

// PortBinding represents a host-to-container port binding
type PortBinding struct {
	HostIP   string `json:"host_ip"`
	HostPort uint   `json:"host_port"`
}

// Mount represents a mount point of volume
type Mount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Type     string `json:"type"`
	ReadOnly bool   `json:"read_only"`
}

// ResourceLimits represents the resource limits of the container
type ResourceLimits struct {
	Cpu    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
	Disk   uint64  `json:"disk"`
}

var _ domain.Validatable = &Request{}

// Validate validates the request
func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	if len(r.Name) == 0 {
		validationErrors["name"] = "required_field"
	}

	if len(r.Image) == 0 {
		validationErrors["image"] = "required_field"
	}

	if r.ResourceLimits.Cpu <= 0 {
		validationErrors["resource_limits.cpu"] = "required_field"
	}

	if r.ResourceLimits.Memory <= 0 {
		validationErrors["resource_limits.memory"] = "required_field"
	}

	if r.ResourceLimits.Disk <= 0 {
		validationErrors["resource_limits.disk"] = "required_field"
	}

	return validationErrors
}
