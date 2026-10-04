package vmhost

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Create makes a VM: its image ready, its record, and its scratch disk. It
// boots nothing, as docker's create starts nothing: Start does.
//
// Room is taken when a VM is made rather than when it is booted, since the
// orchestrator makes a VM to boot it straight away: a VM that would not fit is
// refused here, with vm.ErrCapacity, while the task can still be placed
// elsewhere. A VM holds its memory and its CPUs from being made until its
// task ends, its scratch disk until it is deleted, and its machine's host user
// for as long as it is held.
func (e *Engine) Create(ctx context.Context, spec vm.Spec) (string, error) {
	image, err := e.images.Ensure(ctx, spec.Image)
	if err != nil {
		return "", err
	}

	process, err := vm.ResolveProcess(image.Config, spec)
	if err != nil {
		return "", err
	}

	vcpus, memoryMiB := vm.MachineSize(spec.Resources, e.config.MinMemory)

	var scratch uint64
	if !spec.ReadOnly {
		scratch = vm.ScratchSize(spec.Resources)
	}

	made := vm.VM{
		Spec:        spec,
		State:       vm.StateCreated,
		ImageDigest: image.Digest,
		Process:     process,
		VCPUs:       vcpus,
		MemoryMiB:   memoryMiB,
	}

	id, err := e.reserve(ctx, made, scratch)
	if err != nil {
		return "", err
	}

	unlock := e.locks.lock(id)
	defer unlock()

	if !spec.ReadOnly {
		if err := e.images.MakeScratch(ctx, e.config.ScratchPath(id), scratch); err != nil {
			return "", errors.Join(err, e.states.Remove(context.WithoutCancel(ctx), id))
		}
	}

	e.logger.InfoContext(ctx, "vm created", "vm", id, "name", spec.Name, "image", spec.Image, "digest", image.Digest)

	return id, nil
}

// reserve writes a VM down, once nothing it asks for clashes with what is
// held and there is room for it: a name of its own, memory, CPUs and disk
// within the budget, and a host user to run as. It says what the VM is called.
func (e *Engine) reserve(ctx context.Context, made vm.VM, scratch uint64) (string, error) {
	e.admission.Lock()
	defer e.admission.Unlock()

	all, err := e.states.All(ctx)
	if err != nil {
		return "", err
	}

	for _, other := range all {
		if other.Spec.Name == made.Spec.Name {
			return "", fmt.Errorf("%w: a vm called %s is already there (%s)", vm.ErrConflict, made.Spec.Name, other.ID)
		}
	}

	if err := e.admit(ctx, all, "", made.MemoryMiB, made.VCPUs, scratch); err != nil {
		return "", err
	}

	uid, err := e.allocateUID(ctx, all)
	if err != nil {
		return "", err
	}

	id, err := newID(all)
	if err != nil {
		return "", err
	}

	made.ID = id
	made.UID = uid
	made.CreatedAt = time.Now().UTC()

	if err := e.states.Put(ctx, made); err != nil {
		return "", errors.Join(err, e.states.Remove(context.WithoutCancel(ctx), id))
	}

	return id, nil
}

// holdsRoom reports whether a VM in state holds its memory and its CPUs: from
// being made until its task ends, including while it waits to be booted
// again, so that nothing takes its room meanwhile.
func holdsRoom(state vm.State) bool {
	return state == vm.StateCreated || state.Up()
}

// admit refuses a VM there is no room for, with vm.ErrCapacity: its memory,
// with its VMM's overhead, past the memory budget; its CPUs past the host's
// times the overcommit; or its scratch disk past what the disk may be
// promised, or into the disk vmhost keeps free. The VM named except, if any, is
// not counted, since it is the one being admitted. The admission lock is held.
func (e *Engine) admit(ctx context.Context, all []vm.VM, except string, memoryMiB int, vcpus int, scratch uint64) error {
	var (
		memory   uint64
		cpus     int
		promised uint64
	)

	for _, other := range all {
		if other.ID == except {
			continue
		}

		if holdsRoom(other.State) {
			memory += vm.MemoryBytes(other.MemoryMiB) + e.config.MemoryOverhead
			cpus += other.VCPUs
		}

		if !other.Spec.ReadOnly && other.State != vm.StateRemoving {
			promised += vm.ScratchSize(other.Spec.Resources)
		}
	}

	need := vm.MemoryBytes(memoryMiB) + e.config.MemoryOverhead
	if memory+need > e.config.MaxMemory {
		return e.refuse(ctx, "memory", "%d MiB of memory, with its VMM's, is past vmhost's budget of %d MiB, %d MiB of which are taken",
			need>>20, e.config.MaxMemory>>20, memory>>20)
	}

	if e.config.HostCPUs > 0 && e.config.CPUOvercommit > 0 {
		budget := float64(e.config.HostCPUs) * e.config.CPUOvercommit
		if float64(cpus+vcpus) > budget {
			return e.refuse(ctx, "cpu", "%d CPUs are past vmhost's budget of %g, %d of which are taken", vcpus, budget, cpus)
		}
	}

	if scratch == 0 || e.freeSpace == nil {
		return nil
	}

	free, err := e.freeSpace()
	if err != nil {
		return fmt.Errorf("the free disk cannot be told: %w", err)
	}

	if free <= e.config.DiskReserve {
		return e.refuse(ctx, "disk", "only %d MiB of disk are free, which vmhost keeps", free>>20)
	}

	if e.config.DiskOvercommit > 0 {
		budget := float64(free) * e.config.DiskOvercommit
		if float64(promised)+float64(scratch) > budget {
			return e.refuse(ctx, "disk", "a scratch disk of %d MiB is past the %d MiB scratch disks may be promised, %d MiB of which are",
				scratch>>20, uint64(budget)>>20, promised>>20)
		}
	}

	return nil
}

// refuse is a VM there is no room for, counted by what it ran out of.
func (e *Engine) refuse(ctx context.Context, resource string, format string, arguments ...any) error {
	e.metrics.refused(ctx, resource)

	return fmt.Errorf("%w: "+format, append([]any{vm.ErrCapacity}, arguments...)...)
}

// allocateUID is the host user a new VM's machine runs as: the lowest of the
// range no VM holds. Every VM holds its user for as long as it is held, since
// its scratch disk is that user's.
func (e *Engine) allocateUID(ctx context.Context, all []vm.VM) (int, error) {
	if e.config.UIDs <= 0 {
		return 0, nil
	}

	taken := make(map[int]bool, len(all))
	for _, v := range all {
		taken[v.UID] = true
	}

	for uid := e.config.FirstUID; uid < e.config.FirstUID+e.config.UIDs; uid++ {
		if !taken[uid] {
			return uid, nil
		}
	}

	return 0, e.refuse(ctx, "uids", "all %d host users machines run as are taken", e.config.UIDs)
}

// newID names a VM: sixteen hex digits no VM is called yet.
func newID(all []vm.VM) (string, error) {
	for {
		random := make([]byte, 8)
		if _, err := rand.Read(random); err != nil {
			return "", err
		}

		id := hex.EncodeToString(random)

		if !slices.ContainsFunc(all, func(v vm.VM) bool { return v.ID == id }) {
			return id, nil
		}
	}
}
