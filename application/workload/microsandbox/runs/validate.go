package runs

import (
	"fmt"
	"math"
	"path"
	"strings"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

const (
	// maxNameLength is the longest name a run may be given, in bytes.
	maxNameLength = 128

	// maxPorts is the most ports one run may publish.
	maxPorts = 64
)

// validateSpec says whether a run can be created as it was asked for.
//
// A spec that does not hold is invalid, with every problem found rather than
// the first, so that a task that cannot run says everything that is wrong with
// it at once. One that holds but asks for what microsandbox cannot do is
// not_supported, which is what the orchestrator should have refused already.
func validateSpec(spec api.RunSpec) error {
	var problems []string

	if len(spec.Node) == 0 {
		problems = append(problems, "node is required")
	}

	switch {
	case len(spec.Name) == 0:
		problems = append(problems, "name is required")
	case len(spec.Name) > maxNameLength:
		problems = append(problems, fmt.Sprintf("name is longer than %d bytes", maxNameLength))
	}

	if len(strings.TrimSpace(spec.Image)) == 0 {
		problems = append(problems, "image is required")
	}

	if spec.Memory == 0 {
		problems = append(problems, "memory has to be more than 0 bytes")
	}

	if spec.CPU < 0 || math.IsNaN(spec.CPU) || math.IsInf(spec.CPU, 0) {
		problems = append(problems, "cpu has to be a number of cores, 0 or more")
	}

	problems = append(problems, portProblems(spec.Ports)...)
	problems = append(problems, environmentProblems(spec.Environment)...)

	if len(spec.WorkingDir) > 0 && !path.IsAbs(spec.WorkingDir) {
		problems = append(problems, fmt.Sprintf("working_dir %q is not an absolute path", spec.WorkingDir))
	}

	if _, err := parsePolicy(spec.RestartPolicy); err != nil {
		problems = append(problems, err.Error())
	}

	unsupported := false

	switch spec.Network {
	case api.NetworkIsolated, api.NetworkPublic:
	case "":
		problems = append(problems, "network is required")
	case "none":
		unsupported = true
	default:
		problems = append(problems, fmt.Sprintf("network %q is neither isolated nor public", spec.Network))
	}

	if len(problems) > 0 {
		return newError(api.CodeInvalid, "%s", strings.Join(problems, "; "))
	}

	if unsupported {
		return newError(api.CodeNotSupported, "microsandbox cannot run a task with no network interface")
	}

	return nil
}

// validateExec says whether a command can be run inside a run as it was asked
// for.
func validateExec(request api.ExecRequest) error {
	var problems []string

	if len(request.Command) == 0 || len(request.Command[0]) == 0 {
		problems = append(problems, "command is required")
	}

	problems = append(problems, environmentProblems(request.Env)...)

	if len(request.WorkDir) > 0 && !path.IsAbs(request.WorkDir) {
		problems = append(problems, fmt.Sprintf("workdir %q is not an absolute path", request.WorkDir))
	}

	if len(problems) > 0 {
		return newError(api.CodeInvalid, "%s", strings.Join(problems, "; "))
	}

	return nil
}

func portProblems(ports []uint16) []string {
	var problems []string

	if len(ports) > maxPorts {
		problems = append(problems, fmt.Sprintf("at most %d ports may be published", maxPorts))
	}

	seen := make(map[uint16]bool, len(ports))
	for _, port := range ports {
		switch {
		case port == 0:
			problems = append(problems, "port 0 cannot be published")
		case seen[port]:
			problems = append(problems, fmt.Sprintf("port %d is published twice", port))
		}

		seen[port] = true
	}

	return problems
}

// environmentProblems checks entries are KEY=VALUE, and that none holds a tab,
// which microsandbox's guest refuses, or a NUL, which no environment can carry.
func environmentProblems(environment []string) []string {
	var problems []string

	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")

		switch {
		case !found || len(key) == 0:
			problems = append(problems, fmt.Sprintf("environment entry %q is not KEY=VALUE", entry))
		case strings.ContainsRune(entry, '\t'):
			problems = append(problems, fmt.Sprintf("environment entry %s holds a tab", key))
		case strings.ContainsRune(entry, 0):
			problems = append(problems, fmt.Sprintf("environment entry %s holds a NUL", key))
		}
	}

	return problems
}

// environmentMap is a run's environment as microsandbox takes it. Where a key
// is given twice the later one wins, as it does for docker.
func environmentMap(environment []string) map[string]string {
	values := make(map[string]string, len(environment))

	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		values[key] = value
	}

	return values
}
