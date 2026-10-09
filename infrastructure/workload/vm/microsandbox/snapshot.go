//go:build microsandbox

package microsandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// Snapshot writes an instance's disk to archive.
//
// The disk of a running instance is captured where it is, without stopping
// it. A Docker VM whose init is vminit cannot have its filesystems frozen by
// microsandbox's agent, which owns no init but its own, so it is flushed
// first and captured as it is on disk: what it wrote is all there, as after a
// crash, and a database that was writing recovers the way it would from one.
// Stopping it first is what makes a capture of a database consistent.
//
// microsandbox writes an archive only to a file, so it is written to one on
// the vmhost's volume, with the image it boots from, streamed to archive and
// removed whatever happens. The snapshot it is written from is installed only
// for as long as that takes.
func (e *engine) Snapshot(ctx context.Context, id string, archive io.Writer) (vm.Archive, error) {
	i, err := e.get(id)
	if err != nil {
		return vm.Archive{}, err
	}

	captured, r, err := e.capture(ctx, i)
	if err != nil {
		return vm.Archive{}, err
	}
	defer e.removeSnapshot(captured.Path())

	dir, err := os.MkdirTemp(e.tmp, "snapshot-")
	if err != nil {
		return vm.Archive{}, err
	}
	defer os.RemoveAll(dir)

	saved := filepath.Join(dir, "archive.msb")
	if err := msb.Snapshot.Save(ctx, captured.Path(), saved, msb.SnapshotSaveOptions{WithImage: true}); err != nil {
		return vm.Archive{}, fmt.Errorf("the snapshot cannot be saved as an archive: %w", err)
	}

	file, err := os.Open(saved)
	if err != nil {
		return vm.Archive{}, err
	}
	defer file.Close()

	header := archiveHeader{
		Engine: archiveEngine(),
		Kind:   r.kind(e.dockerImage),
		Image:  r.Image,
		Disk:   r.Spec.Resources.Disk,
	}

	written, err := writeHeader(archive, header)
	if err != nil {
		return vm.Archive{}, err
	}

	copied, err := io.Copy(archive, file)
	if err != nil {
		return vm.Archive{}, err
	}

	return vm.Archive{
		Engine: header.Engine,
		Kind:   header.Kind,
		Image:  header.Image,
		Disk:   header.Disk,
		Size:   written + copied,
	}, nil
}

// capture installs a snapshot of an instance's disk, taken in its turn, and
// says what the instance was when it was taken.
func (e *engine) capture(ctx context.Context, i *instance) (*msb.SnapshotArtifact, *record, error) {
	if err := i.acquire(ctx); err != nil {
		return nil, nil, err
	}
	defer i.release()

	h, err := e.sandboxOf(ctx, i.id)
	if err != nil {
		return nil, nil, err
	}

	if h == nil {
		return nil, nil, fmt.Errorf("%w: %q has no disk to snapshot, since its sandbox is gone", domain.ErrNotExists, i.id)
	}

	flush := msb.GuestFlushAuto
	if h.Status() == msb.SandboxStatusRunning {
		flush = e.flushFor(ctx, i)
	}

	captured, err := e.snapshotOf(ctx, i.id, flush)
	if err != nil {
		return nil, nil, err
	}

	return captured, i.current(), nil
}

// snapshotOf installs a snapshot of the disk of the sandbox named id, under a
// name and in a group of its own, so it shares nothing with any other.
func (e *engine) snapshotOf(ctx context.Context, id string, flush msb.GuestFlush) (*msb.SnapshotArtifact, error) {
	captured, err := msb.Snapshot.Create(ctx, msb.SnapshotCreateOptions{
		Name:        newSnapshotName(),
		Group:       newSnapshotName(),
		FromSandbox: id,
		GuestFlush:  flush,
	})
	if err != nil {
		return nil, fmt.Errorf("the disk cannot be snapshotted: %w", err)
	}

	return captured, nil
}

// flushFor is how a running instance's disk is flushed for a snapshot. Its
// filesystems are flushed first either way; microsandbox's agent then
// freezes them, unless the guest's init is vminit, under which it refuses
// to.
func (e *engine) flushFor(ctx context.Context, i *instance) msb.GuestFlush {
	sb, err := e.live(ctx, i)
	if err != nil {
		return msb.GuestFlushAuto
	}

	ctx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()

	out, err := sb.Shell(ctx, syncForSnapshotScript)
	if err == nil && out.ExitCode() == 0 {
		return msb.GuestFlushSkip
	}

	return msb.GuestFlushAuto
}

