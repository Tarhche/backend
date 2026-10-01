package events

const TaskStoppageRequestedName = "workloadTaskStoppageRequested"

type TaskStoppageRequested struct {
	UUID string `json:"uuid"`
}
