package events

const TaskDeletedName = "runnerTaskDeleted"

type TaskDeleted struct {
	UUID string `json:"uuid"`
}
