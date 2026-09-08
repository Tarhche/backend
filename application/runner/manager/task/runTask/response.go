package runTask

import "github.com/khanzadimahdi/testproject/domain"

// Response represents the response for running a task
type Response struct {
	ValidationErrors domain.ValidationErrors `json:"errors,omitempty"`

	UUID string `json:"uuid,omitempty"`

	// Slug is the name the container is addressed by from outside, which is
	// what the caller needs to build the URL its ports are served on.
	Slug string `json:"slug,omitempty"`
}
