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
		assert.Equal(t, archiveHeader{Engine: "microsandbox/0.7.6", Kind: vm.KindDocker, Image: "docker:29-dind", Disk: 4 << 30}, header)

		read, err := io.ReadAll(rest)
		require.NoError(t, err)
		assert.Equal(t, saved, read)
	})

	t.Run("fields it does not know are passed over", func(t *testing.T) {
		t.Parallel()

		header, rest, err := readHeader(strings.NewReader(`{"engine":"microsandbox/0.7.6","later":{"a":[1,2]},"kind":"machine"}` + "\nrest"))
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

		_, err := other.Create(t.Context(), vm.Spec{ID: "vm-1", Kind: vm.KindMachine, Image: "ubuntu:24.04"})
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

		_, _, err := readHeader(strings.NewReader(`{"engine":"microsandbox/0.7.5","kind":"machine"}` + "\n"))
		assert.ErrorIs(t, err, vm.ErrEngineMismatch)
	})

	t.Run("what is not an archive at all is refused", func(t *testing.T) {
		t.Parallel()

		for _, archive := range []string{"", "\x28\xb5\x2f\xfd binary", `{"kind":"machine","engine":"microsandbox/0.7.6"}`, `["engine"]`} {
			_, _, err := readHeader(strings.NewReader(archive))
			assert.ErrorIs(t, err, vm.ErrEngineMismatch, "%q", archive)
		}
	})

	t.Run("a header that does not end its line is refused", func(t *testing.T) {
		t.Parallel()

		_, _, err := readHeader(strings.NewReader(`{"engine":"microsandbox/0.7.6"}rest`))
		assert.Error(t, err)
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
