package hostnet

import (
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cidr(t *testing.T, value string) *net.IPNet {
	t.Helper()

	_, parsed, err := net.ParseCIDR(value)
	require.NoError(t, err)

	return parsed
}

func TestNames(t *testing.T) {
	t.Run("every device name fits in the fifteen characters the kernel allows", func(t *testing.T) {
		assert.LessOrEqual(t, len(bridgeName("runner-orchestrator-01", "runner-stack-a-very-long-stack-slug-indeed")), 15)
		assert.LessOrEqual(t, len(tapName("0123456789abcdef", 9)), 15)
	})

	t.Run("a network is its owner's: two orchestrators' networks of one name are two bridges", func(t *testing.T) {
		assert.NotEqual(t, bridgeName("runner-orchestrator-01", "runner-isolated"), bridgeName("runner-orchestrator-02", "runner-isolated"))
		assert.Equal(t, bridgeName("runner-orchestrator-01", "runner-isolated"), bridgeName("runner-orchestrator-01", "runner-isolated"))
	})

	t.Run("a machine's taps share what they are named with, and nobody else's do", func(t *testing.T) {
		assert.True(t, strings.HasPrefix(tapName("0123456789abcdef", 0), tapPrefixOf("0123456789abcdef")))
		assert.True(t, strings.HasPrefix(tapName("0123456789abcdef", 1), tapPrefixOf("0123456789abcdef")))
		assert.False(t, strings.HasPrefix(tapName("fedcba9876543210", 0), tapPrefixOf("0123456789abcdef")))
	})

	t.Run("a bridge's alias reads back as what it was written from", func(t *testing.T) {
		owner, name, masquerade, ok := parseAlias(alias("runner-orchestrator-01", "runner-public", true))

		assert.True(t, ok)
		assert.Equal(t, "runner-orchestrator-01", owner)
		assert.Equal(t, "runner-public", name)
		assert.True(t, masquerade)

		_, _, _, ok = parseAlias("somebody else's bridge")
		assert.False(t, ok)
	})
}

func TestAllocate(t *testing.T) {
	t.Run("the first /24 nobody holds is the next network's", func(t *testing.T) {
		pool := cidr(t, "10.200.0.0/16")

		subnet, err := allocate(pool, []*net.IPNet{cidr(t, "10.200.0.0/24"), cidr(t, "10.200.2.0/24")})

		require.NoError(t, err)
		assert.Equal(t, "10.200.1.0/24", subnet.String())
		assert.Equal(t, "10.200.1.1", gateway(subnet).String())
	})

	t.Run("a full pool says so", func(t *testing.T) {
		_, err := allocate(cidr(t, "10.200.0.0/24"), []*net.IPNet{cidr(t, "10.200.0.0/24")})

		assert.Error(t, err)
	})

	t.Run("a pool smaller than one network cannot be divided", func(t *testing.T) {
		_, err := allocate(cidr(t, "10.200.0.0/25"), nil)

		assert.Error(t, err)
	})
}

func TestRules(t *testing.T) {
	pool := cidr(t, "10.200.0.0/16")

	networks := []networkState{
		{owner: "runner-orchestrator-01", name: "runner-public", subnet: cidr(t, "10.200.1.0/24"), masquerade: true},
		{owner: "runner-orchestrator-01", name: "runner-isolated", subnet: cidr(t, "10.200.0.0/24")},
	}

	specs := func(chain string) []string {
		var result []string
		for _, r := range rules(pool, networks) {
			if r.chain == chain {
				result = append(result, strings.Join(r.spec, " "))
			}
		}

		return result
	}

	t.Run("machines reach their own network's neighbours, and a public network reaches out", func(t *testing.T) {
		assert.Equal(t, []string{
			"-m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT",
			"-s 10.200.0.0/24 -d 10.200.0.0/24 -j ACCEPT",
			"-s 10.200.1.0/24 -d 10.200.1.0/24 -j DROP",
			"-s 10.200.1.0/24 ! -d 10.200.0.0/16 -j ACCEPT",
			"-s 10.200.0.0/16 -j DROP",
			"-d 10.200.0.0/16 -j DROP",
		}, specs(forwardChain))
	})

	t.Run("only a public network is masqueraded", func(t *testing.T) {
		assert.Equal(t, []string{"-s 10.200.1.0/24 ! -d 10.200.0.0/16 -j MASQUERADE"}, specs(postroutingChain))
	})

	t.Run("nothing a machine starts reaches the host", func(t *testing.T) {
		assert.Equal(t, []string{
			"-i rnb+ -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT",
			"-i rnb+ -j DROP",
		}, specs(inputChain))
	})

	t.Run("forwarded traffic goes through docker's own chain for rules like these, when docker is there", func(t *testing.T) {
		assert.Equal(t, "DOCKER-USER", jumps(true)[0].from)
		assert.Equal(t, "FORWARD", jumps(false)[0].from)
	})
}
