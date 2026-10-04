package hostports

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allocator hands out first to last with nothing listening anywhere, at a
// time the test moves.
func allocator(t *testing.T, first, last uint16) (*Allocator, *time.Time) {
	t.Helper()

	a, err := New(first, last, "127.0.0.1")
	require.NoError(t, err)

	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

	a.now = func() time.Time { return now }
	a.free = func(string) bool { return true }

	return a, &now
}

func TestAllocator(t *testing.T) {
	t.Parallel()

	t.Run("ports are handed out once each", func(t *testing.T) {
		t.Parallel()

		a, _ := allocator(t, 20000, 20009)

		first, err := a.Allocate("a", 2)
		require.NoError(t, err)

		second, err := a.Allocate("b", 3)
		require.NoError(t, err)

		assert.Equal(t, []uint16{20000, 20001}, first)
		assert.Equal(t, []uint16{20002, 20003, 20004}, second)
	})

	t.Run("nothing asked for is nothing handed out", func(t *testing.T) {
		t.Parallel()

		a, _ := allocator(t, 20000, 20009)

		ports, err := a.Allocate("a", 0)

		require.NoError(t, err)
		assert.Empty(t, ports)
	})

	t.Run("a range that cannot take a run hands it nothing", func(t *testing.T) {
		t.Parallel()

		a, _ := allocator(t, 20000, 20002)

		_, err := a.Allocate("a", 2)
		require.NoError(t, err)

		_, err = a.Allocate("b", 2)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "only 1 of the 2")

		ports, err := a.Allocate("c", 1)
		require.NoError(t, err, "a refused run took nothing")
		assert.Equal(t, []uint16{20002}, ports)
	})

	t.Run("the ports a run already has are not handed to another", func(t *testing.T) {
		t.Parallel()

		a, _ := allocator(t, 20000, 20003)

		a.Hold("a", []uint16{20000, 20002})

		ports, err := a.Allocate("b", 2)
		require.NoError(t, err)

		assert.Equal(t, []uint16{20001, 20003}, ports)
	})

	t.Run("ports let go of are quarantined, and handed out last", func(t *testing.T) {
		t.Parallel()

		a, now := allocator(t, 20000, 20003)

		first, err := a.Allocate("a", 2)
		require.NoError(t, err)

		a.Release("a")

		ports, err := a.Allocate("b", 2)
		require.NoError(t, err)
		assert.Equal(t, []uint16{20002, 20003}, ports)

		_, err = a.Allocate("c", 1)
		assert.Error(t, err, "a listener of the sandbox they belonged to may still hold them")

		*now = now.Add(DefaultQuarantine)

		ports, err = a.Allocate("c", 2)
		require.NoError(t, err)
		assert.Equal(t, first, ports)
	})

	t.Run("a port something listens on is skipped", func(t *testing.T) {
		t.Parallel()

		a, err := New(1, 65535, "127.0.0.1")
		require.NoError(t, err)

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		defer listener.Close()

		taken := uint16(listener.Addr().(*net.TCPAddr).Port)

		a.next = taken

		ports, err := a.Allocate("a", 1)
		require.NoError(t, err)

		assert.NotEqual(t, taken, ports[0])
		assert.True(t, testBind(net.JoinHostPort("127.0.0.1", strconv.Itoa(int(ports[0])))), "what is handed out can be bound")
	})

	t.Run("allocation goes round the range", func(t *testing.T) {
		t.Parallel()

		a, now := allocator(t, 65534, 65535)

		ports, err := a.Allocate("a", 2)
		require.NoError(t, err)
		assert.Equal(t, []uint16{65534, 65535}, ports)

		a.Release("a")
		*now = now.Add(DefaultQuarantine)

		ports, err = a.Allocate("b", 2)
		require.NoError(t, err)
		assert.ElementsMatch(t, []uint16{65534, 65535}, ports)
	})

	t.Run("a range or an address that is none is refused", func(t *testing.T) {
		t.Parallel()

		_, err := New(0, 10, "127.0.0.1")
		assert.Error(t, err)

		_, err = New(20, 10, "127.0.0.1")
		assert.Error(t, err)

		_, err = New(10, 20, "not an address")
		assert.Error(t, err)
	})
}
