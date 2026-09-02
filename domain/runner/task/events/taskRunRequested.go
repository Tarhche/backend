package events

import (
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

const TaskRunRequestedName = "runnerTaskRunRequested"

type TaskRunRequested struct {
	Name           string                 `json:"name"`
	Kind           string                 `json:"kind"`
	StackUUID      string                 `json:"stack_uuid,omitempty"`
	StackSlug      string                 `json:"stack_slug,omitempty"`
	ServiceName    string                 `json:"service_name,omitempty"`
	NominatedNode  string                 `json:"nominated_node,omitempty"`
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
	OwnerUUID      string                 `json:"owner_uuid"`
	Labels         map[string]string      `json:"labels"`
}
