//go:build microsandbox

package microsandbox

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Create makes an instance and boots it.
//
// The record is written first and says the sandbox is being made, so a
// vmhost that dies halfway knows not to keep it. A create that fails leaves
// nothing: nothing was on the disk yet, and the next create starts afresh.
// microsandbox's create is not cut short when ctx ends, because a create that
// is cancelled leaves a stopped sandbox behind (#1687): it finishes, and the
// caller asking again finds the instance there.
func (e *engine) Create(ctx context.Context, spec vm.Spec) (vm.Instance, error) {
	if err := e.validate(spec); err != nil {
		return vm.Instance{}, err
	}

	if err := e.shuttingDown(); err != nil {
		return vm.Instance{}, err
	}

	existing, err := e.sandboxOf(ctx, spec.ID)
	if err != nil {
		return vm.Instance{}, err
	}

	if existing != nil {
		return vm.Instance{}, fmt.Errorf("%w: a sandbox %q", domain.ErrAlreadyExists, spec.ID)
	}

	i, created, err := e.instanceFor(spec)
	if err != nil {
		return vm.Instance{}, err
	}

	if !created {
		return vm.Instance{}, fmt.Errorf("%w: an instance %q", domain.ErrAlreadyExists, spec.ID)
	}

	if err := i.acquire(ctx); err != nil {
		e.abandon(i)

		return vm.Instance{}, err
	}
	defer i.release()

	if err := e.boot(ctx, i, nil); err != nil {
		e.abandon(i)

		return vm.Instance{}, err
	}

	return e.inspect(context.WithoutCancel(ctx), i)
}

// abandon lets go of an instance whose sandbox could not be made, and of
// whatever microsandbox made of it before it gave up.
func (e *engine) abandon(i *instance) {
	ctx, cancel := context.WithTimeout(context.Background(), killTimeout)
	defer cancel()

	if err := e.destroy(ctx, i.id); err != nil {
		e.logger.Warn("what was made of a vm whose create failed could not be removed", "vm", i.id, "error", err)
	}

	e.lock.Lock()
	delete(e.instances, i.id)
	e.lock.Unlock()

	if err := e.store.remove(i.id); err != nil {
		e.logger.Warn("the record of a vm whose create failed could not be removed", "vm", i.id, "error", err)
	}
}

// imageOf is what an instance boots from: a Docker VM boots the vmhost's own
// docker-in-docker image, which the control plane names too.
func (e *engine) imageOf(spec vm.Spec) string {
	if spec.Kind == vm.KindDocker && len(e.dockerImage) > 0 {
		return e.dockerImage
	}

	return spec.Image
}

// validate refuses a spec no sandbox can be made of.
func (e *engine) validate(spec vm.Spec) error {
	if err := validID(spec.ID); err != nil {
		return err
	}

	if !spec.Kind.IsValid() {
		return fmt.Errorf("%q is not a kind of vm", spec.Kind)
	}

	if len(e.imageOf(spec)) == 0 {
		return errors.New("a vm boots from an image, and none was named")
	}

	if spec.Kind == vm.KindDocker && spec.HasMainProcess() {
		return errors.New("a docker vm runs dockerd, not a main process of its own")
	}

	if _, err := vcpus(spec.Resources.CPUs); err != nil {
		return err
	}

	if _, err := mebibytes(spec.Resources.Memory); err != nil {
		return err
	}

	if _, err := mebibytes(spec.Resources.Disk); err != nil {
		return err
	}

	for _, guest := range spec.Ports {
		if guest == 0 || guest > 65535 {
			return fmt.Errorf("%d is not a port", guest)
		}
	}

	return nil
}

func (e *engine) Start(ctx context.Context, id string) error {
	i, err := e.get(id)
	if err != nil {
		return err
	}

	if err := i.acquire(ctx); err != nil {
		return err
	}
	defer i.release()

	return e.start(ctx, i)
}

