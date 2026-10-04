package runs

import (
	"slices"

	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
)

// resolveArgv works out a run's main process from what it asked for and what
// its image says, by docker's rules rather than microsandbox's:
//
//   - nothing asked for runs the image's ENTRYPOINT and CMD;
//   - a command alone runs the image's ENTRYPOINT with the command, which is
//     how the code runner's snippets are run;
//   - an entrypoint alone runs the entrypoint, and drops the image's CMD;
//   - both run the entrypoint with the command.
//
// Microsandbox would keep the image's CMD under an entrypoint of the run's
// own, where docker drops it, and a task has to run the same thing on either
// runtime. So the supervisor works out the whole of the argv here and the
// sandbox runs exactly that, never microsandbox's default.
func resolveArgv(entrypoint, command []string, image ImageConfig) ([]string, error) {
	var argv []string

	switch {
	case len(entrypoint) == 0 && len(command) == 0:
		argv = slices.Concat(image.Entrypoint, image.Cmd)
	case len(entrypoint) == 0:
		argv = slices.Concat(image.Entrypoint, command)
	case len(command) == 0:
		argv = slices.Clone(entrypoint)
	default:
		argv = slices.Concat(entrypoint, command)
	}

	if len(argv) == 0 || len(argv[0]) == 0 {
		return nil, newError(api.CodeInvalid, "there is nothing to run: the run names no entrypoint or command, and neither does its image")
	}

	return argv, nil
}
