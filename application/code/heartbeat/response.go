package heartbeat

import "time"

type Response struct {
	TaskUUID  string     `json:"task_uuid,omitempty"`
	Name      string     `json:"name"`
	Logs      []byte     `json:"logs"`
	State     string     `json:"state,omitempty"`
	Endpoints []Endpoint `json:"endpoints,omitempty"`
	Deadline  *time.Time `json:"deadline,omitempty"`
	Error     string     `json:"error,omitempty"`
}

// Endpoint is one of the addresses a running snippet answers on.
type Endpoint struct {
	TaskPort uint   `json:"task_port"`
	URL      string `json:"url"`
}
