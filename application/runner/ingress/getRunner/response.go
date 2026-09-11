package getRunner

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
)

type Response struct {
	ID          string    `json:"id"`
	Connections uint      `json:"connections"`
	ConnectedAt time.Time `json:"connected_at"`
}

func NewResponse(runner *ingress.Runner) *Response {
	return &Response{
		ID:          runner.ID,
		Connections: runner.Connections,
		ConnectedAt: runner.ConnectedAt,
	}
}