// Restore replaces the instance spec.ID names with what archive holds, or
// makes it.
//
// The archive is checked to be this engine's before anything else happens,
// and loaded before the instance is touched, so a restore that cannot read it
// leaves the instance as it was. An instance being replaced is stopped,
// dockerd first, and its sandbox removed; the one restored in its place is
// named the same, given the same host ports and the spec's resources and
// network, and a Docker VM's supervisor is ensured since a restore drops its
// init (#1676). Its labels and environment are the spec's, which the engine
// keeps for the same reason.
func (e *engine) Restore(ctx context.Context, spec vm.Spec, archive io.Reader) (vm.Instance, error) {
	if err := e.validate(spec); err != nil {
		return vm.Instance{}, err
	}

	if err := e.shuttingDown(); err != nil {
		return vm.Instance{}, err
	}

	header, rest, err := readHeader(archive)
	if err != nil {
		return vm.Instance{}, err
	}

	i, created, err := e.instanceFor(spec)
	if err != nil {
		return vm.Instance{}, err
	}

	if err := i.acquire(ctx); err != nil {
		if created {
			e.abandon(i)
		}

		return vm.Instance{}, err
	}
	defer i.release()

	previous := i.current()

	if !created {
		if err := e.reserve(i, func(r *record) {
			r.Spec = cloneSpec(spec)
			r.Image = header.Image
		}); err != nil {
			return vm.Instance{}, err
		}
	}

	undo := func() {
		if created {
			e.abandon(i)
		} else {
			e.put(i, previous)
		}
	}

	loaded, err := e.load(ctx, rest)
	if err != nil {
		undo()

		return vm.Instance{}, err
	}
	defer e.removeSnapshot(loaded)

	if err := e.removeSandbox(ctx, i); err != nil {
		undo()

		return vm.Instance{}, err
	}

	if err := e.restoreFrom(ctx, i, loaded, true, func(r *record) {
		r.Image = header.Image
		r.Exit = nil
		r.Failure = ""
	}); err != nil {
		if created {
			e.abandon(i)
		}

		return vm.Instance{}, err
	}

	return e.inspect(context.WithoutCancel(ctx), i)
}

// instanceFor is the instance a restore is for: the one there is, or one
// recorded for it now, whose resources count against the node's budget and
// whose ports are its own from now on.
func (e *engine) instanceFor(spec vm.Spec) (*instance, bool, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	if i, exists := e.instances[spec.ID]; exists {
		return i, false, nil
	}

	if err := fits(e.budget, e.allocated(""), spec.Resources); err != nil {
		return nil, false, err
	}

	hostPorts, err := e.hostPorts(spec.ID, spec.Ports, nil)
	if err != nil {
		return nil, false, err
	}

	r := &record{
		Spec:      cloneSpec(spec),
		Image:     e.imageOf(spec),
		HostPorts: hostPorts,
		Creating:  true,
	}

	if err := e.store.save(spec.ID, r); err != nil {
		return nil, false, err
	}

	i := newInstance(spec.ID, r)
	e.instances[spec.ID] = i

	return i, true, nil
}

// reserve changes what an instance is given and takes it out of the node's
// budget at once, so that nothing else is given it meanwhile, and gives the
// guest ports it now has host ports, keeping those it had. It is called with
// the instance's turn held.
func (e *engine) reserve(i *instance, change func(r *record)) error {
	e.lock.Lock()
	defer e.lock.Unlock()

	next := i.current()
	change(next)

	if err := fits(e.budget, e.allocated(i.id), next.Spec.Resources); err != nil {
		return err
	}

	hostPorts, err := e.hostPorts(i.id, next.Spec.Ports, next.HostPorts)
	if err != nil {
		return err
	}

	next.HostPorts = hostPorts

	return e.update(i, func(r *record) { *r = *next })
}

// put gives an instance back the record it had, for a change that did not
// happen.
func (e *engine) put(i *instance, previous *record) {
	if err := e.update(i, func(r *record) { *r = *previous.clone() }); err != nil {
		e.logger.Warn("the record of a vm whose change failed could not be put back", "vm", i.id, "error", err)
	}
}

