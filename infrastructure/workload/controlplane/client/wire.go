package client

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// the shapes the workload control plane's HTTP API speaks. They live here rather than
// being shared with the control plane's own handlers, because they are this client's
// side of a contract rather than something the domain knows about.

type taskPayload struct {
	UUID          string            `json:"uuid"`
	Name          string            `json:"name"`
	Slug          string            `json:"slug"`
	Kind          string            `json:"kind"`
	Runtime       string            `json:"runtime"`
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
	Deadline      time.Time         `json:"deadline"`
}

type endpointPayload struct {
	TaskPort uint `json:"task_port"`
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
	Runtime       string        `json:"runtime"`
	NodeName      string        `json:"node_name"`
	OwnerUUID     string        `json:"owner_uuid"`
	Services      []taskPayload `json:"services"`
	CreatedAt     time.Time     `json:"created_at"`
}

// runtimesPayload is the control plane's GET /api/runtimes, whose items are
// runtime.Availability as the domain writes it.
type runtimesPayload struct {
	Items []runtime.Availability `json:"items"`
}

type stacksPayload struct {
	Items      []stackPayload    `json:"items"`
	Pagination paginationPayload `json:"pagination"`
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

var streams = map[string]task.Stream{
	"stdout": task.StreamStdout,
	"stderr": task.StreamStderr,
}

func (p *taskPayload) toTask() task.Task {
	endpoints := make([]task.Endpoint, len(p.Endpoints))
	for i, e := range p.Endpoints {
		endpoints[i] = task.Endpoint{TaskPort: port.Port(e.TaskPort)}
	}

	return task.Task{
		UUID:          p.UUID,
		Name:          p.Name,
		Slug:          p.Slug,
		Kind:          task.Kind(p.Kind),
		Runtime:       runtime.Class(p.Runtime).OrSysbox(),
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
		Deadline:   p.Deadline,
	}
}

func (p *stackPayload) toStack() controlPlaneStack {
	services := make([]task.Task, len(p.Services))
	for i := range p.Services {
		services[i] = p.Services[i].toTask()
	}

	return controlPlaneStack{
		UUID:          p.UUID,
		Name:          p.Name,
		Slug:          p.Slug,
		Runtime:       runtime.Class(p.Runtime).OrSysbox(),
		NodeName:      p.NodeName,
		OwnerUUID:     p.OwnerUUID,
		CreatedAt:     p.CreatedAt,
		State:         states[p.State],
		ExpectedState: states[p.ExpectedState],
		Services:      services,
	}
}

func (p *logPayload) toLog(taskUUID string) task.Log {
	return task.Log{
		TaskUUID: taskUUID,
		Stream:   streams[p.Stream],
		Content:  p.Content,
		At:       p.At,
	}
}
