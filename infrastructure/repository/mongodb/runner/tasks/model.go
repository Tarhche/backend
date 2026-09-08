package tasks

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/network"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type TaskBson struct {
	UUID           string         `bson:"_id,omitempty"`
	Name           string         `bson:"name"`
	Slug           string         `bson:"slug,omitempty"`
	Kind           string         `bson:"kind,omitempty"`
	StackUUID      string         `bson:"stack_uuid,omitempty"`
	ServiceName    string         `bson:"service_name,omitempty"`
	State          uint           `bson:"state"`
	Image          string         `bson:"image"`
	AutoRemove     bool           `bson:"auto_remove,omitempty"`
	PortBindings   []port.PortMap `bson:"port_bindings,omitempty"`
	ExposedPorts   []port.Port    `bson:"exposed_ports,omitempty"`
	NetworkPolicy  string         `bson:"network_policy,omitempty"`
	Endpoints      []Endpoint     `bson:"endpoints,omitempty"`
	RestartPolicy  string         `bson:"restart_policy,omitempty"`
	RestartCount   uint           `bson:"restart_count,omitempty"`
	HealthCheck    string         `bson:"health_check,omitempty"`
	AttachStdin    bool           `bson:"attach_stdin,omitempty"`
	AttachStdout   bool           `bson:"attach_stdout,omitempty"`
	AttachStderr   bool           `bson:"attach_stderr,omitempty"`
	Environment    []string       `bson:"environment,omitempty"`
	Command        []string       `bson:"command,omitempty"`
	Entrypoint     []string       `bson:"entrypoint,omitempty"`
	WorkingDir     string         `bson:"working_dir,omitempty"`
	Mounts         []Mount        `bson:"mounts,omitempty"`
	ResourceLimits ResourceLimits `bson:"resource_limits,omitempty"`
	NodeName       string         `bson:"node_name,omitempty"`
	ContainerLogs  []byte         `bson:"container_logs,omitempty"`
	ContainerID    string         `bson:"container_id,omitempty"`
	OwnerUUID      string         `bson:"owner_uuid"`
	CreatedAt      time.Time      `bson:"created_at,omitempty"`
	StartedAt      time.Time      `bson:"started_at,omitempty"`
	FinishedAt     time.Time      `bson:"finished_at,omitempty"`
}

type Endpoint struct {
	ContainerPort port.Port `bson:"container_port"`
	Host          string    `bson:"host"`
	HostPort      port.Port `bson:"host_port"`
}

type Mount struct {
	Source   string `bson:"source"`
	Target   string `bson:"target"`
	Type     string `bson:"type"`
	ReadOnly bool   `bson:"read_only"`
}

type ResourceLimits struct {
	Cpu    float64 `bson:"cpu"`
	Memory uint64  `bson:"memory"`
	Disk   uint64  `bson:"disk"`
}

// toTask reads a stored task back. Every read goes through here, so a field
// added to the model reaches the domain from one place.
func toTask(t *TaskBson) task.Task {
	return task.Task{
		UUID:          t.UUID,
		Name:          t.Name,
		Slug:          t.Slug,
		Kind:          kind(t.Kind),
		StackUUID:     t.StackUUID,
		ServiceName:   t.ServiceName,
		State:         task.State(t.State),
		Image:         t.Image,
		AutoRemove:    t.AutoRemove,
		PortBindings:  t.PortBindings,
		ExposedPorts:  t.ExposedPorts,
		NetworkPolicy: networkPolicy(t.NetworkPolicy),
		Endpoints:     toEndpoints(t.Endpoints),
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
		Mounts:        toMounts(t.Mounts),
		ResourceLimits: task.ResourceLimits{
			Cpu:    t.ResourceLimits.Cpu,
			Memory: t.ResourceLimits.Memory,
			Disk:   t.ResourceLimits.Disk,
		},
		NodeName:      t.NodeName,
		ContainerID:   t.ContainerID,
		ContainerLogs: t.ContainerLogs,
		OwnerUUID:     t.OwnerUUID,
		CreatedAt:     t.CreatedAt,
		StartedAt:     t.StartedAt,
		FinishedAt:    t.FinishedAt,
	}
}

// toBson prepares a task to be stored.
func toBson(t *task.Task) TaskBson {
	return TaskBson{
		UUID:          t.UUID,
		Name:          t.Name,
		Slug:          t.Slug,
		Kind:          string(t.Kind),
		StackUUID:     t.StackUUID,
		ServiceName:   t.ServiceName,
		State:         uint(t.State),
		Image:         t.Image,
		AutoRemove:    t.AutoRemove,
		PortBindings:  t.PortBindings,
		ExposedPorts:  t.ExposedPorts,
		NetworkPolicy: string(t.NetworkPolicy),
		Endpoints:     fromEndpoints(t.Endpoints),
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
		Mounts:        fromMounts(t.Mounts),
		ResourceLimits: ResourceLimits{
			Cpu:    t.ResourceLimits.Cpu,
			Memory: t.ResourceLimits.Memory,
			Disk:   t.ResourceLimits.Disk,
		},
		NodeName:      t.NodeName,
		ContainerID:   t.ContainerID,
		ContainerLogs: t.ContainerLogs,
		OwnerUUID:     t.OwnerUUID,
		CreatedAt:     t.CreatedAt,
		StartedAt:     t.StartedAt,
		FinishedAt:    t.FinishedAt,
	}
}

// kind reads a stored kind back. Tasks stored before there were kinds are jobs,
// which is what every one of them was.
func kind(stored string) task.Kind {
	if k := task.Kind(stored); k.IsValid() {
		return k
	}

	return task.DefaultKind
}

// networkPolicy reads a stored policy back. Tasks stored before there were
// policies ran on the default bridge, which is the public one.
func networkPolicy(stored string) network.Policy {
	if p := network.Policy(stored); p.IsValid() {
		return p
	}

	return network.PolicyPublic
}

func toEndpoints(endpoints []Endpoint) []task.Endpoint {
	result := make([]task.Endpoint, len(endpoints))
	for i, e := range endpoints {
		result[i] = task.Endpoint{
			ContainerPort: e.ContainerPort,
			Host:          e.Host,
			HostPort:      e.HostPort,
		}
	}

	return result
}

func fromEndpoints(endpoints []task.Endpoint) []Endpoint {
	result := make([]Endpoint, len(endpoints))
	for i, e := range endpoints {
		result[i] = Endpoint{
			ContainerPort: e.ContainerPort,
			Host:          e.Host,
			HostPort:      e.HostPort,
		}
	}

	return result
}

func toMounts(mounts []Mount) []task.Mount {
	result := make([]task.Mount, len(mounts))
	for i, m := range mounts {
		result[i] = task.Mount{
			Source:   m.Source,
			Target:   m.Target,
			Type:     m.Type,
			ReadOnly: m.ReadOnly,
		}
	}

	return result
}

func fromMounts(mounts []task.Mount) []Mount {
	result := make([]Mount, len(mounts))
	for i, m := range mounts {
		result[i] = Mount{
			Source:   m.Source,
			Target:   m.Target,
			Type:     m.Type,
			ReadOnly: m.ReadOnly,
		}
	}

	return result
}
