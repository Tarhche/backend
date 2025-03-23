package tasks

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

type TaskBson struct {
	UUID           string         `bson:"_id,omitempty"`
	Name           string         `bson:"name"`
	State          uint           `bson:"state"`
	Image          string         `bson:"image"`
	AutoRemove     bool           `bson:"auto_remove,omitempty"`
	PortBindings   []port.PortMap `bson:"port_bindings,omitempty"`
	RestartPolicy  string         `bson:"restart_policy,omitempty"`
	RestartCount   uint           `bson:"restart_count,omitempty"`
	HealthCheck    string         `bson:"health_check,omitempty"`
	AttachStdin    bool           `bson:"attach_stdin,omitempty"`
	AttachStdout   bool           `bson:"attach_stdout,omitempty"`
	AttachStderr   bool           `bson:"attach_stderr,omitempty"`
	Environment    []string       `bson:"environment,omitempty"`
	Command        []string       `bson:"command,omitempty"`
	Entrypoint     []string       `bson:"entrypoint,omitempty"`
	Mounts         []Mount        `bson:"mounts,omitempty"`
	ResourceLimits ResourceLimits `bson:"resource_limits,omitempty"`
	ContainerLogs  []byte         `bson:"container_logs,omitempty"`
	ContainerID    string         `bson:"container_id,omitempty"`
	OwnerUUID      string         `bson:"owner_uuid"`
	CreatedAt      time.Time      `bson:"created_at,omitempty"`
	StartedAt      time.Time      `bson:"started_at,omitempty"`
	FinishedAt     time.Time      `bson:"finished_at,omitempty"`
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
