package fabric

import (
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func cidr(t *testing.T, value string) *net.IPNet {
	t.Helper()

	_, parsed, err := net.ParseCIDR(value)
	require.NoError(t, err)

	return parsed
}

func TestNames(t *testing.T) {
	t.Parallel()

	const (
		id    = "0123456789abcdef"
		other = "fedcba9876543210"
	)

	t.Run("every device name fits in the fifteen characters the kernel allows", func(t *testing.T) {
		t.Parallel()

		assert.Len(t, bridgeName("workload-stack-"+strings.Repeat("a", 48)), 15)
		assert.Len(t, tapName(id, 0), 15)
		assert.Len(t, tapName(id, maxDevices-1), 15)
	})

	t.Run("a network is one bridge, whoever asks for it", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, bridgeName("workload-isolated"), bridgeName("workload-isolated"))
		assert.NotEqual(t, bridgeName("workload-isolated"), bridgeName(vm.PublicNetwork))
		assert.True(t, strings.HasPrefix(bridgeName(vm.PublicNetwork), bridgePrefix))
	})

	t.Run("a machine's taps share what they are named with, and nobody else's do", func(t *testing.T) {
		t.Parallel()

		assert.True(t, strings.HasPrefix(tapName(id, 0), tapPrefixOf(id)))
		assert.True(t, strings.HasPrefix(tapName(id, 15), tapPrefixOf(id)))
		assert.NotEqual(t, tapName(id, 0), tapName(id, 1))
		assert.False(t, strings.HasPrefix(tapName(other, 0), tapPrefixOf(id)))
	})

	t.Run("a tap of the fabric's is told from everything else", func(t *testing.T) {
		t.Parallel()

		assert.True(t, isTap(tapName(id, 3)))
		assert.False(t, isTap(bridgeName("workload-isolated")))
		assert.False(t, isTap("wkt0"), "a name that starts the same is not one")
		assert.False(t, isTap("eth0"))
	})

	t.Run("a bridge's alias reads back as what it was written from", func(t *testing.T) {
		t.Parallel()

		name, masquerade, ok := parseAlias(alias(vm.PublicNetwork, true))
		assert.True(t, ok)
		assert.Equal(t, vm.PublicNetwork, name)
		assert.True(t, masquerade)

		name, masquerade, ok = parseAlias(alias("workload-stack-shop", false))
		assert.True(t, ok)
		assert.Equal(t, "workload-stack-shop", name)
		assert.False(t, masquerade)

		for _, foreign := range []string{
			"",
			"somebody else's bridge",
			"runner workload-isolated masquerade=false",
			"workload Not-A-Network masquerade=false",
			"workload workload-isolated masquerade=maybe",
			"workload workload-isolated",
		} {
			_, _, ok := parseAlias(foreign)
			assert.False(t, ok, foreign)
		}
	})

	t.Run("a tap says whose it is", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "workload-vm 0123456789abcdef public", tapAlias(id, vm.PublicNetwork))
	})
}

func TestAllocate(t *testing.T) {
	t.Parallel()

	pool := cidr(t, "10.250.0.0/16")

	t.Run("the first /24 nobody holds is the next network's", func(t *testing.T) {
		t.Parallel()

		subnet, err := allocate(pool, []*net.IPNet{cidr(t, "10.250.0.0/24"), cidr(t, "10.250.2.0/24")}, nil)

		require.NoError(t, err)
		assert.Equal(t, "10.250.1.0/24", subnet.String())
		assert.Equal(t, "10.250.1.1", gateway(subnet).String())
	})

	t.Run("a network made again is given the subnet it had, when nobody holds it", func(t *testing.T) {
		t.Parallel()

		subnet, err := allocate(pool, []*net.IPNet{cidr(t, "10.250.0.0/24")}, cidr(t, "10.250.7.0/24"))
		require.NoError(t, err)
		assert.Equal(t, "10.250.7.0/24", subnet.String())

		subnet, err = allocate(pool, []*net.IPNet{cidr(t, "10.250.7.0/24")}, cidr(t, "10.250.7.0/24"))
		require.NoError(t, err)
		assert.Equal(t, "10.250.0.0/24", subnet.String(), "and the first free one when somebody does")

		subnet, err = allocate(pool, nil, cidr(t, "10.99.7.0/24"))
		require.NoError(t, err)
		assert.Equal(t, "10.250.0.0/24", subnet.String(), "one outside the pool is not given")
	})

	t.Run("a pool spanning more than one /16 is counted across", func(t *testing.T) {
		t.Parallel()

		taken := make([]*net.IPNet, 0, 256)
		for i := range 256 {
			taken = append(taken, &net.IPNet{IP: net.IPv4(10, 250, byte(i), 0).To4(), Mask: net.CIDRMask(24, 32)})
		}

		subnet, err := allocate(cidr(t, "10.250.0.0/15"), taken, nil)

		require.NoError(t, err)
		assert.Equal(t, "10.251.0.0/24", subnet.String())
	})

	t.Run("a full pool says so", func(t *testing.T) {
		t.Parallel()

		_, err := allocate(cidr(t, "10.250.0.0/24"), []*net.IPNet{cidr(t, "10.250.0.0/24")}, nil)

		assert.ErrorIs(t, err, vm.ErrCapacity)
	})

	t.Run("a pool smaller than one network cannot be divided", func(t *testing.T) {
		t.Parallel()

		_, err := allocate(cidr(t, "10.250.0.0/25"), nil, nil)

		assert.Error(t, err)
	})
}

