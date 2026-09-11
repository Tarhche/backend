package getRunner

import (
	"github.com/khanzadimahdi/testproject/domain/runner/ingress"
)

type Response struct {
	ID string `json:"id"`
}

func NewResponse(runner *ingress.Runner) *Response {
	return &Response{ID: runner.ID}
}
