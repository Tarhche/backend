package runTask

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Request represents a request to create a task
type Request struct {
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
	OwnerUUID      string                 `json:"-"`
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

	if len(r.OwnerUUID) == 0 {
		validationErrors["owner_uuid"] = "required_field"
	}

	return validationErrors
}

// ConvertMounts converts the mounts to task.Mount
func (r *Request) ConvertMounts() []task.Mount {
	result := make([]task.Mount, len(r.Mounts))
	for i, m := range r.Mounts {
		result[i] = task.Mount{
			Source:   m.Source,
			Target:   m.Target,
			Type:     m.Type,
			ReadOnly: m.ReadOnly,
		}
	}

	return result
}

// ConvertPortBindings converts the port bindings to port.PortMap
func (r *Request) ConvertPortBindings() []port.PortMap {
	result := make([]port.PortMap, 0, len(r.PortBindings))
	for containerPort, hostBindings := range r.PortBindings {
		portMap := make(port.PortMap)
		portMap[port.Port(containerPort)] = r.convertPortBinding(hostBindings)
		result = append(result, portMap)
	}

	return result
}

// convertPortBinding converts the port binding to port.PortBinding
func (r *Request) convertPortBinding(bindings []PortBinding) []port.PortBinding {
	result := make([]port.PortBinding, len(bindings))
	for i, binding := range bindings {
		result[i] = port.PortBinding{
			HostIP:   binding.HostIP,
			HostPort: port.Port(binding.HostPort),
		}
	}

	return result
}
