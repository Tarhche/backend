package events

const TaskStoppedName = "runnerTaskStopped"

type TaskStopped struct {
	UUID     string `json:"uuid"`
	NodeName string `json:"node_name"`
}
