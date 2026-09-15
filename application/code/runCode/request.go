package runCode

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/port"
)

const (
	codeRunnerImageUrl = "ghcr.io/tarhche/code-runner"

	// maximum exposed ports threshold
	maxPorts = 3
)

type Request struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Runner string `json:"runner"`

	// Ports are the ports the code listens on, which the runner publishes and
	// answers for by name while the code is running. A snippet that only prints
	// something names none.
	Ports []port.Port `json:"ports,omitempty"`

	// Terminal asks for a way into the task while the code is running.
	// What is behind it is the same shell the dashboard opens, on a task
	// that holds nothing but this snippet.
	Terminal bool `json:"terminal,omitempty"`
}

var supportedCodeRunners = []string{
	// Go
	"go-1.24",
	"go-1.23",

	// NodeJS
	"nodejs-23.11",
	"nodejs-22.14",
	"nodejs-20.19",

	// PHP
	"php-8.4",
	"php-8.3",

	// nats
	"nats-2.10.0",
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Code) == 0 {
		validationErrors["code"] = "required_field"
	}

	if len(r.Runner) == 0 {
		validationErrors["runner"] = "required_field"
	}

	if !slices.Contains(supportedCodeRunners, r.Runner) {
		validationErrors["runner"] = "invalid_value"
	}

	if len(r.Ports) > maxPorts {
		validationErrors["ports"] = "too_many"
	}

	if slices.Contains(r.Ports, 0) {
		validationErrors["ports"] = "invalid_value"
	}

	return validationErrors
}

func (r *Request) Live() bool {
	return len(r.Ports) > 0 || r.Terminal
}

func (r *Request) Image() string {
	return fmt.Sprintf("%s:%s-latest", codeRunnerImageUrl, r.Runner)
}

// Keepable reports whether the answer to a run is the same answer every time.
//
// A snippet that only prints something prints the same thing whenever the same
// code is run, so that answer is worth keeping. One that serves a port, or that
// somebody is given a way into, is a task to be reached rather than an
// answer to be repeated.
func Keepable(payload []byte) bool {
	var request Request
	if err := json.Unmarshal(payload, &request); err != nil {
		return false
	}

	return !request.Live()
}
