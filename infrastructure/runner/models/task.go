package models

type TasksDefinition struct {
	Tasks []Task `json:"tasks,omitempty"`
}

type Task struct {
	Name           string    `json:"name,omitempty"`
	Image          string    `json:"runner,omitempty"`
	Command        []string  `json:"command,omitempty"`
	Cleanup        bool      `json:"cleanup,omitempty"`
	Resources      Resources `json:"resources,omitempty"`
	WaitToComplete bool      `json:"wait_to_complete,omitempty"`
}

type Resources struct {
	Limits       ResourceLimits       `json:"limits,omitempty"`
	Reservations ResourceReservations `json:"reservations,omitempty"`
}

type ResourceLimits struct {
	CPUs   string `json:"cpus,omitempty"`
	Memory string `json:"memory,omitempty"`
}

type ResourceReservations struct {
	CPUs   string `json:"cpus,omitempty"`
	Memory string `json:"memory,omitempty"`
}
