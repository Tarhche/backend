package runTask

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Request represents a request to run a task
type Request struct {
	UUID string    `json:"uuid"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
	Kind task.Kind `json:"kind"`

	// StackSlug and ServiceName place this container in a stack: the slug
	// names the private network its services share, and the service name is
	// what its neighbours reach it by on that network.
	StackUUID   string `json:"stack_uuid,omitempty"`
	StackSlug   string `json:"stack_slug,omitempty"`
	ServiceName string `json:"service_name,omitempty"`

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
	ReadOnly       bool                   `json:"read_only"`
	Mounts         []Mount                `json:"mounts"`
	ResourceLimits ResourceLimits         `json:"resource_limits"`

	// Attempt is which try this is, counting from zero, and MaxRetries how
	// many the container is worth. The node does not decide either — it hands
	// them back with whatever becomes of this attempt — but a retry is a fresh
	// container rather than the failed one started again.
	Attempt    int `json:"attempt"`
	MaxRetries int `json:"max_retries"`
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

	if !r.Policy().IsValid() {
		validationErrors["network_policy"] = "invalid_network_policy"
	}

	if len(r.ExposedPorts) > 0 && r.Policy().IsValid() && !r.Policy().AllowsPorts() {
		validationErrors["exposed_ports"] = "ports_require_network"
	}

	return validationErrors
}

// TaskKind is the kind this request asked for, or the default when it named
// none.
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

// ContainerName is what the container is called on the node. It is the slug,
// which is unique and is also the name the container's ports are served on, so
// a container is called the same thing wherever it is looked at.
func (r *Request) ContainerName() string {
	if len(r.Slug) > 0 {
		return r.Slug
	}

	return r.Name
}

// PublishedPorts are the bindings the container is created with. Every exposed
// port is published on a host port docker picks, so the runner never has to
// keep track of what is already taken on the node.
func (r *Request) PublishedPorts() port.PortMap {
	bindings := make(port.PortMap, len(r.ExposedPorts))
	for _, p := range r.ExposedPorts {
		bindings[p] = []port.PortBinding{{HostIP: "0.0.0.0"}}
	}

	return bindings
}

// ExposedPortSet is the set of ports the image declares open.
func (r *Request) ExposedPortSet() port.PortSet {
	set := make(port.PortSet, len(r.ExposedPorts))
	for _, p := range r.ExposedPorts {
		set[p] = struct{}{}
	}

	return set
}
