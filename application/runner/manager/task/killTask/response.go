package killTask

import "github.com/khanzadimahdi/testproject/domain"

// Response represents the response for killing a task
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`
}
