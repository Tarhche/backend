package runTask

import "github.com/khanzadimahdi/testproject/domain"

// Response represents the response for running a task
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	UUID string `json:"uuid,omitempty"`
}
