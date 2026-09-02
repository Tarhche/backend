package events

import (
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

const TaskScheduledName = "runnerTaskScheduled"

type TaskScheduled struct {
	UUID           string         `json:"uuid"`
	Name           string         `json:"name"`
	Slug           string         `json:"slug"`
	Kind           string         `json:"kind"`
	StackUUID      string         `json:"stack_uuid,omitempty"`
	StackSlug      string         `json:"stack_slug,omitempty"`
	ServiceName    string         `json:"service_name,omitempty"`
	Image          string         `json:"image"`
	AutoRemove     bool           `json:"auto_remove"`
	PortBindings   []PortMap      `json:"port_bindings"`
	ExposedPorts   []port.Port    `json:"exposed_ports"`
	NetworkPolicy  network.Policy `json:"network_policy"`
	RestartPolicy  string         `json:"restart_policy"`
	RestartCount   uint           `json:"restart_count"`
	HealthCheck    string         `json:"health_check"`
	AttachStdin    bool           `json:"attach_stdin"`
	AttachStdout   bool           `json:"attach_stdout"`
	AttachStderr   bool           `json:"attach_stderr"`
	Environment    []string       `json:"environment"`
	Command        []string       `json:"command"`
	Entrypoint     []string       `json:"entrypoint"`
	WorkingDir     string         `json:"working_dir"`
	Mounts         []Mount        `json:"mounts"`
	ResourceLimits ResourceLimits `json:"resource_limits"`
	NominatedNode  string         `json:"nominated_node"`
}

type PortBinding struct {
	HostIP   string    `json:"host_ip"`
	HostPort port.Port `json:"host_port"`
}

type PortMap map[port.Port][]PortBinding

type Mount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Type     string `json:"type"`
	ReadOnly bool   `json:"read_only"`
}

type ResourceLimits struct {
	Cpu    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
	Disk   uint64  `json:"disk"`
}

// Endpoint is an exposed container port as the worker actually published it.
type Endpoint struct {
	ContainerPort port.Port `json:"container_port"`
	Host          string    `json:"host"`
	HostPort      port.Port `json:"host_port"`
}
