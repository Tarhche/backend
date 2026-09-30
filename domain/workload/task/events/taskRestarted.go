package events

import "time"

const TaskRestartedName = "workloadTaskRestarted"

type TaskRestarted struct {
	UUID        string    `json:"uuid"`
	NodeName    string    `json:"node_name"`
	ExecutionID string    `json:"execution_id"`
	At          time.Time `json:"at"`
}
