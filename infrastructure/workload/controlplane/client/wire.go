package client

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
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
	CurrentState  string            `json:"current_state"`
	ExpectedState string            `json:"expected_state"`
	Image         string            `json:"image"`
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
