package layout

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaths(t *testing.T) {
	t.Parallel()

	const (
		dataDir = "/var/lib/workload-vmhost"
		id      = "0123456789abcdef"
	)

	assert.Equal(t, "/var/lib/workload-vmhost/vms/0123456789abcdef/state.json", State(dataDir, id))
	assert.Equal(t, "/var/lib/workload-vmhost/vms/0123456789abcdef/scratch.ext4", Scratch(dataDir, id))
	assert.Equal(t, "/var/lib/workload-vmhost/vms/0123456789abcdef/output.log", Output(dataDir, id))
	assert.Equal(t, "/var/lib/workload-vmhost/j/0123456789abcdef/root", MachineRoot(dataDir, id))
	assert.Equal(t, "/var/lib/workload-vmhost/j/0123456789abcdef/console.log", Console(dataDir, id))

	// a unix socket's path is at most 108 bytes, terminator included.
	for _, socket := range []string{APISocket(dataDir, id), VsockSocket(dataDir, id)} {
		assert.Less(t, len(socket), 108, socket)
	}
}

func TestPrepare(t *testing.T) {
	t.Parallel()

	dataDir := filepath.Join(t.TempDir(), "vmhost")

	require.NoError(t, Prepare(dataDir))

	// laid out again, it stays as it is.
	require.NoError(t, Prepare(dataDir))

	modes := map[string]os.FileMode{
		dataDir:           0o711,
		Machines(dataDir): 0o711,
		Bin(dataDir):      0o700,
		Boot(dataDir):     0o700,
		Images(dataDir):   0o700,
		VMs(dataDir):      0o700,
		Fabric(dataDir):   0o700,
	}

	for path, mode := range modes {
		info, err := os.Stat(path)
		require.NoError(t, err, path)

		assert.True(t, info.IsDir(), path)
		assert.Equal(t, mode, info.Mode().Perm(), path)
	}

	t.Run("something that is not a directory is refused", func(t *testing.T) {
		t.Parallel()

		file := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(file, nil, 0o600))

		assert.Error(t, Prepare(file))
	})
}
