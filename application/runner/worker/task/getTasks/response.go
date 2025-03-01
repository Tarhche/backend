package gettasks

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type Response struct {
	Items []TaskResponse `json:"items"`
}

type TaskResponse struct {
	UUID       string    `json:"uuid"`
	Name       string    `json:"name"`
	State      string    `json:"state"`
	Image      string    `json:"image"`
	CreatedAt  time.Time `json:"created_at"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

func NewResponse(tasks []task.Task) *Response {
	items := make([]TaskResponse, len(tasks))

	for i, t := range tasks {
		items[i] = TaskResponse{
			UUID:       t.UUID,
			Name:       t.Name,
			State:      t.State.String(),
			Image:      t.Image,
			CreatedAt:  t.CreatedAt,
			StartedAt:  t.StartedAt,
			FinishedAt: t.FinishedAt,
		}
	}

	return &Response{
		Items: items,
	}
}
