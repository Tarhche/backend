package deletetask

import "github.com/khanzadimahdi/testproject/domain"

// Response represents the response for running a task
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`
}
