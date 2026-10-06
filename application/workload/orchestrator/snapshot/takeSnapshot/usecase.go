// Package takeSnapshot takes a snapshot of a VM this node holds and stores
// it, as the control plane asks with SnapshotRequested, and says what came of
// it with SnapshotCompleted or SnapshotFailed. The snapshot is not a kind yet:
// it is taken under the lock every command to the VM takes.
package takeSnapshot

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/lock"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot"
	"github.com/khanzadimahdi/testproject/domain/workload/snapshot/events"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// timeout bounds one snapshot. A large disk streamed to S3 takes minutes; one
// that has taken this long is not going to finish, and the VM it holds the
// lock of cannot be stopped, restored or deleted meanwhile.
const timeout = 2 * time.Hour

// errStored is how the archive's reader is closed once the store has read all
// it is going to, which leaves the engine nothing to write to.
var errStored = errors.New("the archive is no longer being stored")

// Recorder is where how long a snapshot took, and how large it came out, are
// recorded.
type Recorder interface {
	Snapshot(ctx context.Context, took time.Duration, size int64)
}

// UseCase takes snapshots.
//
// The engine writes the archive into a pipe and the store reads it out of the
// other end as one upload of unknown length, so it is streamed part by part
// and never held whole: a node has the memory for one part, not for a disk. A
// snapshot that fails leaves nothing behind, and either way what became of it
// is said, so the snapshot is never left being made.
type UseCase struct {
	engine    vm.Engine
	store     snapshot.Store
	locks     *lock.Locks
	producer  domain.Producer
	validator domain.Validator
	recorder  Recorder
	nodeName  string
}

func NewUseCase(
	engine vm.Engine,
	store snapshot.Store,
	locks *lock.Locks,
	producer domain.Producer,
	validator domain.Validator,
	recorder Recorder,
	nodeName string,
) *UseCase {
	return &UseCase{
		engine:    engine,
		store:     store,
		locks:     locks,
		producer:  producer,
		validator: validator,
		recorder:  recorder,
		nodeName:  nodeName,
	}
}

// Execute takes the snapshot and says it is stored, or says why it is not. The
// error it returns is only ever a failure to say so, which is worth asking
// again for.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	release, err := uc.locks.Lock(ctx, request.VMUUID)
	if err != nil {
		return nil, err
	}
	defer release()

	started := time.Now()

	archive, size, err := uc.take(ctx, request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, err
		}

		if err := uc.failed(ctx, request.SnapshotUUID, err); err != nil {
			return nil, err
		}

		return &Response{}, nil
	}

	uc.recorder.Snapshot(ctx, time.Since(started), size)

	if err := publish(ctx, uc.producer, events.SnapshotCompletedName, events.SnapshotCompleted{
		SnapshotUUID: request.SnapshotUUID,
		NodeName:     uc.nodeName,
		Size:         size,
		Engine:       archive.Engine,
		Disk:         archive.Disk,
		At:           time.Now(),
	}); err != nil {
		return nil, err
	}

	return &Response{}, nil
}

// take streams the VM's archive into the store, and says what the engine wrote
// and how much of it was stored.
func (uc *UseCase) take(ctx context.Context, request *Request) (vm.Archive, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	key := snapshot.ObjectKey(request.SnapshotUUID)

	reader, writer := io.Pipe()

	type taken struct {
		archive vm.Archive
		err     error
	}

	written := make(chan taken, 1)

	go func() {
		archive, err := uc.engine.Snapshot(ctx, request.VMUUID, writer)

		// the end of the archive, or why it ends early: the store reads
		// either as the end of what it is storing.
		_ = writer.CloseWithError(err)
		written <- taken{archive: archive, err: err}
	}()

	stored := &counter{reader: reader}
	storeErr := uc.store.Store(ctx, key, stored, -1)

	// a store that stopped reading lets the engine go rather than leave it
	// writing into a pipe nobody empties.
	_ = reader.CloseWithError(cmp.Or(storeErr, errStored))

	result := <-written

	if err := cmp.Or(result.err, storeErr); err != nil {
		// nothing of a snapshot that failed is left behind. An upload that
		// failed stored nothing anyway; this is for one that did not say so.
		_ = uc.store.Delete(context.WithoutCancel(ctx), key)

		return vm.Archive{}, 0, err
	}

	return result.archive, stored.read, nil
}

// failed says a snapshot could not be taken or stored, and why.
func (uc *UseCase) failed(ctx context.Context, snapshotUUID string, cause error) error {
	return publish(ctx, uc.producer, events.SnapshotFailedName, events.SnapshotFailed{
		SnapshotUUID: snapshotUUID,
		NodeName:     uc.nodeName,
		Reason:       cause.Error(),
		At:           time.Now(),
	})
}

// counter counts what is read through it, which is how much was stored.
type counter struct {
	reader io.Reader
	read   int64
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.reader.Read(p)
	c.read += int64(n)

	return n, err
}

// publish sends one event. It is detached from the request's context, so
// what became of a snapshot is said even when whoever was waiting has gone.
func publish(ctx context.Context, producer domain.Producer, subject string, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return producer.Produce(context.WithoutCancel(ctx), subject, payload)
}
