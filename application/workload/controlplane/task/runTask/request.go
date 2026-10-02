package runTask

import (
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/spec"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// Request represents a request to create a task
type Request struct {
	Name string    `json:"name"`
	Kind task.Kind `json:"kind"`

	// Runtime is the class the task asks to be run with. Empty is the
	// platform's default, which is resolved and stored on the task when it
	// is created, so that a default changed later never moves it.
	Runtime runtime.Class `json:"runtime,omitempty"`

	// StackUUID, StackSlug and ServiceName are set when this task is one
	// service of a stack. NominatedNode is the node the rest of that stack was
	// scheduled onto, because services that share a network share a node.
	StackUUID     string `json:"stack_uuid,omitempty"`
	StackSlug     string `json:"stack_slug,omitempty"`
	ServiceName   string `json:"service_name,omitempty"`
	NominatedNode string `json:"nominated_node,omitempty"`

	Image         string                 `json:"image"`
	AutoRemove    bool                   `json:"auto_remove"`
	PortBindings  map[uint][]PortBinding `json:"port_bindings"`
	ExposedPorts  []port.Port            `json:"exposed_ports"`
	NetworkPolicy network.Policy         `json:"network_policy"`
	RestartPolicy string                 `json:"restart_policy"`
	RestartCount  uint                   `json:"restart_count"`
	HealthCheck   string                 `json:"health_check"`
	AttachStdin   bool                   `json:"attach_stdin"`
	AttachStdout  bool                   `json:"attach_stdout"`
	AttachStderr  bool                   `json:"attach_stderr"`
	Environment   []string               `json:"environment"`
	Command       []string               `json:"command"`
	Entrypoint    []string               `json:"entrypoint"`
	WorkingDir    string                 `json:"working_dir"`
	ReadOnly      bool                   `json:"read_only"`
	Interactive   bool                   `json:"interactive,omitempty"`

	// MaxRetries is how many times this task is asked for again after it
	// fails, before the workload gives up on it. Nothing at all is whatever its
	// kind is usually worth, zero is not at all, and -1 never gives up.
	MaxRetries *int `json:"max_retries,omitempty"`

	// TTL is how long a job may run for. Zero is no limit, and a service —
	// which is meant to keep running — may not ask for one.
	TTL            time.Duration  `json:"ttl"`
	Mounts         []Mount        `json:"mounts"`
	ResourceLimits ResourceLimits `json:"resource_limits"`
	OwnerUUID      string         `json:"-"`
}

// PortBinding represents a host-to-task port binding
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

// ResourceLimits represents the resource limits of the task. Cpu is in cores,
// and Memory and Disk are in bytes, as they are everywhere in the workload.
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
		Runtime:       service.Runtime,
		Image:         service.Image,
		Command:       service.Command,
		Entrypoint:    service.Entrypoint,
		WorkingDir:    service.WorkingDir,
		ReadOnly:      service.ReadOnly,
		MaxRetries:    service.Deploy.RestartPolicy.MaxAttempts,
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

	// whether the class may be asked for is the platform's to say, which the
	// use case asks it (admission); whether it can be a class at all is
	// something the request can answer for itself.
	if len(r.Runtime) > 0 && !r.Runtime.IsValid() {
		validationErrors["runtime"] = "invalid_value"
	}

	if r.ResourceLimits.Cpu <= 0 {
		validationErrors["resource_limits.cpu"] = "required_field"
	}

	switch {
	case r.ResourceLimits.Memory <= 0:
		validationErrors["resource_limits.memory"] = "required_field"

	// docker will not create a container with less. Every task is asked for
	// through here — the code runner, a configured default and this API as
	// well as a compose service — so this is where it is told so, rather than
	// on whichever node it would have been given to.
	case r.ResourceLimits.Memory < task.MinMemory:
		validationErrors["resource_limits.memory"] = "memory_below_minimum"
	}

	// required so that every task says how much it means to write, although
	// docker does not hold a task to it yet: see Create in
	// infrastructure/workload/container.
	if r.ResourceLimits.Disk <= 0 {
		validationErrors["resource_limits.disk"] = "required_field"
	}

	if len(r.OwnerUUID) == 0 {
		validationErrors["owner_uuid"] = "required_field"
	}

	if !r.TaskKind().IsValid() {
		validationErrors["kind"] = "invalid_value"
	}

	if r.MaxRetries != nil && *r.MaxRetries < task.RetryForever {
		validationErrors["max_retries"] = "invalid_value"
	}

	switch {
	case r.TTL < 0:
		validationErrors["ttl"] = "invalid_value"

	// a service runs until it is stopped, so there is no run for a limit to
	// bound: asking for one is asking for something else.
	case r.TTL > 0 && r.TaskKind() != task.KindJob:
		validationErrors["ttl"] = "ttl_requires_a_job"
	}

	if !r.Policy().IsValid() {
		validationErrors["network_policy"] = "invalid_network_policy"
	}

	if slices.Contains(r.ExposedPorts, 0) {
		validationErrors["exposed_ports"] = "invalid_value"
	}

	// a task with no network has nothing to publish a port on.
	if len(r.ExposedPorts) > 0 && r.Policy().IsValid() && !r.Policy().AllowsPorts() {
		validationErrors["exposed_ports"] = "ports_require_network"
	}

	// nothing applies either of these yet: no runtime mounts a volume into a
	// task or checks on its health. They used to be taken and then dropped on
	// the way to the node, which told whoever asked that something held when
	// nothing did, so they are refused until a runtime can honour them.
	if len(r.Mounts) > 0 {
		validationErrors["mounts"] = "not_supported"
	}

	if len(r.HealthCheck) > 0 {
		validationErrors["health_check"] = "not_supported"
	}

	return validationErrors
}

// Retries is how many times this task is worth asking for again, or what
// its kind is usually worth when it did not say.
func (r *Request) Retries() int {
	if r.MaxRetries == nil {
		return task.DefaultMaxRetries(r.TaskKind())
	}

	return *r.MaxRetries
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
	for taskPort, hostBindings := range r.PortBindings {
		portMap := make(port.PortMap)
		portMap[port.Port(taskPort)] = r.convertPortBinding(hostBindings)
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
