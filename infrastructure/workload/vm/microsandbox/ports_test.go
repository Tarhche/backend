package microsandbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestPortRange(t *testing.T) {
	t.Parallel()

	free := func(port.Port) bool { return true }

	t.Run("no range given is the vmhost's own", func(t *testing.T) {
		t.Parallel()

		r, err := newPortRange(0, 0)
		require.NoError(t, err)
		assert.Equal(t, portRange{first: 20000, last: 29999}, r)
	})

	t.Run("what is not a range is refused", func(t *testing.T) {
		t.Parallel()

		for _, bounds := range [][2]port.Port{{0, 100}, {200, 100}, {60000, 70000}} {
			_, err := newPortRange(bounds[0], bounds[1])
			assert.Error(t, err, "%v", bounds)
		}
	})

	t.Run("new ports are given the lowest host ports nobody has, lowest guest port first", func(t *testing.T) {
		t.Parallel()

		r := portRange{first: 20000, last: 20010}

		given, err := r.assign([]port.Port{8080, 80, 80}, nil, map[port.Port]bool{20000: true}, free)
		require.NoError(t, err)
		assert.Equal(t, map[port.Port]port.Port{80: 20001, 8080: 20002}, given)
	})

	t.Run("a guest port keeps the host port it had, and one it no longer has is let go of", func(t *testing.T) {
		t.Parallel()

		r := portRange{first: 20000, last: 20010}

		given, err := r.assign([]port.Port{80, 443}, map[port.Port]port.Port{80: 20005, 22: 20006}, nil, free)
		require.NoError(t, err)
		assert.Equal(t, map[port.Port]port.Port{80: 20005, 443: 20000}, given)
	})

	t.Run("a host port something else listens on is passed over", func(t *testing.T) {
		t.Parallel()

		r := portRange{first: 20000, last: 20010}

		given, err := r.assign([]port.Port{80}, nil, nil, func(p port.Port) bool { return p != 20000 })
		require.NoError(t, err)
		assert.Equal(t, map[port.Port]port.Port{80: 20001}, given)
	})

	t.Run("a range with nothing left has no capacity", func(t *testing.T) {
		t.Parallel()

		r := portRange{first: 20000, last: 20001}

		_, err := r.assign([]port.Port{80, 81, 82}, nil, nil, free)
		assert.ErrorIs(t, err, vm.ErrNoCapacity)
	})

	t.Run("what is not a port is refused", func(t *testing.T) {
		t.Parallel()

		r := portRange{first: 20000, last: 20010}

		_, err := r.assign([]port.Port{0}, nil, nil, free)
		assert.Error(t, err)

		_, err = r.assign([]port.Port{65536}, nil, nil, free)
		assert.Error(t, err)
	})
}
