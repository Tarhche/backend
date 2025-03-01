package events

const TaskStoppageRequestedName = "runnerTaskStoppageRequested"

type TaskStoppageRequested struct {
	UUID string `json:"uuid"`
}
