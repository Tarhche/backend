package runCode

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/domain"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
)

const (
	codeRunnerImageUrl = "ghcr.io/tarhche/code-runner"

	// goRunners is what the name of every Go runner starts with.
	goRunners = "go-"

	// maximum exposed ports threshold
	maxPorts = 3
)

type Request struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Runner string `json:"runner"`

	// Ports are the ports the code listens on, which the workload publishes and
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

// ResourceLimits is what a snippet's task is given: the defaults, but for a
// Go snippet, which is built before it runs and needs more memory and disk
// for that than any other snippet does to run.
func (r *Request) ResourceLimits() taskKind.Limits {
	limits := taskKind.Limits{
		CPU:    DefaultMaxCpu,
		Memory: DefaultMaxMemorySize,
		Disk:   DefaultMaxDiskSize,
	}

	if strings.HasPrefix(r.Runner, goRunners) {
		limits.Memory, limits.Disk = GoMaxMemorySize, GoMaxDiskSize
	}

	return limits
}

// Keepable reports whether the answer to a run is worth keeping, and giving
// whoever asks for the same code again while it is kept.
//
// A snippet that only prints something is answered once, with what it
// printed, and the same code asked for again within the hour the answer is
// kept is answered with what it printed then, rather than run again. That is
// not what running it again would always print: a snippet reaches the public
// internet, and one that fetches a page, reads the time or draws a random
// number may print something else every time. The hour is how stale such an
// answer may be, and what it saves is a VM booted for every reader of a page
// whose snippet prints what it printed for the reader before. One that serves
// a port, or that somebody is given a way into, is a task to be reached rather
// than an answer to be repeated, and is never kept.
func Keepable(payload []byte) bool {
	var request Request
	if err := json.Unmarshal(payload, &request); err != nil {
		return false
	}

	return !request.Live()
}
