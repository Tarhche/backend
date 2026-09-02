package events

const TaskRestartRequestedName = "runnerTaskRestartRequested"

type TaskRestartRequested struct {
	UUID string `json:"uuid"`
}
