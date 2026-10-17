package microsandbox

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

func TestArchiveHeader(t *testing.T) {
	t.Parallel()

	t.Run("what follows the header is what microsandbox saved, byte for byte", func(t *testing.T) {
		t.Parallel()

		saved := []byte{0x28, 0xb5, 0x2f, 0xfd, '\n', '{', 0x00, 0xff}

		var archive bytes.Buffer
		written, err := writeHeader(&archive, archiveHeader{Engine: archiveEngine(), Kind: vm.KindDocker, Image: "docker:29-dind", Disk: 4 << 30})
		require.NoError(t, err)
		assert.Equal(t, int64(archive.Len()), written)

		archive.Write(saved)

		header, rest, err := readHeader(&archive)
		require.NoError(t, err)
		assert.Equal(t, archiveHeader{Engine: "microsandbox/0.7.8", Kind: vm.KindDocker, Image: "docker:29-dind", Disk: 4 << 30}, header)

		read, err := io.ReadAll(rest)
		require.NoError(t, err)
		assert.Equal(t, saved, read)
	})

	t.Run("fields it does not know are passed over", func(t *testing.T) {
		t.Parallel()

		header, rest, err := readHeader(strings.NewReader(`{"engine":"microsandbox/0.7.8","later":{"a":[1,2]},"kind":"machine"}` + "\nrest"))
		require.NoError(t, err)
		assert.Equal(t, vm.KindMachine, header.Kind)

		read, err := io.ReadAll(rest)
		require.NoError(t, err)
		assert.Equal(t, "rest", string(read))
	})

	t.Run("an archive another engine wrote is refused before it is read through", func(t *testing.T) {
		t.Parallel()

		// the shape of the memory engine's archive: its whole disk is on its
		// first line, after what wrote it.
		other, err := json.Marshal(struct {
			Engine string `json:"engine"`
			Disk   string `json:"disk"`
		}{Engine: "memory/1", Disk: strings.Repeat("A", 1<<20)})
		require.NoError(t, err)

		reader := &countingReader{reader: bytes.NewReader(other)}

		_, _, err = readHeader(reader)
		assert.ErrorIs(t, err, vm.ErrEngineMismatch)
		assert.ErrorContains(t, err, `"memory/1"`)
		assert.Less(t, reader.read, 64<<10, "the rest of it is never read")
	})

	t.Run("the memory engine's own archive is refused", func(t *testing.T) {
		t.Parallel()

		other := memory.New()

		_, err := other.Create(t.Context(), vm.Spec{ID: "vm-1", Image: "ubuntu:24.04"})
		require.NoError(t, err)
		require.NoError(t, other.SetDisk("vm-1", []byte("a disk")))

		var archive bytes.Buffer
		_, err = other.Snapshot(t.Context(), "vm-1", &archive)
		require.NoError(t, err)

		_, _, err = readHeader(&archive)
		assert.ErrorIs(t, err, vm.ErrEngineMismatch)
		assert.ErrorContains(t, err, `"memory/1"`)
	})

	t.Run("another release of this engine is refused", func(t *testing.T) {
		t.Parallel()

		_, _, err := readHeader(strings.NewReader(`{"engine":"microsandbox/0.7.6","kind":"machine"}` + "\n"))
		assert.ErrorIs(t, err, vm.ErrEngineMismatch)
	})

	t.Run("what is not an archive at all is refused", func(t *testing.T) {
		t.Parallel()

		for _, archive := range []string{"", "\x28\xb5\x2f\xfd binary", `{"kind":"machine","engine":"microsandbox/0.7.8"}`, `["engine"]`} {
			_, _, err := readHeader(strings.NewReader(archive))
			assert.ErrorIs(t, err, vm.ErrEngineMismatch, "%q", archive)
		}
	})

	t.Run("a header that does not end its line is refused", func(t *testing.T) {
		t.Parallel()

		_, _, err := readHeader(strings.NewReader(`{"engine":"microsandbox/0.7.8"}rest`))
		assert.Error(t, err)
	})
}

func TestRestoreDiskOf(t *testing.T) {
	t.Parallel()

	t.Run("a vm's flat disk needs as much as it was given", func(t *testing.T) {
		t.Parallel()

		for _, disk := range []uint64{0, 100 << 20, 1 << 30, 6 << 30} {
			assert.Equal(t, disk, restoreDiskOf(vm.Spec{Image: "alpine:3.20", Resources: vm.Resources{Disk: disk}}), "%d", disk)
		}
	})

	// a run's disk is a managed layer, which microsandbox 0.7.8 grows once
	// it is restored only to 4 GiB and 3 MiB or more, as the KVM suite holds,
	// and its archive says the next whole GiB.
	t.Run("a run's managed disk needs 5 GiB, or as much as it was given past that", func(t *testing.T) {
		t.Parallel()

		run := func(disk uint64) vm.Spec {
			return vm.Spec{Image: "alpine:3.20", Resources: vm.Resources{Disk: disk}, Command: []string{"sh", "-c", "sleep 600"}}
		}

		for disk, want := range map[uint64]uint64{0: 5 << 30, 100 << 20: 5 << 30, 4 << 30: 5 << 30, 4099 << 20: 5 << 30, 5 << 30: 5 << 30, 6 << 30: 6 << 30, 10 << 30: 10 << 30} {
			assert.Equal(t, want, restoreDiskOf(run(disk)), "%d", disk)
		}

		assert.Equal(t, uint64(5<<30), restoreDiskOf(vm.Spec{Resources: vm.Resources{Disk: 100 << 20}, Entrypoint: []string{"node"}}), "an entrypoint is a main process too")
	})
}

type countingReader struct {
	reader io.Reader
	read   int
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += n

	return n, err
}
