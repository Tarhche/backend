package runTask

import (
	"github.com/khanzadimahdi/testproject/application/runner/spec"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Request represents a request to create a task
type Request struct {
	Name string    `json:"name"`
	Kind task.Kind `json:"kind"`

	// StackUUID, StackSlug and ServiceName are set when this container is one
	// service of a stack. NominatedNode is the node the rest of that stack was
	// scheduled onto, because services that share a network share a node.
	StackUUID     string `json:"stack_uuid,omitempty"`
	StackSlug     string `json:"stack_slug,omitempty"`
	ServiceName   string `json:"service_name,omitempty"`
	NominatedNode string `json:"nominated_node,omitempty"`

	Image          string                 `json:"image"`
	AutoRemove     bool                   `json:"auto_remove"`
	PortBindings   map[uint][]PortBinding `json:"port_bindings"`
	ExposedPorts   []port.Port            `json:"exposed_ports"`
	NetworkPolicy  network.Policy         `json:"network_policy"`
	RestartPolicy  string                 `json:"restart_policy"`
	RestartCount   uint                   `json:"restart_count"`
	HealthCheck    string                 `json:"health_check"`
	AttachStdin    bool                   `json:"attach_stdin"`
	AttachStdout   bool                   `json:"attach_stdout"`
	AttachStderr   bool                   `json:"attach_stderr"`
	Environment    []string               `json:"environment"`
	Command        []string               `json:"command"`
	Entrypoint     []string               `json:"entrypoint"`
	WorkingDir     string                 `json:"working_dir"`
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

// FromSpec turns a compose service into a request to run it, filling in the
// limits it did not name from the given defaults.
func FromSpec(name string, service *spec.Service, defaults task.ResourceLimits) *Request {
	limits := service.ResourceLimits(defaults)

	return &Request{
		Name:          name,
		Kind:          task.KindService,
		Image:         service.Image,
		Command:       service.Command,
		Entrypoint:    service.Entrypoint,
		WorkingDir:    service.WorkingDir,
		Environment:   service.Environment,
		ExposedPorts:  service.ExposedPorts(),
		NetworkPolicy: service.NetworkPolicy(),
		RestartPolicy: service.Restart,
		ResourceLimits: ResourceLimits{
			Cpu:    limits.Cpu,
			Memory: limits.Memory,
			Disk:   limits.Disk,
		},
	}
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

	if !r.TaskKind().IsValid() {
		validationErrors["kind"] = "invalid_value"
	}

	if !r.Policy().IsValid() {
		validationErrors["network_policy"] = "invalid_network_policy"
	}

	for _, p := range r.ExposedPorts {
		if p == 0 {
			validationErrors["exposed_ports"] = "invalid_value"

			break
		}
	}

	// a container with no network has nothing to publish a port on.
	if len(r.ExposedPorts) > 0 && r.Policy().IsValid() && !r.Policy().AllowsPorts() {
		validationErrors["exposed_ports"] = "ports_require_network"
	}

	return validationErrors
}

// TaskKind is the kind this request asked for, or the default when it named
// none, which is what every producer of a task did before there were kinds.
func (r *Request) TaskKind() task.Kind {
	if len(r.Kind) == 0 {
		return task.DefaultKind
	}

	return r.Kind
}

// Policy is the network policy this request asked for, or the default when it
// named none.
func (r *Request) Policy() network.Policy {
	if len(r.NetworkPolicy) == 0 {
		return network.DefaultPolicy
	}

	return r.NetworkPolicy
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
