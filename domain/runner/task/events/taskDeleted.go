package events

import "time"

const TaskDeletedName = "runnerTaskDeleted"

type TaskDeleted struct {
	UUID string    `json:"uuid"`
	At   time.Time `json:"at"`
}