// removeSandbox stops an instance's sandbox, gracefully, and removes it.
func (e *engine) removeSandbox(ctx context.Context, i *instance) error {
	e.halt(i)
	e.forget(i)

	h, err := e.sandboxOf(ctx, i.id)
	if err != nil || h == nil {
		return err
	}

	if up(h.Status()) {
		if err := e.shutdownSandbox(ctx, i, h, stopTimeout); err != nil {
			return err
		}
	}

	if err := h.Remove(ctx); err != nil && !msb.IsKind(err, msb.ErrSandboxNotFound) {
		return fmt.Errorf("the sandbox being replaced cannot be removed: %w", err)
	}

	return nil
}

// restoreFrom restores an instance's sandbox from the snapshot installed at
// path, with the record change makes. microsandbox boots whatever it
// restores; one that is not to keep running is stopped again at once, before
// anything is started in it.
func (e *engine) restoreFrom(ctx context.Context, i *instance, path string, keepRunning bool, change func(r *record)) error {
	if err := e.update(i, change); err != nil {
		return err
	}

	r := i.current()

	config, err := e.restoreConfig(r)
	if err != nil {
		return err
	}

	turn, err := e.turn(ctx)
	if err != nil {
		return err
	}
	defer turn()

	booting := context.WithoutCancel(ctx)

	sb, err := msb.RestoreSandbox(booting, path, i.id, msb.WithRestoreConfig(config))
	if err != nil {
		reason := fmt.Sprintf("it could not be restored: %v", err)
		if updateErr := e.update(i, func(r *record) { r.Failure = reason }); updateErr != nil {
			e.logger.Warn("a vm's failed restore could not be recorded", "vm", i.id, "error", updateErr)
		}

		return fmt.Errorf("the vm could not be restored: %w", err)
	}

	e.keep(i, sb)
	e.grow(booting, i, sb, r)

	if err := e.update(i, func(r *record) {
		r.Restored = true
		r.StartedAt = time.Now()
		r.MainRunning = false
		r.Creating = false
	}); err != nil {
		return err
	}

	if !keepRunning {
		turn()

		h, err := e.sandboxOf(booting, i.id)
		if err != nil || h == nil {
			return err
		}

		defer e.forget(i)

		return e.shutdownSandbox(booting, i, h, endedStopTimeout)
	}

	return e.booted(booting, i, sb, turn)
}

// grow gives a restored disk the size its record asks for, when that is more
// than the snapshot had. A flat disk grows while it runs, and never shrinks.
func (e *engine) grow(ctx context.Context, i *instance, sb *msb.Sandbox, r *record) {
	if r.Spec.HasMainProcess() || r.Spec.Resources.Disk == 0 {
		return
	}

	size, err := mebibytes(r.Spec.Resources.Disk)
	if err != nil {
		return
	}

	if _, err := sb.Modify(ctx, msb.ModifyOptions{RootDiskSizeMiB: size}); err != nil {
		e.logger.Debug("a restored disk was left the size it was", "vm", i.id, "error", err)
	}
}

// load installs the snapshot an archive holds, reading it as it arrives, and
// says where it is.
//
// microsandbox reads an archive from a path only, in one pass, so the archive
// is fed to it through a named pipe on the vmhost's volume and is never
// written down whole. The pipe is opened for reading and writing, which on
// Linux never waits for a reader, so a load that fails before it opens the
// pipe leaves nothing waiting, and closing it lets go of a feeder whose reader
// stopped reading.
func (e *engine) load(ctx context.Context, archive io.Reader) (string, error) {
	dir, err := os.MkdirTemp(e.tmp, "restore-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "archive.msb")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		return "", err
	}

	pipe, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return "", err
	}

	fed := make(chan error, 1)

	go func() {
		_, err := io.Copy(pipe, archive)

		// the end of the archive: its reader sees it once the only writer
		// is gone.
		_ = pipe.Close()
		fed <- err
	}()

	loaded, err := msb.Snapshot.Load(ctx, path, "")
	if err != nil {
		_ = pipe.Close()

		select {
		case feedErr := <-fed:
			if feedErr != nil && !errors.Is(feedErr, os.ErrClosed) {
				return "", fmt.Errorf("the archive could not be read: %w", feedErr)
			}
		default:
		}

		return "", fmt.Errorf("the archive could not be loaded: %w", err)
	}

	return loaded.Path(), nil
}

