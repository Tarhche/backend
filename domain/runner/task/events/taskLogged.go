package events

import "time"

const TaskLoggedName = "runnerTaskLogged"

// TaskLogged carries a batch of lines a task wrote. The worker ships them
// as they are produced and the manager is what stores them, so a line survives
// its task.
type TaskLogged struct {
	UUID        string    `json:"uuid"`
	ExecutionID string    `json:"execution_id"`
	NodeName    string    `json:"node_name"`
	Lines       []LogLine `json:"lines"`
}

type LogLine struct {
	Stream  uint8     `json:"stream"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}
