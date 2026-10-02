package vm

import (
	"fmt"
	"math"
	"strings"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// What a VM runs, and how large its machine is, worked out from its spec and
// its image together, the way a container runtime works a container's out
// from the container and its image. Ported from PR #101's runner, which ran
// it end to end.

const (
	mebibyte = 1 << 20

	// DefaultMemory is what a machine is given when its VM names no memory:
	// enough to boot the kernel and the agent and leave the task most of it.
	// A task always names one when it comes from the workload; this is for
	// one that does not.
	DefaultMemory = 256 << 20

	// DefaultScratch is the scratch disk a VM that names no disk gets, and
	// MinScratch the least one is made as, which ext4 needs to be made at all.
	DefaultScratch = 1 << 30
	MinScratch     = 64 << 20
)

// ErrNoCommand is a VM whose image names nothing to run, and which names
// nothing itself. It is an invalid request rather than a failure: nothing
// vmhost could do would give it something to run.
var ErrNoCommand = fmt.Errorf("%w: neither the vm nor its image names a command to run", ErrInvalid)

// ResolveProcess is what a VM runs: its command as its image and its spec say
// it together.
//
// An entrypoint the spec names replaces the image's, and with it the image's
// command, since the two were written for each other; a command the spec
// names replaces the image's command and keeps its entrypoint. Variables the
// spec sets are laid over the image's, and where the task starts is the
// spec's to say if it says it. Who it runs as is the image's: a task names no
// user.
func ResolveProcess(config ImageConfig, spec Spec) (guest.Process, error) {
	entrypoint, command := config.Entrypoint, config.Cmd

	if len(spec.Entrypoint) > 0 {
		entrypoint, command = spec.Entrypoint, nil
	}

	if len(spec.Command) > 0 {
		command = spec.Command
	}

	args := append(append([]string(nil), entrypoint...), command...)
	if len(args) == 0 {
		return guest.Process{}, ErrNoCommand
	}

	workingDir := config.WorkingDir
	if len(spec.WorkingDir) > 0 {
		workingDir = spec.WorkingDir
	}

	return guest.Process{
		Args:       args,
		Env:        MergeEnv(config.Env, spec.Env),
		WorkingDir: workingDir,
		User:       config.User,
	}, nil
}

// MergeEnv lays overrides over base: a variable in both takes its value from
// overrides, and keeps its place in base, so an image's PATH stays first
// however a task changes it.
func MergeEnv(base []string, overrides []string) []string {
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

// MachineSize is the machine a VM is given for what its task may use.
//
// A machine's CPUs are whole ones, so a share of a CPU is a whole one, at
// least one; the share itself is held to by a quota where the hypervisor can
// set one (MachineSpec.CPU). Memory is rounded up to a MiB, and is never less
// than minMemory, which is the least vmhost boots a machine with. A VM that
// names no memory gets DefaultMemory, or minMemory if that is more.
func MachineSize(resources Resources, minMemory uint64) (vcpus int, memoryMiB int) {
	vcpus = max(1, int(math.Ceil(resources.CPU)))

	memory := resources.Memory
	if memory == 0 {
		memory = DefaultMemory
	}

	memory = max(memory, minMemory)

	return vcpus, int((memory + mebibyte - 1) / mebibyte)
}

// ScratchSize is how large a VM's scratch disk is made: as large as the task
// may write, which is what holds it to the disk it names, and never too small
// to be made.
func ScratchSize(resources Resources) uint64 {
	if resources.Disk == 0 {
		return DefaultScratch
	}

	return max(MinScratch, resources.Disk)
}

// MemoryBytes is a machine's memory in bytes.
func MemoryBytes(memoryMiB int) uint64 {
	return uint64(max(0, memoryMiB)) * mebibyte
}
