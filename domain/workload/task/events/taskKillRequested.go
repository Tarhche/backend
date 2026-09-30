package events

const TaskKillRequestedName = "workloadTaskKillRequested"

type TaskKillRequested struct {
	UUID string `json:"uuid"`
}
