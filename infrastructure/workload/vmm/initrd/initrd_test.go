package initrd

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// entry is one entry of a cpio archive, as read back.
type entry struct {
	name         string
	mode         uint64
	major, minor uint64
	content      []byte
}

// read reads a compressed newc archive back, the way the kernel would.
func read(t *testing.T, archive []byte) []entry {
	t.Helper()

	decompressed, err := gzip.NewReader(bytes.NewReader(archive))
	require.NoError(t, err)

	raw, err := io.ReadAll(decompressed)
	require.NoError(t, err)

	var entries []entry

	for offset := 0; ; {
		require.GreaterOrEqual(t, len(raw)-offset, 110, "the archive ends in the middle of a header")
		require.Equal(t, "070701", string(raw[offset:offset+6]))

		field := func(i int) uint64 {
			value, err := strconv.ParseUint(string(raw[offset+6+i*8:offset+14+i*8]), 16, 64)
			require.NoError(t, err)

			return value
		}

		mode, size, major, minor, nameSize := field(1), field(6), field(9), field(10), field(11)

		name := string(raw[offset+110 : offset+110+int(nameSize)-1])
		offset = align(offset + 110 + int(nameSize))

		if name == trailer {
			return entries
		}

		entries = append(entries, entry{name: name, mode: mode, major: major, minor: minor, content: raw[offset : offset+int(size)]})
		offset = align(offset + int(size))
	}
}

func align(offset int) int {
	return (offset + 3) &^ 3
}

func TestBuild(t *testing.T) {
	t.Run("init is the agent, and the directories it mounts over are there", func(t *testing.T) {
		agent := []byte("#!the agent, an odd number of bytes long")

		var archive bytes.Buffer
		require.NoError(t, Build(&archive, bytes.NewReader(agent), int64(len(agent))))

		entries := read(t, archive.Bytes())

		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.name
		}

		assert.Equal(t, []string{"dev", "mnt", "proc", "run", "sys", "tmp", "dev/console", "dev/null", "init"}, names)

		init := entries[len(entries)-1]
		assert.Equal(t, agent, init.content)
		assert.Equal(t, uint64(modeRegular|0o755), init.mode)
		assert.Equal(t, uint64(modeDirectory|0o755), entries[0].mode)
	})

	t.Run("the console and /dev/null are there before anything is mounted", func(t *testing.T) {
		var archive bytes.Buffer
		require.NoError(t, Build(&archive, bytes.NewReader(nil), 0))

		found := make(map[string]entry)
		for _, e := range read(t, archive.Bytes()) {
			found[e.name] = e
		}

		console := found["dev/console"]
		assert.Equal(t, uint64(modeCharacter|0o600), console.mode)
		assert.Equal(t, [2]uint64{5, 1}, [2]uint64{console.major, console.minor})
		assert.Empty(t, console.content)

		null := found["dev/null"]
		assert.Equal(t, uint64(modeCharacter|0o666), null.mode)
		assert.Equal(t, [2]uint64{1, 3}, [2]uint64{null.major, null.minor})
	})
}

func TestEnsure(t *testing.T) {
	t.Run("an initramfs is named by the agent it holds, and built once", func(t *testing.T) {
		dir := t.TempDir()

		agentPath := filepath.Join(dir, "agent")
		require.NoError(t, os.WriteFile(agentPath, []byte("agent"), 0o755))

		path, err := Ensure(filepath.Join(dir, "boot"), agentPath)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(filepath.Base(path), "initrd-"))

		built, err := os.Stat(path)
		require.NoError(t, err)

		again, err := Ensure(filepath.Join(dir, "boot"), agentPath)
		require.NoError(t, err)
		assert.Equal(t, path, again)

		unchanged, err := os.Stat(again)
		require.NoError(t, err)
		assert.Equal(t, built.ModTime(), unchanged.ModTime(), "the one already there is used rather than built again")

		require.NoError(t, os.WriteFile(agentPath, []byte("another agent"), 0o755))

		other, err := Ensure(filepath.Join(dir, "boot"), agentPath)
		require.NoError(t, err)
		assert.NotEqual(t, path, other, "a different agent is a different initramfs")
	})

	t.Run("an agent that is not there says so", func(t *testing.T) {
		_, err := Ensure(t.TempDir(), filepath.Join(t.TempDir(), "missing"))

		assert.Error(t, err)
	})
}
