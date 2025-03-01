package events

const TaskCreatedName = "runnerTaskCreated"

type TaskCreated struct {
	UUID string `json:"uuid"`
}
