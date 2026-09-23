package firecracker

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/runner/machine"
	"github.com/khanzadimahdi/testproject/domain/runner/network"
	machineMock "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/machine"
)

const owner = "runner-orchestrator-01"

func isolated() machine.Network {
	return machine.Network{Owner: owner, Name: network.IsolatedNetworkName, Subnet: "10.200.0.0/24", Gateway: "10.200.0.1"}
}

func TestNetworks(t *testing.T) {
	t.Run("addresses are handed out from the first after the host's, one each", func(t *testing.T) {
		var launcher machineMock.MockLauncher
		launcher.On("EnsureNetwork", mock.Anything, owner, network.IsolatedNetworkName, false).Return(isolated(), nil)

		networks, err := NewNetworks(&launcher, owner, t.TempDir())
		require.NoError(t, err)

		first, err := networks.lease(context.Background(), network.Attachment{Name: network.IsolatedNetworkName}, "0000000000000001")
		require.NoError(t, err)

		second, err := networks.lease(context.Background(), network.Attachment{Name: network.IsolatedNetworkName, Aliases: []string{"db"}}, "0000000000000002")
		require.NoError(t, err)

		assert.Equal(t, iface{Network: network.IsolatedNetworkName, Address: "10.200.0.2/24", MAC: "06:00:0a:c8:00:02"}, first)
		assert.Equal(t, "10.200.0.3/24", second.Address)
		assert.Equal(t, []string{"db"}, second.Aliases)
		assert.Empty(t, first.Gateway, "a network that is not the way out is not routed through")

		again, err := networks.lease(context.Background(), network.Attachment{Name: network.IsolatedNetworkName}, "0000000000000001")
		require.NoError(t, err)
		assert.Equal(t, first.Address, again.Address, "a machine asking again keeps what it holds")

		require.NoError(t, networks.release("0000000000000001"))

		third, err := networks.lease(context.Background(), network.Attachment{Name: network.IsolatedNetworkName}, "0000000000000003")
		require.NoError(t, err)
		assert.Equal(t, "10.200.0.2/24", third.Address, "what is released is handed out again")
	})

	t.Run("the network that routes out is routed through, and masqueraded by the host", func(t *testing.T) {
		public := machine.Network{Owner: owner, Name: network.PublicNetworkName, Subnet: "10.200.1.0/24", Gateway: "10.200.1.1", Masquerade: true}

		var launcher machineMock.MockLauncher
		launcher.On("EnsureNetwork", mock.Anything, owner, network.PublicNetworkName, true).Once().Return(public, nil)
		defer launcher.AssertExpectations(t)

		networks, err := NewNetworks(&launcher, owner, t.TempDir())
		require.NoError(t, err)

		leased, err := networks.lease(context.Background(), network.Attachment{Name: network.PublicNetworkName, Gateway: true}, "0000000000000001")
		require.NoError(t, err)

		assert.Equal(t, "10.200.1.1", leased.Gateway)
	})

	t.Run("a network made again is a new one, and nothing handed out before holds on it", func(t *testing.T) {
		dir := t.TempDir()

		var launcher machineMock.MockLauncher
		launcher.On("EnsureNetwork", mock.Anything, owner, network.IsolatedNetworkName, false).Once().Return(isolated(), nil)

		networks, err := NewNetworks(&launcher, owner, dir)
		require.NoError(t, err)

		_, err = networks.lease(context.Background(), network.Attachment{Name: network.IsolatedNetworkName}, "0000000000000001")
		require.NoError(t, err)

		// the host restarted, and gave the network another subnet.
		moved := isolated()
		moved.Subnet, moved.Gateway = "10.200.7.0/24", "10.200.7.1"
		launcher.On("EnsureNetwork", mock.Anything, owner, network.IsolatedNetworkName, false).Return(moved, nil)

		reopened, err := NewNetworks(&launcher, owner, dir)
		require.NoError(t, err)

		leased, err := reopened.lease(context.Background(), network.Attachment{Name: network.IsolatedNetworkName}, "0000000000000002")
		require.NoError(t, err)
		assert.Equal(t, "10.200.7.2/24", leased.Address)
	})

	t.Run("a stack's network is taken away once nothing holds an address on it", func(t *testing.T) {
		stack := machine.Network{Owner: owner, Name: network.StackNetworkName("shop"), Subnet: "10.200.2.0/24", Gateway: "10.200.2.1"}

		var launcher machineMock.MockLauncher
		launcher.On("EnsureNetwork", mock.Anything, owner, stack.Name, false).Return(stack, nil)
		launcher.On("RemoveNetwork", mock.Anything, owner, stack.Name).Once().Return(nil)
		defer launcher.AssertExpectations(t)

		networks, err := NewNetworks(&launcher, owner, t.TempDir())
		require.NoError(t, err)

		_, err = networks.lease(context.Background(), network.Attachment{Name: stack.Name}, "0000000000000001")
		require.NoError(t, err)

		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = networks.release("0000000000000001")
		}()

		require.NoError(t, networks.RemoveStackNetwork(context.Background(), "shop"))
	})
}
