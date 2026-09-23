package stopTask

import "github.com/khanzadimahdi/testproject/domain"

// Response represents the response for stopping a task
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`
}
