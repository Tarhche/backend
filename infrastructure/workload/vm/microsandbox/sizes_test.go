package microsandbox

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVCPUs(t *testing.T) {
	t.Parallel()

	for asked, want := range map[uint]uint8{0: 1, 1: 1, 4: 4, 255: 255} {
		got, err := vcpus(asked)
		require.NoError(t, err)
		assert.Equal(t, want, got, "%d", asked)
	}

	_, err := vcpus(256)
	assert.Error(t, err)
}

func TestMebibytes(t *testing.T) {
	t.Parallel()

	for bytes, want := range map[uint64]uint32{0: 0, 1: 1, 1 << 20: 1, 1<<20 + 1: 2, 2 << 30: 2048} {
		got, err := mebibytes(bytes)
		require.NoError(t, err)
		assert.Equal(t, want, got, "%d", bytes)
	}

	_, err := mebibytes(math.MaxUint64)
	assert.Error(t, err)
}

func TestManagedDiskMiB(t *testing.T) {
	t.Parallel()

	// what the earlier spike measured: ceil((limit + 72 MiB) / 0.96), and
	// never less than the smallest disk that boots.
	for limit, want := range map[uint64]uint32{64 << 20: 142, 1 << 30: 1142, 100 << 20: 180, 0: 75, 1: 76} {
		got, err := managedDiskMiB(limit)
		require.NoError(t, err)
		assert.Equal(t, want, got, "%d", limit)
	}

	_, err := managedDiskMiB(math.MaxUint64)
	assert.Error(t, err)
}

func TestParseDF(t *testing.T) {
	t.Parallel()

	used, total, ok := parseDF("Filesystem     1-byte-blocks      Used   Available Capacity Mounted on\n/dev/vda        10464022528 476078080 9971138560       5% /\n")
	assert.True(t, ok)
	assert.Equal(t, uint64(476078080), used)
	assert.Equal(t, uint64(10464022528), total)

	for _, report := range []string{"", "Filesystem 1-byte-blocks Used Available Capacity Mounted on", "header\n/dev/vda x y z 5% /"} {
		_, _, ok := parseDF(report)
		assert.False(t, ok, "%q", report)
	}
}
