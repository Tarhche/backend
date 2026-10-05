package microsandbox

import (
	"maps"
	"slices"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// mainCommand is how an instance's main process is asked of microsandbox,
// which runs the entrypoint and command its sandbox was made with.
//
// They are worked out as Docker works out a container's: a spec's entrypoint
// replaces the image's, and its command replaces the image's CMD. A spec that
// replaces the entrypoint and gives no command drops the image's CMD too,
// because that CMD was written as arguments to the entrypoint being replaced.
// So the code runner, which names no entrypoint, hands its command to the
// image's own, as a container of the same image would.
type mainCommand struct {
	// entrypoint replaces the image's, when it is not nil.
	entrypoint []string

	// cmd replaces the image's CMD, when it is not nil. An empty one drops it.
	cmd []string
}

func mainCommandOf(spec vm.Spec) mainCommand {
	var command mainCommand

	if len(spec.Entrypoint) > 0 {
		command.entrypoint = slices.Clone(spec.Entrypoint)

		// the image's CMD went with its entrypoint.
		command.cmd = []string{}
	}

	if len(spec.Command) > 0 {
		command.cmd = slices.Clone(spec.Command)
	}

	return command
}

// envOf is a spec's environment, as microsandbox takes one. An entry with no
// '=' is a variable set to nothing, and a later entry of a name wins.
func envOf(env []string) map[string]string {
	if len(env) == 0 {
		return nil
	}

	vars := make(map[string]string, len(env))

	for _, entry := range env {
		name, value, _ := strings.Cut(entry, "=")
		if len(name) == 0 {
			continue
		}

		vars[name] = value
	}

	return vars
}

// execEnv is what a command exec'd into an instance runs with: the instance's
// environment, and the command's own over it. A terminal says what it is when
// the command does not, since a shell draws nothing right without it.
//
// The instance's environment is given again on every command because a
// restored sandbox no longer has it (#1676).
func execEnv(instance []string, options vm.ExecOptions) map[string]string {
	vars := envOf(instance)
	if vars == nil {
		vars = make(map[string]string)
	}

	maps.Copy(vars, envOf(options.Env))

	if _, ok := vars["TERM"]; options.TTY && !ok {
		vars["TERM"] = "xterm-256color"
	}

	if len(vars) == 0 {
		return nil
	}

	return vars
}

// spawnExitCode is the exit code of a command that never started, as a shell
// gives it: 127 for one that is not there, and 126 for one that cannot be run.
func spawnExitCode(kind string) int {
	if kind == "not_found" {
		return 127
	}

	return 126
}
