package client

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// the shapes the runner manager's HTTP API speaks. They live here rather than
// being shared with the manager's own handlers, because they are this client's
// side of a contract rather than something the domain knows about.

type taskPayload struct {
	UUID          string            `json:"uuid"`
	Name          string            `json:"name"`
	Slug          string            `json:"slug"`
	Kind          string            `json:"kind"`
	CurrentState  string            `json:"current_state"`
	ExpectedState string            `json:"expected_state"`
	Image         string            `json:"image"`
	StackUUID     string            `json:"stack_uuid"`
	ServiceName   string            `json:"service_name"`
	Network       string            `json:"network_policy"`
	Endpoints     []endpointPayload `json:"endpoints"`
	Environment   []string          `json:"environment"`
	Command       []string          `json:"command"`
	Entrypoint    []string          `json:"entrypoint"`
	WorkingDir    string            `json:"working_dir"`
	ReadOnly      bool              `json:"read_only"`
	MaxRetries    int               `json:"max_retries"`
	Retries       int               `json:"retries"`
	Reason        string            `json:"reason"`
	Limits        limitsPayload     `json:"resource_limits"`
	NodeName      string            `json:"node_name"`
	OwnerUUID     string            `json:"owner_uuid"`
	CreatedAt     time.Time         `json:"created_at"`
	StartedAt     time.Time         `json:"started_at"`
	FinishedAt    time.Time         `json:"finished_at"`
}

type endpointPayload struct {
	ContainerPort uint `json:"container_port"`
}

type limitsPayload struct {
	Cpu    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
	Disk   uint64  `json:"disk"`
}

type paginationPayload struct {
	TotalPages  uint `json:"total_pages"`
	CurrentPage uint `json:"current_page"`
}

type tasksPayload struct {
	Items      []taskPayload     `json:"items"`
	Pagination paginationPayload `json:"pagination"`
}

type stackPayload struct {
	UUID          string        `json:"uuid"`
	Name          string        `json:"name"`
	Slug          string        `json:"slug"`
	State         string        `json:"state"`
	ExpectedState string        `json:"expected_state"`
	NodeName      string        `json:"node_name"`
	OwnerUUID     string        `json:"owner_uuid"`
	Services      []taskPayload `json:"services"`
	CreatedAt     time.Time     `json:"created_at"`
}

type stacksPayload struct {
	Items      []stackPayload    `json:"items"`
	Pagination paginationPayload `json:"pagination"`
}

// changePayload is one message of a container watch: "changed" carries the
// container as it is now, "deleted" only the uuid of one that is gone.
type changePayload struct {
	Kind string       `json:"kind"`
	UUID string       `json:"uuid"`
	Task *taskPayload `json:"task,omitempty"`
}

// stackChangePayload is one message of a stack watch.
type stackChangePayload struct {
	Kind  string        `json:"kind"`
	UUID  string        `json:"uuid"`
	Stack *stackPayload `json:"stack,omitempty"`
}

type logsPayload struct {
	Items []logPayload `json:"items"`
}

type logPayload struct {
	Stream  string    `json:"stream"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}

// states maps the words the API uses back onto the domain's own states.
var states = map[string]task.State{
	"created":    task.Created,
	"scheduled":  task.Scheduled,
	"running":    task.Running,
	"stopping":   task.Stopping,
	"stopped":    task.Stopped,
	"completed":  task.Completed,
	"failed":     task.Failed,
	"restarting": task.Restarting,
}

var streams = map[string]container.Stream{
	"stdout": container.StreamStdout,
	"stderr": container.StreamStderr,
}

func (p *taskPayload) toTask() task.Task {
	endpoints := make([]task.Endpoint, len(p.Endpoints))
	for i, e := range p.Endpoints {
		endpoints[i] = task.Endpoint{ContainerPort: port.Port(e.ContainerPort)}
	}

	return task.Task{
		UUID:          p.UUID,
		Name:          p.Name,
		Slug:          p.Slug,
		Kind:          task.Kind(p.Kind),
		StackUUID:     p.StackUUID,
		ServiceName:   p.ServiceName,
		CurrentState:  states[p.CurrentState],
		ExpectedState: states[p.ExpectedState],
		Image:         p.Image,
		Endpoints:     endpoints,
		Environment:   p.Environment,
		Command:       p.Command,
		Entrypoint:    p.Entrypoint,
		WorkingDir:    p.WorkingDir,
		ReadOnly:      p.ReadOnly,
		MaxRetries:    p.MaxRetries,
		Retries:       p.Retries,
		Reason:        p.Reason,
		ResourceLimits: task.ResourceLimits{
			Cpu:    p.Limits.Cpu,
			Memory: p.Limits.Memory,
			Disk:   p.Limits.Disk,
		},
		NodeName:   p.NodeName,
		OwnerUUID:  p.OwnerUUID,
		CreatedAt:  p.CreatedAt,
		StartedAt:  p.StartedAt,
		FinishedAt: p.FinishedAt,
	}
}

func (p *stackPayload) toStack() managerStack {
	services := make([]task.Task, len(p.Services))
	for i := range p.Services {
		services[i] = p.Services[i].toTask()
	}

	return managerStack{
		Stack: stack.Stack{
			UUID:      p.UUID,
			Name:      p.Name,
			Slug:      p.Slug,
			NodeName:  p.NodeName,
			OwnerUUID: p.OwnerUUID,
			CreatedAt: p.CreatedAt,
		},
		State:         states[p.State],
		ExpectedState: states[p.ExpectedState],
		Services:      services,
	}
}

func (p *changePayload) toChange() managerContainerChange {
	change := managerContainerChange{
		UUID:    p.UUID,
		Deleted: p.Kind == "deleted",
	}

	if p.Task != nil {
		change.Container = p.Task.toTask()
	}

	return change
}

func (p *stackChangePayload) toChange() managerStackChange {
	change := managerStackChange{
		UUID:    p.UUID,
		Deleted: p.Kind == "deleted",
	}

	if p.Stack != nil {
		change.Stack = p.Stack.toStack()
	}

	return change
}

func (p *logPayload) toLog(taskUUID string) container.Log {
	return container.Log{
		TaskUUID: taskUUID,
		LogLine: container.LogLine{
			Stream:  streams[p.Stream],
			Content: p.Content,
			At:      p.At,
		},
	}
}
