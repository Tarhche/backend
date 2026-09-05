package gettask

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Response represents the response for getting a task
type Response struct {
	UUID          string             `json:"uuid"`
	Name          string             `json:"name"`
	Slug          string             `json:"slug"`
	Kind          string             `json:"kind"`
	CurrentState  string             `json:"current_state"`
	ExpectedState string             `json:"expected_state,omitempty"`
	Image         string             `json:"image"`
	StackUUID     string             `json:"stack_uuid,omitempty"`
	ServiceName   string             `json:"service_name,omitempty"`
	NetworkPolicy string             `json:"network_policy"`
	Endpoints     []EndpointResponse `json:"endpoints"`
	AutoRemove    bool               `json:"auto_remove"`
	RestartPolicy string             `json:"restart_policy"`
	RestartCount  uint               `json:"restart_count"`
	HealthCheck   string             `json:"health_check"`
	AttachStdin   bool               `json:"attach_stdin"`
	AttachStdout  bool               `json:"attach_stdout"`
	AttachStderr  bool               `json:"attach_stderr"`
	Environment   []string           `json:"environment"`
	Command       []string           `json:"command"`
	Entrypoint    []string           `json:"entrypoint"`
	WorkingDir    string             `json:"working_dir"`
	ReadOnly      bool               `json:"read_only"`
	Interactive   bool               `json:"interactive,omitempty"`
	MaxRetries    int                `json:"max_retries"`
	Retries       int                `json:"retries"`
	TTL           time.Duration      `json:"ttl,omitempty"`
	Reason        string             `json:"reason,omitempty"`
	Limits        LimitsResponse     `json:"resource_limits"`
	NodeName      string             `json:"node_name"`
	OwnerUUID     string             `json:"owner_uuid"`
	CreatedAt     time.Time          `json:"created_at"`
	StartedAt     time.Time          `json:"started_at"`
	FinishedAt    time.Time          `json:"finished_at"`
	ContainerID   string             `json:"container_id"`
	ContainerLogs []byte             `json:"container_logs"`
}

// EndpointResponse is one of a container's exposed ports. The node and host
// port behind it are the runner's own business, so only the container port
// leaves it: a caller reaches a port through the ingress, by name.
type EndpointResponse struct {
	ContainerPort uint `json:"container_port"`
}

type LimitsResponse struct {
	Cpu    float64 `json:"cpu"`
	Memory uint64  `json:"memory"`
	Disk   uint64  `json:"disk"`
}

// NewResponse creates a new response from a task
func NewResponse(t task.Task) *Response {
	environment := make([]string, len(t.Environment))
	copy(environment, t.Environment)

	command := make([]string, len(t.Command))
	copy(command, t.Command)

	entrypoint := make([]string, len(t.Entrypoint))
	copy(entrypoint, t.Entrypoint)

	return &Response{
		UUID:          t.UUID,
		Name:          t.Name,
		Slug:          t.Slug,
		Kind:          string(t.Kind),
		CurrentState:  t.CurrentState.String(),
		ExpectedState: t.ExpectedState.String(),
		Image:         t.Image,
		StackUUID:     t.StackUUID,
		ServiceName:   t.ServiceName,
		NetworkPolicy: string(t.NetworkPolicy),
		Endpoints:     NewEndpoints(t),
		AutoRemove:    t.AutoRemove,
		RestartPolicy: t.RestartPolicy,
		RestartCount:  t.RestartCount,
		HealthCheck:   t.HealthCheck,
		AttachStdin:   t.AttachStdin,
		AttachStdout:  t.AttachStdout,
		AttachStderr:  t.AttachStderr,
		Environment:   environment,
		Command:       command,
		Entrypoint:    entrypoint,
		WorkingDir:    t.WorkingDir,
		ReadOnly:      t.ReadOnly,
		Interactive:   t.Interactive,
		MaxRetries:    t.MaxRetries,
		Retries:       t.Retries,
		TTL:           t.TTL,
		Reason:        t.Reason,
		Limits: LimitsResponse{
			Cpu:    t.ResourceLimits.Cpu,
			Memory: t.ResourceLimits.Memory,
			Disk:   t.ResourceLimits.Disk,
		},
		NodeName:      t.NodeName,
		OwnerUUID:     t.OwnerUUID,
		CreatedAt:     t.CreatedAt,
		StartedAt:     t.StartedAt,
		FinishedAt:    t.FinishedAt,
		ContainerID:   t.ContainerID,
		ContainerLogs: t.ContainerLogs,
	}
}

// NewEndpoints reports which of a container's ports are actually reachable.
func NewEndpoints(t task.Task) []EndpointResponse {
	endpoints := make([]EndpointResponse, 0, len(t.Endpoints))
	for _, e := range t.Endpoints {
		if e.HostPort == 0 {
			continue
		}

		endpoints = append(endpoints, EndpointResponse{ContainerPort: uint(e.ContainerPort)})
	}

	return endpoints
}
