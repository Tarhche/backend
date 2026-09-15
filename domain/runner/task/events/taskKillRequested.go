package events

const TaskKillRequestedName = "runnerTaskKillRequested"

type TaskKillRequested struct {
	UUID string `json:"uuid"`
}
