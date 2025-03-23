package events

import "time"

const TaskStoppedName = "runnerTaskStopped"

type TaskStopped struct {
	UUID     string    `json:"uuid"`
	NodeName string    `json:"node_name"`
	At       time.Time `json:"at"`
}
