package events

import "time"

const TaskRanName = "workloadTaskRan"

type TaskRan struct {
	UUID        string     `json:"uuid"`
	NodeName    string     `json:"node_name"`
	ExecutionID string     `json:"execution_id"`
	Endpoints   []Endpoint `json:"endpoints"`

	// StartedAt is when the task's run started, as the node holding it
	// says: the moment its main process did. It is zero when the node did not
	// say.
	StartedAt time.Time `json:"started_at"`
	Deadline  time.Time `json:"deadline"`
}
