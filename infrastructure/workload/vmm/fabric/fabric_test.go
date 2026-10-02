package fabric

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestCheckForwarding(t *testing.T) {
	t.Parallel()

	t.Run("a namespace that forwards is taken as it is", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "ip_forward")
		require.NoError(t, os.WriteFile(path, []byte("1\n"), 0o644))

		assert.NoError(t, checkForwarding(path))
	})

	t.Run("a namespace that does not is refused, and left as it is", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "ip_forward")
		require.NoError(t, os.WriteFile(path, []byte("0\n"), 0o644))

		err := checkForwarding(path)
		assert.ErrorIs(t, err, vm.ErrUnavailable)
		assert.ErrorContains(t, err, "workload-vmnet", "it says where forwarding is set")

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "0\n", string(content), "the fabric never turns forwarding on itself")
	})

	t.Run("a namespace whose forwarding cannot be read is refused", func(t *testing.T) {
		t.Parallel()

		assert.ErrorIs(t, checkForwarding(filepath.Join(t.TempDir(), "missing")), vm.ErrUnavailable)
	})
}
