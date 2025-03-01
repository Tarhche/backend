package events

import (
	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

const TaskScheduledName = "runnerTaskScheduled"

type TaskScheduled struct {
	UUID           string         `json:"uuid"`
	Name           string         `json:"name"`
	Image          string         `json:"image"`
	AutoRemove     bool           `json:"auto_remove"`
	PortBindings   []PortMap      `json:"port_bindings"`
	RestartPolicy  string         `json:"restart_policy"`
	RestartCount   uint           `json:"restart_count"`
	HealthCheck    string         `json:"health_check"`
	AttachStdin    bool           `json:"attach_stdin"`
	AttachStdout   bool           `json:"attach_stdout"`
	AttachStderr   bool           `json:"attach_stderr"`
	Environment    []string       `json:"environment"`
	Command        []string       `json:"command"`
	Entrypoint     []string       `json:"entrypoint"`
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
