package container

import (
	"io"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/stats"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Container represents a container specification
type Container struct {
	ID               string
	Name             string
	Status           Status
	Image            string
	ResourceLimits   ResourceLimits
	RestartPolicy    string
	RestartCount     uint
	WorkingDirectory string
	ExposedPorts     port.PortSet
	PortBindings     port.PortMap
	HealthCheck      string
	AutoRemove       bool
	Environment      []string
	Entrypoint       []string
	Command          []string
	Labels           map[string]string
	CreatedAt        time.Time
}

// ResourceLimits represents the resource limits of the container
type ResourceLimits struct {
	Cpu    float64
	Memory uint64
	Disk   uint64
}

// Manager represents a manager of containers
type Manager interface {
	GetAll() ([]Container, error)
	GetByLabel(labelName string, labelValue string) ([]Container, error)
	Create(container *Container) (containerUUID string, err error)
	Start(containerUUID string) error
	Stop(containerUUID string) error
	Delete(containerUUID string) error
	Inspect(containerUUID string) (Container, error)
	Stats(containerUUID string) (stats.Stats, error)
	Logs(containerUUID string, writer io.Writer) error
	EvaluateTaskState(status Status) task.State
}

const (
	TaskUUIDLabelKey = "task.uuid" // The UUID of the task that the container is running
	TaskNameLabelKey = "task.name" // The name of the task that the container is running
	NodeNameLabelKey = "node.name" // the name of the node that manages the container.
)
