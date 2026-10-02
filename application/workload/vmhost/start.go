package vmhost

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Start boots a VM and runs its task in it. Starting a VM that runs is
// starting nothing, as it is for a container.
func (e *Engine) Start(ctx context.Context, id string) error {
	unlock := e.locks.lock(id)
	defer unlock()

	v, err := e.states.Get(ctx, id)
	if err != nil {
		return err
	}

	// a VM that cannot be booted is left as it was found: one that had
	// ended has still ended, and one waiting to be booted again by its
	// restart policy, which this boot stands in for, is dead.
	fallback := v.State
	if !fallback.Ended() {
		fallback = vm.StateDead
	}

	return e.start(ctx, id, fallback)
}

// start boots a VM and runs its task. A VM that cannot be booted is left in
// fallback if it was on its way back up, and says why. The VM's lock is held.
func (e *Engine) start(ctx context.Context, id string, fallback vm.State) (err error) {
	v, err := e.states.Get(ctx, id)
	if err != nil {
		return err
	}

	if e.keeper(id) != nil {
		return nil
	}

	switch v.State {
	case vm.StateRemoving:
		return fmt.Errorf("%w: vm %s is being deleted", vm.ErrConflict, id)
	case vm.StateRunning:
		// it says it runs and nobody looks after it: vmhost has not taken
		// it back yet, which reconciling does.
		return nil
	}

	e.cancelReboot(id)

	// a VM that had ended holds no room any more: it takes it again, if
	// there is any, and says it is on its way back up from here. Booting
	// takes seconds, and it has not ended again in any of them.
	if v.State.Ended() {
		if err := e.readmit(ctx, v); err != nil {
			return err
		}
	}

	defer func() {
		if err != nil {
			e.settle(ctx, id, fallback, err)
		}
	}()

	root, err := e.imageRoot(ctx, v)
	if err != nil {
		return err
	}

	interfaces, err := e.plug(ctx, v)
	if err != nil {
		return err
	}

	v.Interfaces = interfaces

	fail := func(err error) error {
		letGo := context.WithoutCancel(ctx)

		return errors.Join(err, e.hypervisor.Terminate(letGo, id), e.unplug(letGo, id))
	}

	// whatever an earlier boot of the VM left behind goes first.
	if err := e.hypervisor.Terminate(ctx, id); err != nil {
		return fail(err)
	}

	// a boot is measured from asking for the machine until its agent
	// answers, which is what booting it is.
	booting := time.Now()

	machine, err := e.hypervisor.Boot(ctx, e.machineSpec(v, root))
	if err != nil {
		e.metrics.booted(ctx, time.Since(booting), err)

		return fail(err)
	}

	client := e.guests.Connect(machine.VsockPath)

	failWithClient := func(err error) error {
		return errors.Join(fail(err), client.Close())
	}

	ready, cancel := context.WithTimeout(ctx, e.timing.boot)
	defer cancel()

	err = client.Ready(ready)
	e.metrics.booted(ctx, time.Since(booting), err)

	if err != nil {
		e.metrics.agentFailed(ctx, "ready")

		return failWithClient(fmt.Errorf("the machine did not come up: %w", err))
	}

	if err := client.Configure(ctx, e.guestConfig(ctx, v)); err != nil {
		e.metrics.agentFailed(ctx, "configure")

		return failWithClient(fmt.Errorf("the machine could not be told what it is: %w", err))
	}

	out, err := e.logs.Writer(id)
	if err != nil {
		return failWithClient(err)
	}

	status, err := client.Start(ctx, v.Process)
	if err != nil {
		e.metrics.agentFailed(ctx, "start")

		// a task that could not be started at all has ended, the way a
		// container whose command is not there has.
		_, _ = e.states.Update(context.WithoutCancel(ctx), id, func(v *vm.VM) {
			v.State = vm.StateExited
			v.ExitCode = 127
			v.FinishedAt = time.Now().UTC()
		})

		return errors.Join(failWithClient(fmt.Errorf("the task could not be started: %w", err)), out.Close())
	}

	started := status.StartedAt
	if started.IsZero() {
		started = time.Now().UTC()
	}

	updated, err := e.states.Update(ctx, id, func(v *vm.VM) {
		v.State = vm.StateRunning
		v.Interfaces = interfaces
		v.StartedAt = started
		v.FinishedAt = time.Time{}
		v.ExitCode = 0
		v.Generation = status.Generation
		v.LogBase = out.Last()
		v.Stopped = false
		v.Reason = ""
	})
	if err != nil {
		return errors.Join(failWithClient(err), out.Close())
	}

	e.attach(updated, client, out)
	e.refreshHosts(ctx, id, updated.Interfaces)

	e.logger.InfoContext(ctx, "vm started", "vm", id, "name", v.Spec.Name, "uid", v.UID)

	return nil
}

// readmit takes room again for a VM whose task had ended, and says it is on
// its way back up. The VM's lock is held.
func (e *Engine) readmit(ctx context.Context, v vm.VM) error {
	e.admission.Lock()
	defer e.admission.Unlock()

	all, err := e.states.All(ctx)
	if err != nil {
		return err
	}

	// its scratch disk was counted all along.
	if err := e.admit(ctx, all, v.ID, v.MemoryMiB, v.VCPUs, 0); err != nil {
		return err
	}

	_, err = e.states.Update(ctx, v.ID, func(v *vm.VM) { v.State = vm.StateRestarting })

	return err
}