// start boots an instance that is down. One that is running already is what
// was asked for.
func (e *engine) start(ctx context.Context, i *instance) error {
	h, err := e.sandboxOf(ctx, i.id)
	if err != nil {
		return err
	}

	if h != nil && h.Status() == msb.SandboxStatusDraining {
		if h, err = e.stopped(ctx, h); err != nil {
			return err
		}
	}

	if h != nil && up(h.Status()) {
		return nil
	}

	return e.boot(ctx, i, h)
}

// stopped waits for a sandbox on its way down to be down.
func (e *engine) stopped(ctx context.Context, h *msb.SandboxHandle) (*msb.SandboxHandle, error) {
	ctx, cancel := context.WithTimeout(ctx, stopTimeout)
	defer cancel()

	if _, err := h.WaitUntilStopped(ctx); err != nil {
		return nil, fmt.Errorf("%q is still stopping: %w", h.Name(), err)
	}

	return e.sandboxOf(ctx, h.Name())
}

func (e *engine) Stop(ctx context.Context, id string) error {
	i, err := e.get(id)
	if err != nil {
		return err
	}

	if err := i.acquire(ctx); err != nil {
		return err
	}
	defer i.release()

	return e.stop(ctx, i)
}

// stop stops an instance that is up. What runs in it goes down with it: the
// sessions into it, and its main process, which was stopped rather than ended
// on its own, so the instance is stopped rather than exited.
func (e *engine) stop(ctx context.Context, i *instance) error {
	e.halt(i)

	h, err := e.sandboxOf(ctx, i.id)
	if err != nil {
		return err
	}

	if h != nil && up(h.Status()) {
		if err := e.shutdownSandbox(ctx, i, h, stopTimeout); err != nil {
			return err
		}
	}

	e.forget(i)

	if !i.current().MainRunning {
		return nil
	}

	return e.update(i, func(r *record) { r.MainRunning = false })
}

func (e *engine) Restart(ctx context.Context, id string) error {
	i, err := e.get(id)
	if err != nil {
		return err
	}

	if err := i.acquire(ctx); err != nil {
		return err
	}
	defer i.release()

	if err := e.stop(ctx, i); err != nil {
		return err
	}

	h, err := e.sandboxOf(ctx, i.id)
	if err != nil {
		return err
	}

	return e.boot(ctx, i, h)
}

// Delete removes an instance, disk and all. A sandbox the engine has no
// record of is removed all the same: under the vmhost's home, every sandbox is
// the engine's.
func (e *engine) Delete(ctx context.Context, id string) error {
	if err := validID(id); err != nil {
		return err
	}

	e.lock.Lock()
	i, ok := e.instances[id]
	e.lock.Unlock()

	if !ok {
		return e.destroy(ctx, id)
	}

	if err := i.acquire(ctx); err != nil {
		return err
	}
	defer i.release()

	e.halt(i)
	e.forget(i)

	if err := e.destroy(ctx, id); err != nil {
		return err
	}

	e.lock.Lock()
	delete(e.instances, id)
	e.lock.Unlock()

	return e.store.remove(id)
}

// destroy removes the sandbox named id, killing it if it runs. Its disk is
// going, so there is nothing to stop gracefully.
func (e *engine) destroy(ctx context.Context, id string) error {
	h, err := e.sandboxOf(ctx, id)
	if err != nil || h == nil {
		return err
	}

	err = h.Destroy(ctx, msb.WithDestroyForce(), msb.WithDestroyTimeout(killTimeout))
	if msb.IsKind(err, msb.ErrSandboxNotFound) {
		return nil
	}

	return err
}

