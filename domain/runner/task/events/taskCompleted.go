package events

const TaskCompletedName = "runnerTaskCompleted"

type TaskCompleted struct {
	UUID     string `json:"uuid"`
	NodeName string `json:"node_name"`
}
