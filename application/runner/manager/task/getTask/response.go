package gettask

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// Response represents the response for getting a task
type Response struct {
	UUID          string    `json:"uuid"`
	Name          string    `json:"name"`
	State         string    `json:"state"`
	Image         string    `json:"image"`
	AutoRemove    bool      `json:"auto_remove"`
	RestartPolicy string    `json:"restart_policy"`
	RestartCount  uint      `json:"restart_count"`
	HealthCheck   string    `json:"health_check"`
	AttachStdin   bool      `json:"attach_stdin"`
	AttachStdout  bool      `json:"attach_stdout"`
	AttachStderr  bool      `json:"attach_stderr"`
	Environment   []string  `json:"environment"`
	Command       []string  `json:"command"`
	Entrypoint    []string  `json:"entrypoint"`
	OwnerUUID     string    `json:"owner_uuid"`
	CreatedAt     time.Time `json:"created_at"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	ContainerID   string    `json:"container_id"`
	ContainerLogs []byte    `json:"container_logs"`
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
		State:         t.State.String(),
		Image:         t.Image,
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
		OwnerUUID:     t.OwnerUUID,
		CreatedAt:     t.CreatedAt,
		StartedAt:     t.StartedAt,
		FinishedAt:    t.FinishedAt,
		ContainerID:   t.ContainerID,
		ContainerLogs: t.ContainerLogs,
	}
}
