package getTask

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type Response struct {
	UUID          string    `json:"uuid"`
	Name          string    `json:"name"`
	State         string    `json:"state"`
	Image         string    `json:"image"`
	ContainerID   string    `json:"container_id"`
	ContainerLogs []byte    `json:"container_logs"`
	CreatedAt     time.Time `json:"created_at"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
}

func NewResponse(t *task.Task) *Response {

	return &Response{
		UUID:          t.UUID,
		Name:          t.Name,
		State:         t.State.String(),
		Image:         t.Image,
		ContainerID:   t.ContainerID,
		ContainerLogs: t.ContainerLogs,
		CreatedAt:     t.CreatedAt,
		StartedAt:     t.StartedAt,
		FinishedAt:    t.FinishedAt,
	}
}
