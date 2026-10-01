package events

import "time"

const TaskDeletedName = "workloadTaskDeleted"

type TaskDeleted struct {
	UUID string    `json:"uuid"`
	At   time.Time `json:"at"`
}
