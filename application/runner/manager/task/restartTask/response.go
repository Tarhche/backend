package restartTask

import "github.com/khanzadimahdi/testproject/domain"

// Response represents the response for restarting a task
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`
}
