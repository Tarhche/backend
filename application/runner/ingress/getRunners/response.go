package getRunners

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
)

type Response struct {
	Items []RunnerResponse `json:"items"`
}

type RunnerResponse struct {
	ID          string    `json:"id"`
	Connections uint      `json:"connections"`
	ConnectedAt time.Time `json:"connected_at"`
}

func NewResponse(runners []ingress.Runner) *Response {
	items := make([]RunnerResponse, len(runners))

	for i, runner := range runners {
		items[i] = RunnerResponse{
			ID:          runner.ID,
			Connections: runner.Connections,
			ConnectedAt: runner.ConnectedAt,
		}
	}

	return &Response{Items: items}
}
