package events

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

const TaskScheduledName = "runnerTaskScheduled"

type TaskScheduled struct {
	UUID          string         `json:"uuid"`
	Name          string         `json:"name"`
	Slug          string         `json:"slug"`
	Kind          string         `json:"kind"`
	StackUUID     string         `json:"stack_uuid,omitempty"`
	StackSlug     string         `json:"stack_slug,omitempty"`
	ServiceName   string         `json:"service_name,omitempty"`
	Image         string         `json:"image"`
	AutoRemove    bool           `json:"auto_remove"`
	PortBindings  []PortMap      `json:"port_bindings"`
	ExposedPorts  []port.Port    `json:"exposed_ports"`
	NetworkPolicy network.Policy `json:"network_policy"`
	RestartPolicy string         `json:"restart_policy"`
	RestartCount  uint           `json:"restart_count"`
	HealthCheck   string         `json:"health_check"`
	AttachStdin   bool           `json:"attach_stdin"`
	AttachStdout  bool           `json:"attach_stdout"`
	AttachStderr  bool           `json:"attach_stderr"`
	Environment   []string       `json:"environment"`
	Command       []string       `json:"command"`
	Entrypoint    []string       `json:"entrypoint"`
	WorkingDir    string         `json:"working_dir"`
	ReadOnly      bool           `json:"read_only"`
	Interactive   bool           `json:"interactive,omitempty"`

	// TTL is how long the container may run for once it is up, in
	// nanoseconds. Zero is no limit.
	TTL            time.Duration  `json:"ttl,omitempty"`
	Mounts         []Mount        `json:"mounts"`
	ResourceLimits ResourceLimits `json:"resource_limits"`
	NominatedNode  string         `json:"nominated_node"`

	// Attempt is which try this is, counting from zero, and MaxRetries how
	// many the container is worth. They travel with the request because that
	// is where the count is kept: the node hands them back with whatever
	// becomes of this attempt, and nothing has to remember them meanwhile.
	Attempt    int `json:"attempt"`
	MaxRetries int `json:"max_retries"`
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

// NewTaskScheduled is a task, as the node that is to run it needs to see it.
//
// It is built in one place because it is asked for in two: when a task is first
// scheduled, and again whenever the runner finds a container that is not what it
// was asked to be.
func NewTaskScheduled(t *task.Task, stackSlug string, nominatedNode string, attempt int) TaskScheduled {
	return TaskScheduled{
		UUID:          t.UUID,
		Name:          t.Name,
		Slug:          t.Slug,
		Kind:          string(t.Kind),
		StackUUID:     t.StackUUID,
		StackSlug:     stackSlug,
		ServiceName:   t.ServiceName,
		Image:         t.Image,
		AutoRemove:    t.AutoRemove,
		PortBindings:  portBindingsOf(t),
		ExposedPorts:  t.ExposedPorts,
		NetworkPolicy: t.NetworkPolicy,
		RestartPolicy: t.RestartPolicy,
		RestartCount:  t.RestartCount,
		HealthCheck:   t.HealthCheck,
		AttachStdin:   t.AttachStdin,
		AttachStdout:  t.AttachStdout,
		AttachStderr:  t.AttachStderr,
		Environment:   t.Environment,
		Command:       t.Command,
		Entrypoint:    t.Entrypoint,
		WorkingDir:    t.WorkingDir,
		ReadOnly:      t.ReadOnly,
		Interactive:   t.Interactive,
		TTL:           t.TTL,
		Mounts:        mountsOf(t),
		ResourceLimits: ResourceLimits{
			Cpu:    t.ResourceLimits.Cpu,
			Memory: t.ResourceLimits.Memory,
			Disk:   t.ResourceLimits.Disk,
		},
		NominatedNode: nominatedNode,
		Attempt:       attempt,
		MaxRetries:    t.MaxRetries,
	}
}

func portBindingsOf(t *task.Task) []PortMap {
	result := make([]PortMap, len(t.PortBindings))
	for i, p := range t.PortBindings {
		portMap := make(PortMap)
		for portNumber, bindings := range p {
			portBindings := make([]PortBinding, len(bindings))
			for j, b := range bindings {
				portBindings[j] = PortBinding{HostIP: b.HostIP, HostPort: b.HostPort}
			}

			portMap[portNumber] = portBindings
		}

		result[i] = portMap
	}

	return result
}

func mountsOf(t *task.Task) []Mount {
	result := make([]Mount, len(t.Mounts))
	for i, m := range t.Mounts {
		result[i] = Mount{
			Source:   m.Source,
			Target:   m.Target,
			Type:     m.Type,
			ReadOnly: m.ReadOnly,
		}
	}

	return result
}
