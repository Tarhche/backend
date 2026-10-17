package vmhost_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

// disk is several megabytes of what a VM wrote, none of it the same.
func disk(t *testing.T) []byte {
	t.Helper()

	written := make([]byte, 6<<20)
	_, err := rand.Read(written)
	require.NoError(t, err)

	return written
}

// TestClient_SnapshotRestore takes a VM's archive and makes VMs from it,
// through the socket both ways.
func TestClient_SnapshotRestore(t *testing.T) {
	t.Parallel()

	engine := newEngine()
	client := serve(t, engine).client
	ctx := t.Context()

	_, err := client.Create(ctx, machine("vm-1"))
	require.NoError(t, err)

	written := disk(t)
	require.NoError(t, engine.SetDisk("vm-1", written))

	var archive bytes.Buffer

	took, err := client.Snapshot(ctx, "vm-1", &archive)
	require.NoError(t, err)

	assert.Equal(t, vm.Archive{
		Engine: memory.Name + "/" + memory.Version,
		Kind:   vm.KindMachine,
		Image:  "ubuntu:24.04",
		Disk:   1 << 30,
		Size:   int64(archive.Len()),
	}, took)
	assert.Greater(t, took.Size, int64(len(written)), "the whole disk is in it")

	t.Run("restored as a new vm", func(t *testing.T) {
		restored, err := client.Restore(ctx, machine("vm-2"), bytes.NewReader(archive.Bytes()))
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceRunning, restored.State)

		got, err := engine.Disk("vm-2")
		require.NoError(t, err)
		assert.True(t, bytes.Equal(written, got), "the new vm has the disk the archive holds")
	})

	t.Run("restored onto the vm it was taken of", func(t *testing.T) {
		before, err := client.Inspect(ctx, "vm-1")
		require.NoError(t, err)

		require.NoError(t, engine.SetDisk("vm-1", []byte("written after the snapshot")))

		restored, err := client.Restore(ctx, machine("vm-1"), bytes.NewReader(archive.Bytes()))
		require.NoError(t, err)

		got, err := engine.Disk("vm-1")
		require.NoError(t, err)
		assert.True(t, bytes.Equal(written, got), "the vm is back to what it was")
		assert.Equal(t, before.Endpoints, restored.Endpoints, "and keeps its ports")
	})
}

// lockstep is an engine whose snapshot writes an archive a chunk at a time and
// whose restore reads one a chunk at a time, and which goes no further until
// the other end of the socket has the chunk. An archive held whole anywhere
// on the way would never get there.
type lockstep struct {
	vm.Engine

	chunk  int
	chunks int

	// arrived says how much of an archive reached the other end.
	arrived chan int
}

func (e *lockstep) Snapshot(ctx context.Context, id string, archive io.Writer) (vm.Archive, error) {
	written := 0

	for n := range e.chunks {
		chunk := bytes.Repeat([]byte{byte(n)}, e.chunk)

		if _, err := archive.Write(chunk); err != nil {
			return vm.Archive{}, err
		}

		written += len(chunk)

		if err := e.await(ctx, written); err != nil {
			return vm.Archive{}, err
		}
	}

	return vm.Archive{Engine: "lockstep/1", Size: int64(written)}, nil
}

func (e *lockstep) Restore(ctx context.Context, spec vm.Spec, archive io.Reader) (vm.Instance, error) {
	read := 0
	buffer := make([]byte, 64<<10)

	for {
		n, err := archive.Read(buffer)
		read += n

		if n > 0 && read%e.chunk == 0 {
			e.arrived <- read
		}

		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return vm.Instance{}, err
		}
	}

	if read != e.chunk*e.chunks {
		return vm.Instance{}, fmt.Errorf("%d bytes of archive arrived", read)
	}

	return vm.Instance{ID: spec.ID, State: vm.InstanceRunning}, nil
}

