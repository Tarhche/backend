package events

import "time"

const TaskCreatedName = "runnerTaskCreated"

type TaskCreated struct {
	UUID string    `json:"uuid"`
	At   time.Time `json:"at"`
}
