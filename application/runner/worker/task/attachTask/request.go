package attachTask

import (
	"github.com/khanzadimahdi/testproject/domain"
)

// defaultShell is what a terminal is opened with when the caller names no
// command. A busybox image has sh but not bash, so sh is the one to reach for.
var defaultShell = []string{"/bin/sh"}

// Request represents a request to run a command inside a task's container.
type Request struct {
	UUID    string   `json:"uuid"`
	Command []string `json:"command"`

	// TTY asks for the command to run under a terminal, which is what an
	// interactive shell needs and what makes its output one stream.
	TTY bool `json:"tty"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.UUID) == 0 {
		validationErrors["uuid"] = "required_field"
	}

	return validationErrors
}

// Shell is the command to run, or an interactive shell when none was named.
func (r *Request) Shell() []string {
	if len(r.Command) > 0 {
		return r.Command
	}

	return defaultShell
}