// await waits for the other end to have had written bytes.
func (e *lockstep) await(ctx context.Context, written int) error {
	for {
		select {
		case arrived := <-e.arrived:
			if arrived >= written {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(settle):
			return fmt.Errorf("%d bytes were written and the other end never had them: the archive is being held somewhere", written)
		}
	}
}

// counting is the caller's end of a streamed archive, saying how much of it
// has arrived.
type counting struct {
	arrived chan<- int
	total   int
	writes  int
}

func (c *counting) Write(p []byte) (int, error) {
	c.total += len(p)
	c.writes++

	select {
	case c.arrived <- c.total:
	default:
	}

	return len(p), nil
}

// TestClient_Streams holds archives to being streamed, never held: an
// archive is as large as a disk, and the orchestrator holds very little.
func TestClient_Streams(t *testing.T) {
	t.Parallel()

	t.Run("a snapshot arrives as it is written", func(t *testing.T) {
		t.Parallel()

		engine := &lockstep{Engine: newEngine(), chunk: 1 << 20, chunks: 8, arrived: make(chan int, 64)}
		client := serve(t, engine).client

		caller := &counting{arrived: engine.arrived}

		took, err := client.Snapshot(t.Context(), "vm-1", caller)
		require.NoError(t, err)

		assert.Equal(t, int64(8<<20), took.Size)
		assert.Equal(t, 8<<20, caller.total)
		assert.Greater(t, caller.writes, 8, "it arrives in pieces, as it was written")
	})

	t.Run("a restore is sent as it is read", func(t *testing.T) {
		t.Parallel()

		engine := &lockstep{Engine: newEngine(), chunk: 1 << 20, chunks: 8, arrived: make(chan int, 64)}
		client := serve(t, engine).client

		reader, writer := io.Pipe()

		go func() {
			for n := range engine.chunks {
				if _, err := writer.Write(bytes.Repeat([]byte{byte(n)}, engine.chunk)); err != nil {
					return
				}

				// the next chunk is read off the caller's archive only once
				// the vmhost has the one before it.
				if err := engine.await(t.Context(), (n+1)*engine.chunk); err != nil {
					_ = writer.CloseWithError(err)

					return
				}
			}

			_ = writer.Close()
		}()

		restored, err := client.Restore(t.Context(), machine("vm-1"), reader)
		require.NoError(t, err)
		assert.Equal(t, "vm-1", restored.ID)
	})
}

// failing is an engine whose snapshot writes some of an archive and then
// fails, or fails before writing any.
type failing struct {
	vm.Engine

	before int
	err    error
}

func (e *failing) Snapshot(ctx context.Context, id string, archive io.Writer) (vm.Archive, error) {
	if e.before > 0 {
		if _, err := archive.Write(make([]byte, e.before)); err != nil {
			return vm.Archive{}, err
		}
	}

	return vm.Archive{}, e.err
}

// short is an engine whose snapshot says it wrote more than it did.
type short struct {
	vm.Engine
}

func (e *short) Snapshot(ctx context.Context, id string, archive io.Writer) (vm.Archive, error) {
	n, err := archive.Write([]byte("half"))

	return vm.Archive{Engine: "short/1", Size: int64(n) * 2}, err
}

// refusing is a caller's writer that refuses once it has taken limit bytes.
type refusing struct {
	limit int
	taken int
	err   error
}

func (r *refusing) Write(p []byte) (int, error) {
	if r.taken+len(p) > r.limit {
		return 0, r.err
	}

	r.taken += len(p)

	return len(p), nil
}

// watched is an engine that says how its snapshot ended.
type watched struct {
	vm.Engine

	ended chan error
}

func (e *watched) Snapshot(ctx context.Context, id string, archive io.Writer) (vm.Archive, error) {
	took, err := e.Engine.Snapshot(ctx, id, archive)
	e.ended <- err

	return took, err
}

func TestClient_Snapshot_Failures(t *testing.T) {
	t.Parallel()

	t.Run("one that fails before anything was written is answered as any failure", func(t *testing.T) {
		t.Parallel()

		client := serve(t, &failing{Engine: newEngine(), err: fmt.Errorf("%w: it is stopped", vm.ErrNotRunning)}).client

		_, err := client.Snapshot(t.Context(), "vm-1", io.Discard)
		assert.ErrorIs(t, err, vm.ErrNotRunning)
		assert.Contains(t, err.Error(), "it is stopped")
	})

	t.Run("one that fails after its archive began streaming says why at its end", func(t *testing.T) {
		t.Parallel()

		client := serve(t, &failing{Engine: newEngine(), before: 3 << 20, err: fmt.Errorf("%w: the disk went away", vm.ErrNotRunning)}).client

		_, err := client.Snapshot(t.Context(), "vm-1", io.Discard)
		assert.ErrorIs(t, err, vm.ErrNotRunning, "the error is still the engine's")
		assert.Contains(t, err.Error(), "the disk went away")
	})

	t.Run("an archive shorter than the vmhost says is not an archive", func(t *testing.T) {
		t.Parallel()

		client := serve(t, &short{Engine: newEngine()}).client

		_, err := client.Snapshot(t.Context(), "vm-1", io.Discard)
		assert.ErrorContains(t, err, "wrote 8 bytes of archive and 4 arrived")
	})

	t.Run("a caller that stops taking the archive stops the vmhost writing it", func(t *testing.T) {
		t.Parallel()

		engine := &watched{Engine: newEngine(), ended: make(chan error, 1)}
		client := serve(t, engine).client

		_, err := client.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)

		// far more than any buffer on the way holds, so the engine is still
		// writing when the caller gives up.
		require.NoError(t, engine.Engine.(*memory.Engine).SetDisk("vm-1", make([]byte, 32<<20)))

		full := errors.New("the bucket is full")

		_, err = client.Snapshot(t.Context(), "vm-1", &refusing{limit: 1 << 20, err: full})
		assert.ErrorIs(t, err, full, "the caller's own error, as it gave it")

		select {
		case err := <-engine.ended:
			assert.Error(t, err, "the engine is told nobody takes what it writes")
		case <-time.After(settle):
			t.Fatal("the engine was left writing an archive nobody takes")
		}
	})

	t.Run("a caller that gives up stops the vmhost writing it", func(t *testing.T) {
		t.Parallel()

		engine := &watched{Engine: newEngine(), ended: make(chan error, 1)}
		client := serve(t, engine).client

		_, err := client.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)
		require.NoError(t, engine.Engine.(*memory.Engine).SetDisk("vm-1", make([]byte, 32<<20)))

		ctx, cancel := context.WithCancel(t.Context())

		var once sync.Once
		giveUp := writerFunc(func(p []byte) (int, error) {
			once.Do(cancel)

			return len(p), nil
		})

		_, err = client.Snapshot(ctx, "vm-1", giveUp)
		assert.ErrorIs(t, err, context.Canceled)

		select {
		case err := <-engine.ended:
			assert.Error(t, err)
		case <-time.After(settle):
			t.Fatal("the engine was left writing an archive nobody takes")
		}
	})
}

