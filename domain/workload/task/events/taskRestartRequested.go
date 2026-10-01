package events

const TaskRestartRequestedName = "workloadTaskRestartRequested"

type TaskRestartRequested struct {
	UUID string `json:"uuid"`
}
