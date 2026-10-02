package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
)

func TestNames(t *testing.T) {
	t.Parallel()

	t.Run("a class with no prefix calls its networks what they always were", func(t *testing.T) {
		t.Parallel()

		names, err := NewNames("")
		require.NoError(t, err)

		for _, name := range []string{
			network.IsolatedNetworkName,
			network.StackNetworkName("shop-abcde"),
			network.PublicNetworkName,
			network.NoNetworkName,
		} {
			assert.Equal(t, name, names.Docker(name))
			assert.Equal(t, name, names.Workload(name))
		}
	})

	t.Run("a prefix keeps a class's own networks apart, and is taken off again", func(t *testing.T) {
		t.Parallel()

		names, err := NewNames("gvisor-")
		require.NoError(t, err)

		assert.Equal(t, "gvisor-workload-isolated", names.Docker(network.IsolatedNetworkName))
		assert.Equal(t, "gvisor-workload-stack-shop-abcde", names.Docker(network.StackNetworkName("shop-abcde")))

		assert.Equal(t, network.IsolatedNetworkName, names.Workload("gvisor-workload-isolated"))
		assert.Equal(t, network.StackNetworkName("shop-abcde"), names.Workload("gvisor-workload-stack-shop-abcde"))
	})

	t.Run("docker's own networks are docker's, whatever the class", func(t *testing.T) {
		t.Parallel()

		names, err := NewNames("gvisor-")
		require.NoError(t, err)

		assert.Equal(t, network.PublicNetworkName, names.Docker(network.PublicNetworkName))
		assert.Equal(t, network.NoNetworkName, names.Docker(network.NoNetworkName))
		assert.Equal(t, network.PublicNetworkName, names.Workload(network.PublicNetworkName))
	})

	t.Run("what cannot start a network's name is refused", func(t *testing.T) {
		t.Parallel()

		for _, prefix := range []string{"-gvisor", "gv/", "g v", "ünïcode"} {
			_, err := NewNames(prefix)
			assert.Error(t, err, prefix)
		}
	})
}
