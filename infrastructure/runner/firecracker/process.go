package firecracker

import (
	"errors"
	"math"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/image"
)

const (
	mebibyte = 1 << 20

	// what a machine is given when its task names nothing: enough to boot
	// the kernel and the agent and leave the task most of it.
	defaultMemoryMiB = 256
	minimumMemoryMiB = 128

	// defaultScratchBytes is the scratch disk a task that names no disk gets,
	// and minimumScratchBytes the least one is made as, which ext4 needs to
	// be made at all.
	defaultScratchBytes = 1 << 30
	minimumScratchBytes = 64 << 20
)

// errNoCommand is a task whose image names nothing to run, and which names
// nothing itself.
var errNoCommand = errors.New("neither the task nor its image names a command to run")

// resolveProcess is what a machine runs: the task's command as its image and
// the task say it together, the way a container runtime reads them.
//
// An entrypoint the task names replaces the image's, and with it the image's
// command, since the two were written for each other; a command the task names
// replaces the image's command and keeps its entrypoint. Variables the task
// sets are laid over the image's, and where the task starts is the task's to
// say if it says it.
func resolveProcess(config image.Config, execution *task.Execution) (guest.Process, error) {
	entrypoint, command := config.Entrypoint, config.Cmd

	if len(execution.Entrypoint) > 0 {
		entrypoint, command = execution.Entrypoint, nil
	}

	if len(execution.Command) > 0 {
		command = execution.Command
	}

	args := append(append([]string(nil), entrypoint...), command...)
	if len(args) == 0 {
		return guest.Process{}, errNoCommand
	}

	workingDir := config.WorkingDir
	if len(execution.WorkingDirectory) > 0 {
		workingDir = execution.WorkingDirectory
	}

	return guest.Process{
		Args:       args,
		Env:        mergeEnv(config.Env, execution.Environment),
		WorkingDir: workingDir,
		User:       config.User,
	}, nil
}

// mergeEnv lays overrides over base: a variable in both takes its value from
// overrides, and keeps its place in base.
func mergeEnv(base []string, overrides []string) []string {
	merged := make([]string, 0, len(base)+len(overrides))
	index := make(map[string]int, len(base)+len(overrides))

	for _, variable := range append(append([]string(nil), base...), overrides...) {
		name, _, _ := strings.Cut(variable, "=")

		if at, found := index[name]; found {
			merged[at] = variable

			continue
		}

		index[name] = len(merged)
		merged = append(merged, variable)
	}

	return merged
}

// resources is what a machine is given for what its task may use. A share of
// a CPU is a whole CPU used that share of the time, and memory is in bytes.
func resources(limits task.ResourceLimits) (vcpus int, quota float64, memoryMiB int) {
	vcpus = max(1, int(math.Ceil(limits.Cpu)))

	if limits.Cpu > 0 {
		quota = limits.Cpu
	}

	memoryMiB = defaultMemoryMiB
	if limits.Memory > 0 {
		memoryMiB = max(minimumMemoryMiB, int((limits.Memory+mebibyte-1)/mebibyte))
	}

	return vcpus, quota, memoryMiB
}

// scratchBytes is how large a task's scratch disk is made.
func scratchBytes(limits task.ResourceLimits) int64 {
	if limits.Disk == 0 {
		return defaultScratchBytes
	}

	return max(minimumScratchBytes, int64(limits.Disk))
}
