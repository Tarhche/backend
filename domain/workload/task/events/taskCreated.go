package events

import "time"

const TaskCreatedName = "workloadTaskCreated"

type TaskCreated struct {
	UUID string    `json:"uuid"`
	At   time.Time `json:"at"`
}