func TestMAC(t *testing.T) {
	t.Parallel()

	mac := macOf(net.ParseIP("10.250.0.2"))

	assert.Equal(t, "06:00:0a:fa:00:02", mac.String())
	assert.Equal(t, byte(0x02), mac[0]&0x03, "locally administered, and not multicast")
	assert.Equal(t, "06:00:0a:fa:03:01", macOf(gateway(cidr(t, "10.250.3.0/24"))).String(), "a bridge's is its gateway's")
}

func TestValidate(t *testing.T) {
	t.Parallel()

	valid := func() Config {
		return Config{Dir: "/var/lib/workload-vmhost/fabric", Pool: cidr(t, "10.250.0.0/16"), FirstUID: 1_000_000_000, UIDs: 65536}
	}

	assert.NoError(t, valid().validate())

	development := valid()
	development.FirstUID, development.UIDs = 0, 0
	assert.NoError(t, development.validate(), "machines may run as vmhost itself, for development")

	for reason, change := range map[string]func(*Config){
		"a relative directory":   func(c *Config) { c.Dir = "fabric" },
		"no pool":                func(c *Config) { c.Pool = nil },
		"an IPv6 pool":           func(c *Config) { c.Pool = cidr(t, "fd00::/64") },
		"a pool smaller than 24": func(c *Config) { c.Pool = cidr(t, "10.250.0.0/25") },
		"machines as root":       func(c *Config) { c.FirstUID = 0 },
		"no users at all":        func(c *Config) { c.UIDs = -1 },
		"users past 2^32":        func(c *Config) { c.FirstUID = 1 << 32 },
	} {
		config := valid()
		change(&config)

		assert.Error(t, config.validate(), reason)
	}
}

func TestValidatePlug(t *testing.T) {
	t.Parallel()

	const id = "0123456789abcdef"

	assert.NoError(t, validatePlug(id, nil), "a machine may join no network at all")
	assert.NoError(t, validatePlug(id, []vm.Attachment{{Network: "workload-stack-shop", Aliases: []string{"db"}}, {Network: vm.PublicNetwork, Gateway: true}}))

	tooMany := make([]vm.Attachment, maxDevices+1)
	for i := range tooMany {
		tooMany[i] = vm.Attachment{Network: "net-" + string(rune('a'+i))}
	}

	for reason, attachments := range map[string][]vm.Attachment{
		"a network that cannot be one":   {{Network: "Not A Network"}},
		"the same network twice":         {{Network: "workload-isolated"}, {Network: "workload-isolated"}},
		"two ways out":                   {{Network: vm.PublicNetwork, Gateway: true}, {Network: "workload-isolated", Gateway: true}},
		"more devices than can be named": tooMany,
	} {
		assert.ErrorIs(t, validatePlug(id, attachments), vm.ErrInvalid, reason)
	}

	assert.ErrorIs(t, validatePlug("machine-1", nil), vm.ErrInvalid, "a machine is called what vmhost calls it")
}

func TestOwner(t *testing.T) {
	t.Parallel()

	machines := Config{FirstUID: 1_000_000_000, UIDs: 65536}

	owner, err := machines.owner(1_000_000_005, 0)
	require.NoError(t, err)
	assert.Equal(t, 1_000_000_005, owner)

	for _, uid := range []int{0, 999_999_999, 1_000_065_536} {
		_, err := machines.owner(uid, 0)
		assert.ErrorIs(t, err, vm.ErrInvalid, "user %d is not one machines run as", uid)
	}

	development := Config{}

	owner, err = development.owner(0, 1000)
	require.NoError(t, err)
	assert.Equal(t, 1000, owner, "a machine that runs as vmhost has its taps made for vmhost")

	_, err = development.owner(1_000_000_005, 1000)
	assert.ErrorIs(t, err, vm.ErrInvalid)
}