// boot brings an instance's sandbox up, given what there is of it.
//
// A VM with a persistent disk boots the sandbox it has, and one that has none
// is not made again: that would be a new, empty disk in place of the one that
// was lost, and nobody would be told. Any other instance boots a sandbox made
// afresh from its image every time, which is how its disk is pristine on
// every start; its first one is its create.
//
// A boot takes one of the node's turns, and a Docker VM keeps it until dockerd
// answers: what a storm of boots starves is dockerd's start. Once it is up, a
// Docker VM's supervisor is ensured, and an instance with a main process
// starts it before anything else may be exec'd into it.
func (e *engine) boot(ctx context.Context, i *instance, h *msb.SandboxHandle) error {
	if err := e.shuttingDown(); err != nil {
		return err
	}

	r := i.current()
	fresh := !r.Spec.PersistentDisk || r.Creating

	if !fresh && h == nil {
		const gone = "its sandbox is gone, and its disk with it; it is not made again empty"

		if err := e.update(i, func(r *record) { r.Failure = gone }); err != nil {
			return err
		}

		return errors.New(gone)
	}

	options, err := e.createOptions(r)
	if err != nil {
		return err
	}

	turn, err := e.turn(ctx)
	if err != nil {
		return err
	}
	defer turn()

	// a boot is not cut short halfway by a caller that gave up: a sandbox
	// whose start is cancelled is left in no state to be started again.
	booting := context.WithoutCancel(ctx)

	var sb *msb.Sandbox

	if fresh {
		if !r.Creating {
			if err := e.destroy(booting, i.id); err != nil {
				return fmt.Errorf("the last boot's sandbox cannot be removed: %w", err)
			}
		}

		sb, err = msb.CreateSandbox(booting, i.id, options...)
	} else {
		sb, err = h.StartDetached(booting)
	}

	if err != nil {
		err = fmt.Errorf("the vm could not be booted: %w", err)

		// one made afresh has no sandbox left to say it is down, so its
		// record says why instead, until it boots.
		if fresh && !r.Creating {
			if updateErr := e.update(i, func(r *record) { r.Failure = err.Error() }); updateErr != nil {
				e.logger.Warn("a vm's failed boot could not be recorded", "vm", i.id, "error", updateErr)
			}
		}

		return err
	}

	e.keep(i, sb)

	if err := e.update(i, func(r *record) {
		r.StartedAt = time.Now()
		r.Failure = ""
		r.Exit = nil
		r.MainRunning = false

		// a sandbox made afresh has the init it was made with.
		if fresh {
			r.Restored = false
		}

		r.Creating = false
	}); err != nil {
		return err
	}

	return e.booted(booting, i, sb, turn)
}

// booted finishes a boot once the sandbox is up: a Docker VM's dockerd is
// looked after and waited for, and an instance's main process is started.
// turn gives back the boot's turn, which is done as soon as nothing that
// follows is part of the boot.
func (e *engine) booted(ctx context.Context, i *instance, sb *msb.Sandbox, turn func()) error {
	r := i.current()

	if r.Spec.Kind == vm.KindDocker {
		if err := e.ensure(ctx, i, sb); err != nil {
			e.logger.Warn("dockerd's supervisor could not be ensured", "vm", i.id, "error", err)
		}

		if !e.waitDocker(ctx, sb) {
			e.logger.Warn("dockerd is not answering yet; the vm is left to bring it up", "vm", i.id, "waited", dockerReadyTimeout)
		}
	}

	turn()

	if r.Spec.HasMainProcess() {
		return e.startMain(ctx, i, sb)
	}

	return nil
}