type writerFunc func(p []byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) {
	return f(p)
}

// broken is a caller's archive that fails partway through.
type broken struct {
	archive io.Reader
	err     error
}

func (b *broken) Read(p []byte) (int, error) {
	n, err := b.archive.Read(p)
	if errors.Is(err, io.EOF) {
		return n, b.err
	}

	return n, err
}

func TestClient_Restore_Failures(t *testing.T) {
	t.Parallel()

	t.Run("an archive the caller cannot read to its end is not restored", func(t *testing.T) {
		t.Parallel()

		engine := newEngine()
		client := serve(t, engine).client

		_, err := engine.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)
		require.NoError(t, engine.SetDisk("vm-1", disk(t)))

		var archive bytes.Buffer
		_, err = client.Snapshot(t.Context(), "vm-1", &archive)
		require.NoError(t, err)

		gone := errors.New("the bucket went away")

		// the first half of a good archive, so the engine is still reading
		// when it stops coming.
		half := io.LimitReader(&archive, int64(archive.Len()/2))

		_, err = client.Restore(t.Context(), machine("vm-2"), &broken{archive: half, err: gone})
		assert.ErrorIs(t, err, gone)

		_, err = client.Inspect(t.Context(), "vm-2")
		assert.Error(t, err, "nothing was made of half an archive")
	})

	t.Run("a caller that gives up stops the restore", func(t *testing.T) {
		t.Parallel()

		engine := newEngine()
		client := serve(t, engine).client

		ctx, cancel := context.WithCancel(t.Context())

		reader, writer := io.Pipe()
		t.Cleanup(func() { _ = writer.Close() })

		go func() {
			_, _ = writer.Write([]byte(`{"engine":"memory/1","spec":`))
			cancel()
		}()

		_, err := client.Restore(ctx, machine("vm-1"), reader)
		assert.ErrorIs(t, err, context.Canceled)
	})
}
