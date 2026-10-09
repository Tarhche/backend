package answerCodeRun

import (
	"fmt"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
)

type Response struct {
	TaskUUID  string     `json:"task_uuid,omitempty"`
	Name      string     `json:"name"`
	Logs      []byte     `json:"logs"`
	State     string     `json:"state,omitempty"`
	Endpoints []Endpoint `json:"endpoints,omitempty"`
	Deadline  *time.Time `json:"deadline,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// Endpoint is one of the addresses a running snippet answers on.
type Endpoint struct {
	TaskPort uint   `json:"task_port"`
	URL      string `json:"url"`
}

func NewResponse(uuid string, status taskKind.Status, ingressDomain string) *Response {
	response := &Response{
		Name:      status.Run.Name,
		State:     string(status.State),
		TaskUUID:  uuid,
		Endpoints: endpoints(ingressDomain, status.Run, status.State),
		Deadline:  deadline(status.Run, status.State),
	}

	if len(status.Run.Output) > 0 {
		response.Logs = []byte(status.Run.Output)
	}

	return response
}

// deadline is when a snippet being watched will be stopped. Its node sets it
// as the snippet's run comes up and reports it with every beat; a snippet that
// is not running any more has none left to report.
func deadline(run *taskKind.Run, state kind.State) *time.Time {
	if !run.Interactive || state != taskKind.Running || run.Deadline.IsZero() {
		return nil
	}

	at := run.Deadline

	return &at
}

// endpoints are where a running snippet answers: each of its ports that came
// up, under its slug and the workload's domain.
func endpoints(ingressDomain string, run *taskKind.Run, state kind.State) []Endpoint {
	if state != taskKind.Running || len(run.Slug) == 0 {
		return nil
	}

	endpoints := make([]Endpoint, 0, len(run.Endpoints))
	for _, e := range run.Endpoints {
		host := fmt.Sprintf("%s-%d.%s", run.Slug, e.Port, ingressDomain)

		endpoints = append(endpoints, Endpoint{
			TaskPort: uint(e.Port),
			URL:      "http://" + host,
		})
	}

	return endpoints
}
