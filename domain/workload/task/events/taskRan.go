package events

import "time"

const TaskRanName = "workloadTaskRan"

type TaskRan struct {
	UUID        string     `json:"uuid"`
	NodeName    string     `json:"node_name"`
	ExecutionID string     `json:"execution_id"`
	Endpoints   []Endpoint `json:"endpoints"`
	StartedAt   time.Time  `json:"started_at"`
	Deadline    time.Time  `json:"deadline"`
}