// turn waits for one of the node's turns to boot a VM, and gives back what
// returns it.
func (e *engine) turn(ctx context.Context) (func(), error) {
	select {
	case e.boots <- struct{}{}:
		var once sync.Once

		return func() { once.Do(func() { <-e.boots }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// halt ends what runs in an instance: its exec sessions, and its main
// process, which the engine ended rather than it ending on its own.
func (e *engine) halt(i *instance) {
	i.mu.Lock()
	sessions := slices.Collect(maps.Keys(i.sessions))
	clear(i.sessions)
	main := i.main
	i.main = nil
	i.mu.Unlock()

	if main != nil {
		main.end()
	}

	var closing sync.WaitGroup
	for _, s := range sessions {
		closing.Go(func() { _ = s.Close() })
	}
	closing.Wait()
}

// shutdownSandbox stops a sandbox that is up, gracefully when it can and by
// killing it when it cannot. A Docker VM's supervisor is stopped first: with
// the agent as the guest's init, a stop kills everything at once.
func (e *engine) shutdownSandbox(ctx context.Context, i *instance, h *msb.SandboxHandle, timeout time.Duration) error {
	if i.current().Spec.Kind == vm.KindDocker && h.Status() == msb.SandboxStatusRunning {
		e.preStop(ctx, i)
	}

	stopping, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	err := h.StopWithTimeout(stopping, timeout)
	cancel()

	if err == nil {
		return nil
	}

	e.logger.Warn("a vm did not stop in time and is killed", "vm", i.id, "error", err)

	killing, cancel := context.WithTimeout(context.WithoutCancel(ctx), killTimeout+5*time.Second)
	defer cancel()

	if err := h.Kill(killing, msb.WithKillTimeout(killTimeout)); err != nil {
		return fmt.Errorf("the vm could not be stopped: %w", err)
	}

	return nil
}

// ensure makes sure a Docker VM's dockerd is looked after: by vminit as the
// guest's init, or else by the supervisor it starts, which a restored VM
// needs since a restore drops the init (#1676). It is run after every boot of
// a Docker VM, and costs nothing for one whose init is in place.
func (e *engine) ensure(ctx context.Context, i *instance, sb *msb.Sandbox) error {
	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	// the directory is the image's own; a failure to make it again says
	// nothing that the write does not.
	_ = sb.FS().Mkdir(ctx, "/usr/local/sbin")

	if err := sb.FS().WriteString(ctx, vminitStaged, vminitScript); err != nil {
		return fmt.Errorf("the supervisor cannot be written: %w", err)
	}

	out, err := sb.Shell(ctx, ensureScript)
	if err != nil {
		return err
	}

	switch out.ExitCode() {
	case 0:
		return nil
	case ensureStarted:
		e.logger.Info("dockerd's supervisor was started", "vm", i.id)

		return nil
	default:
		return fmt.Errorf("ensure exited with %d", out.ExitCode())
	}
}

// preStop stops a Docker VM's supervisor, and dockerd with it, so that its
// containers stop the way docker stops them before the VM goes down. What
// fails here fails the stop no worse than not having tried.
func (e *engine) preStop(ctx context.Context, i *instance) {
	sb, err := e.live(ctx, i)
	if err != nil {
		e.logger.Warn("a docker vm could not be reached to stop dockerd first", "vm", i.id, "error", err)

		return
	}

	ctx, cancel := context.WithTimeout(ctx, preStopTimeout)
	defer cancel()

	if _, err := sb.Shell(ctx, preStopScript); err != nil {
		e.logger.Warn("dockerd could not be stopped ahead of its vm", "vm", i.id, "error", err)
	}
}

// waitDocker waits for a Docker VM's dockerd to answer, and says whether it
// did in time.
func (e *engine) waitDocker(ctx context.Context, sb *msb.Sandbox) bool {
	deadline := time.Now().Add(dockerReadyTimeout)

	for {
		asking, cancel := context.WithTimeout(ctx, commandTimeout)
		out, err := sb.Shell(asking, dockerReadyScript)
		cancel()

		if err == nil && out.ExitCode() == 0 {
			return true
		}

		if time.Now().After(deadline) {
			return false
		}

		select {
		case <-ctx.Done():
			return false
		case <-time.After(dockerReadyPoll):
		}
	}
}

// unchanged reports whether a reconfigure leaves a record as it was.
func unchanged(r *record, next *record) bool {
	return slices.Equal(sortedUnique(r.Spec.Ports), sortedUnique(next.Spec.Ports)) &&
		r.Spec.Network == next.Spec.Network &&
		r.Spec.Resources == next.Spec.Resources &&
		maps.Equal(r.HostPorts, next.HostPorts)
}
