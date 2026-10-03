package fabric

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestLeases(t *testing.T) {
	t.Parallel()

	const (
		isolated = "workload-isolated"
		first    = "0000000000000001"
		second   = "0000000000000002"
		third    = "0000000000000003"
	)

	t.Run("addresses are handed out from the first after the fabric's own, one each", func(t *testing.T) {
		t.Parallel()

		book, err := openLeases(t.TempDir())
		require.NoError(t, err)

		subnet := cidr(t, "10.250.0.0/24")

		a, err := book.hand(isolated, subnet, first)
		require.NoError(t, err)

		b, err := book.hand(isolated, subnet, second)
		require.NoError(t, err)

		assert.Equal(t, "10.250.0.2", a.String())
		assert.Equal(t, "10.250.0.3", b.String())

		again, err := book.hand(isolated, subnet, first)
		require.NoError(t, err)
		assert.Equal(t, a, again, "a machine asking again keeps what it holds")

		book.release(first)

		c, err := book.hand(isolated, subnet, third)
		require.NoError(t, err)
		assert.Equal(t, "10.250.0.2", c.String(), "what is given back is handed out again")
	})

	t.Run("what is handed out outlives the fabric that handed it out", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		subnet := cidr(t, "10.250.4.0/24")

		book, err := openLeases(dir)
		require.NoError(t, err)

		_, err = book.hand(isolated, subnet, first)
		require.NoError(t, err)
		require.NoError(t, book.save())

		reopened, err := openLeases(dir)
		require.NoError(t, err)

		next, err := reopened.hand(isolated, subnet, second)
		require.NoError(t, err)
		assert.Equal(t, "10.250.4.3", next.String(), "an address held before a restart is not handed out again")

		held, err := reopened.hand(isolated, subnet, first)
		require.NoError(t, err)
		assert.Equal(t, "10.250.4.2", held.String())

		assert.Equal(t, subnet.String(), reopened.subnetOf(isolated).String(), "and the network's subnet is remembered")
		assert.True(t, reopened.inUse(isolated))
	})

	t.Run("a network made again on another subnet is a new one, and nothing handed out before holds on it", func(t *testing.T) {
		t.Parallel()

		book, err := openLeases(t.TempDir())
		require.NoError(t, err)

		_, err = book.hand(isolated, cidr(t, "10.250.0.0/24"), first)
		require.NoError(t, err)

		moved, err := book.hand(isolated, cidr(t, "10.250.7.0/24"), second)
		require.NoError(t, err)

		assert.Equal(t, "10.250.7.2", moved.String())
	})

	t.Run("a full network says so", func(t *testing.T) {
		t.Parallel()

		book, err := openLeases(t.TempDir())
		require.NoError(t, err)

		subnet := cidr(t, "10.250.0.0/24")

		for i := range 253 {
			_, err := book.hand(isolated, subnet, "machine"+string(rune(i)))
			require.NoError(t, err)
		}

		_, err = book.hand(isolated, subnet, first)
		assert.ErrorIs(t, err, vm.ErrCapacity)
	})

	t.Run("what machines that are gone held is given back, and said who they were", func(t *testing.T) {
		t.Parallel()

		book, err := openLeases(t.TempDir())
		require.NoError(t, err)

		for _, id := range []string{first, second, third} {
			_, err := book.hand(isolated, cidr(t, "10.250.0.0/24"), id)
			require.NoError(t, err)
		}

		_, err = book.hand(vm.PublicNetwork, cidr(t, "10.250.1.0/24"), third)
		require.NoError(t, err)

		released := book.retain(func(id string) bool { return id == second })

		assert.Equal(t, []string{first, third}, released)
		assert.Empty(t, book.retain(func(id string) bool { return id == second }), "nothing is given back twice")
		assert.False(t, book.inUse(vm.PublicNetwork))
		assert.True(t, book.inUse(isolated))
	})

	t.Run("a machine keeps what it holds only on the networks it still joins", func(t *testing.T) {
		t.Parallel()

		book, err := openLeases(t.TempDir())
		require.NoError(t, err)

		_, err = book.hand(isolated, cidr(t, "10.250.0.0/24"), first)
		require.NoError(t, err)
		_, err = book.hand(vm.PublicNetwork, cidr(t, "10.250.1.0/24"), first)
		require.NoError(t, err)

		book.keepOnly(first, []string{vm.PublicNetwork})

		assert.False(t, book.inUse(isolated))
		assert.True(t, book.inUse(vm.PublicNetwork))
	})

	t.Run("a network taken away is forgotten", func(t *testing.T) {
		t.Parallel()

		book, err := openLeases(t.TempDir())
		require.NoError(t, err)

		book.on(isolated, cidr(t, "10.250.0.0/24"))
		book.forget(isolated)

		assert.Nil(t, book.subnetOf(isolated))
	})

	t.Run("a file that cannot be read is said to be so, rather than handing every address out again", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, leasesName), []byte("{"), 0o600))

		_, err := openLeases(dir)

		assert.Error(t, err)
	})
}