// settle says a VM that could not be booted is not on its way up after all,
// and why.
func (e *Engine) settle(ctx context.Context, id string, fallback vm.State, cause error) {
	_, err := e.states.Update(context.WithoutCancel(ctx), id, func(v *vm.VM) {
		if v.State == vm.StateRestarting {
			v.State = fallback
		}

		v.Reason = reason(cause)
	})
	if err != nil && !errors.Is(err, vm.ErrNotFound) {
		e.logger.ErrorContext(ctx, "what became of a vm that could not be booted could not be kept", "vm", id, "error", err)
	}
}

// maxReason bounds why a VM is where it is, as it is kept and reported.
const maxReason = 1 << 10

func reason(err error) string {
	message := err.Error()
	if len(message) > maxReason {
		message = message[:maxReason]
	}

	return message
}

// imageRoot is the disk holding the image a VM boots: the one it was made
// from, which a tag that moved since does not change. One that was let go of
// is made again from the reference, if that still gives the same image.
func (e *Engine) imageRoot(ctx context.Context, v vm.VM) (string, error) {
	images, err := e.images.List(ctx)
	if err != nil {
		return "", err
	}

	for _, image := range images {
		if image.Digest == v.ImageDigest {
			return image.Root, nil
		}
	}

	image, err := e.images.Ensure(ctx, v.Spec.Image)
	if err != nil {
		return "", err
	}

	if image.Digest != v.ImageDigest {
		return "", fmt.Errorf("%w: the image vm %s was made from (%s) is gone, and %s is another image now", vm.ErrImage, v.ID, v.ImageDigest, v.Spec.Image)
	}

	return image.Root, nil
}

// plug gives a VM's machine its taps and its addresses, and writes them down,
// so that reconciling never takes them for left over. A VM on no network gets
// none.
func (e *Engine) plug(ctx context.Context, v vm.VM) ([]vm.Interface, error) {
	if len(v.Spec.Networks) == 0 {
		return nil, nil
	}

	e.plugging.Lock()
	defer e.plugging.Unlock()

	interfaces, err := e.fabric.Plug(ctx, v.ID, v.UID, v.Spec.Networks)
	if err != nil {
		return nil, errors.Join(err, e.fabric.Unplug(context.WithoutCancel(ctx), v.ID))
	}

	// the fabric says the devices in the order the networks were given;
	// what they are called there is the VM's own to say.
	for i := range interfaces {
		if i >= len(v.Spec.Networks) {
			break
		}

		if len(interfaces[i].Network) == 0 {
			interfaces[i].Network = v.Spec.Networks[i].Network
		}

		if len(interfaces[i].Aliases) == 0 {
			interfaces[i].Aliases = slices.Clone(v.Spec.Networks[i].Aliases)
		}
	}

	if _, err := e.states.Update(ctx, v.ID, func(v *vm.VM) { v.Interfaces = interfaces }); err != nil {
		return nil, errors.Join(err, e.fabric.Unplug(context.WithoutCancel(ctx), v.ID))
	}

	return interfaces, nil
}

// unplug takes a VM's taps away, gives back its addresses, and writes down
// that it has none.
func (e *Engine) unplug(ctx context.Context, id string) error {
	err := e.fabric.Unplug(ctx, id)

	_, updateErr := e.states.Update(ctx, id, func(v *vm.VM) { v.Interfaces = nil })
	if errors.Is(updateErr, vm.ErrNotFound) {
		updateErr = nil
	}

	return errors.Join(err, updateErr)
}

// machineSpec is the machine a VM is booted as: the image shared by every VM
// that runs it, only ever read, and the scratch disk of its own.
func (e *Engine) machineSpec(v vm.VM, root string) vm.MachineSpec {
	drives := []vm.Drive{{Path: root, ReadOnly: true}}
	if !v.Spec.ReadOnly {
		drives = append(drives, vm.Drive{Path: e.config.ScratchPath(v.ID)})
	}

	nics := make([]vm.NIC, 0, len(v.Interfaces))
	for _, i := range v.Interfaces {
		nics = append(nics, vm.NIC{Device: i.Device, MAC: i.MAC})
	}

	return vm.MachineSpec{
		ID:         v.ID,
		VCPUs:      v.VCPUs,
		CPU:        v.Spec.Resources.CPU,
		MemoryMiB:  v.MemoryMiB,
		Kernel:     e.config.Kernel,
		Initrd:     e.config.Initrd,
		KernelArgs: guest.KernelArgs,
		Drives:     drives,
		NICs:       nics,
		UID:        v.UID,
	}
}

// guestConfig is what a machine is told it is.
func (e *Engine) guestConfig(ctx context.Context, v vm.VM) guest.Config {
	config := guest.Config{
		Now:      time.Now().UTC(),
		Hostname: hostnameOf(v),
		Root:     guest.Root{Image: guest.ImageDevice},
		Hosts:    e.hostsOf(ctx, v),
	}

	if !v.Spec.ReadOnly {
		config.Root.Scratch = guest.ScratchDevice
	}

	routesOut := false

	for _, i := range v.Interfaces {
		config.Interfaces = append(config.Interfaces, guest.Interface{MAC: i.MAC, Address: i.Address, Gateway: i.Gateway})

		if len(i.Gateway) > 0 {
			routesOut = true
		}
	}

	// a machine that cannot reach the internet is given nowhere to ask it
	// for names.
	if routesOut {
		config.Nameservers = slices.Clone(e.config.Nameservers)
	}

	return config
}