// removeSnapshot removes a snapshot that was installed for one operation. A
// sandbox restored from it keeps working without it.
func (e *engine) removeSnapshot(path string) {
	if err := msb.Snapshot.Remove(context.Background(), path, true); err != nil {
		e.logger.Warn("a snapshot could not be removed", "snapshot", path, "error", err)
	}
}

// newSnapshotName is a name no other snapshot has, which says it is the
// engine's.
func newSnapshotName() string {
	var random [8]byte
	_, _ = rand.Read(random[:])

	return snapshotPrefix + hex.EncodeToString(random[:])
}

// Reconfigure gives an instance the ports, network and resources a spec says.
//
// microsandbox changes none of them on a sandbox that exists (#1538), so a VM
// with a persistent disk is stopped, its disk snapshotted, its sandbox removed
// and restored from the snapshot under the same name with the new ones; one
// that was not running is stopped again. Any other instance is made afresh
// from its image on every boot anyway, so a running one is booted again with
// the new ones, and one that is down takes them when it boots. A spec that
// changes nothing changes nothing.
func (e *engine) Reconfigure(ctx context.Context, spec vm.Spec) (vm.Instance, error) {
	i, err := e.get(spec.ID)
	if err != nil {
		return vm.Instance{}, err
	}

	if err := e.shuttingDown(); err != nil {
		return vm.Instance{}, err
	}

	if err := i.acquire(ctx); err != nil {
		return vm.Instance{}, err
	}
	defer i.release()

	previous := i.current()

	if err := e.reserve(i, func(r *record) {
		r.Spec.Ports = cloneSpec(spec).Ports
		r.Spec.Network = spec.Network
		r.Spec.Resources = spec.Resources
	}); err != nil {
		return vm.Instance{}, err
	}

	if unchanged(previous, i.current()) {
		return e.inspect(ctx, i)
	}

	if err := e.reconfigure(ctx, i, previous); err != nil {
		return vm.Instance{}, err
	}

	return e.inspect(context.WithoutCancel(ctx), i)
}

// reconfigure puts an instance's new record in place in its sandbox.
func (e *engine) reconfigure(ctx context.Context, i *instance, previous *record) error {
	h, err := e.sandboxOf(ctx, i.id)
	if err != nil {
		e.put(i, previous)

		return err
	}

	running := h != nil && up(h.Status())

	// a sandbox made afresh on every boot takes it when it next boots.
	if !i.current().Spec.PersistentDisk || h == nil {
		if !running {
			return nil
		}

		if err := e.stop(ctx, i); err != nil {
			return err
		}

		h, err := e.sandboxOf(ctx, i.id)
		if err != nil {
			return err
		}

		return e.boot(ctx, i, h)
	}

	e.halt(i)

	if running {
		if err := e.shutdownSandbox(ctx, i, h, stopTimeout); err != nil {
			e.put(i, previous)

			return err
		}
	}

	e.forget(i)

	captured, err := e.snapshotOf(ctx, i.id, msb.GuestFlushAuto)
	if err != nil {
		e.put(i, previous)

		return err
	}

	if err := h.Remove(ctx); err != nil && !msb.IsKind(err, msb.ErrSandboxNotFound) {
		e.removeSnapshot(captured.Path())
		e.put(i, previous)

		return fmt.Errorf("the vm's sandbox cannot be removed to reconfigure it: %w", err)
	}

	clearFailure := func(r *record) { r.Failure = "" }

	restoreErr := e.restoreFrom(ctx, i, captured.Path(), running, clearFailure)
	if restoreErr == nil {
		e.removeSnapshot(captured.Path())

		return nil
	}

	// the disk is in the snapshot, and the sandbox is gone: it is restored
	// with the configuration it had, which it had no trouble with, and the
	// snapshot is kept when even that fails, since it is all there is of the
	// disk.
	e.logger.Warn("a reconfigured vm could not be restored, and is restored as it was", "vm", i.id, "error", restoreErr)

	e.put(i, previous)

	if err := e.restoreFrom(ctx, i, captured.Path(), running, clearFailure); err != nil {
		reason := fmt.Sprintf("it could not be restored after a reconfigure; its disk is kept in snapshot %s", captured.Path())
		if updateErr := e.update(i, func(r *record) { r.Failure = reason }); updateErr != nil {
			e.logger.Warn("a vm's failed reconfigure could not be recorded", "vm", i.id, "error", updateErr)
		}

		return errors.Join(restoreErr, err)
	}

	e.removeSnapshot(captured.Path())

	return restoreErr
}
