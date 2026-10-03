package runtime

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/network"
)

func TestClass(t *testing.T) {
	t.Parallel()

	t.Run("a class that names nothing was written before there were classes", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, Sysbox, Class("").OrSysbox())
		assert.Equal(t, Firecracker, Firecracker.OrSysbox())
	})

	t.Run("what a class can be called", func(t *testing.T) {
		t.Parallel()

		for _, valid := range []Class{"sysbox", "firecracker", "gvisor", "cloud-hypervisor", "k8s"} {
			assert.True(t, valid.IsValid(), valid)
		}

		for _, invalid := range []Class{"", "Sysbox", "fire cracker", "a:b", "-sysbox", "1vm", "a/b"} {
			assert.False(t, invalid.IsValid(), invalid)
		}
	})
}

func TestParseClasses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		given string
		want  []Class
	}{
		{name: "one", given: "sysbox", want: []Class{Sysbox}},
		{name: "several, in order", given: "firecracker,sysbox", want: []Class{Firecracker, Sysbox}},
		{name: "spaces are not part of a class", given: " sysbox , firecracker ", want: []Class{Sysbox, Firecracker}},
		{name: "a class named twice is named once", given: "sysbox,sysbox", want: []Class{Sysbox}},
		{name: "none at all", given: " , ", want: []Class{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseClasses(tt.given)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("something that cannot be a class is refused", func(t *testing.T) {
		t.Parallel()

		_, err := ParseClasses("sysbox,Fire Cracker")
		assert.Error(t, err)
	})
}

func TestQualify(t *testing.T) {
	t.Parallel()

	t.Run("a sysbox run keeps the name docker gave it", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "3f2a9c", Qualify(Sysbox, "3f2a9c"))
		assert.Equal(t, "3f2a9c", Qualify("", "3f2a9c"))
	})

	t.Run("any other run carries its class", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "firecracker:0123456789abcdef", Qualify(Firecracker, "0123456789abcdef"))
	})

	t.Run("a name taken apart is the name that was put together", func(t *testing.T) {
		t.Parallel()

		for _, given := range []struct {
			class  Class
			native string
		}{
			{class: Sysbox, native: "3f2a9c"},
			{class: Firecracker, native: "0123456789abcdef"},
			{class: "gvisor", native: "77aa"},
		} {
			class, native := Split(Qualify(given.class, given.native))

			assert.Equal(t, given.class, class)
			assert.Equal(t, given.native, native)
		}
	})

	t.Run("a bare name is sysbox's", func(t *testing.T) {
		t.Parallel()

		class, native := Split("3f2a9c")

		assert.Equal(t, Sysbox, class)
		assert.Equal(t, "3f2a9c", native)
	})

	t.Run("a sysbox name may carry its class too", func(t *testing.T) {
		t.Parallel()

		class, native := Split("sysbox:3f2a9c")

		assert.Equal(t, Sysbox, class)
		assert.Equal(t, "3f2a9c", native)
	})
}

func TestCapabilities(t *testing.T) {
	t.Parallel()

	capabilities := Capabilities{
		NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated},
		RestartPolicies: []string{"always", "on-failure"},
	}

	t.Run("a policy the class lists is one it honours", func(t *testing.T) {
		t.Parallel()

		assert.True(t, capabilities.SupportsNetworkPolicy(network.PolicyIsolated))
		assert.False(t, capabilities.SupportsNetworkPolicy(network.PolicyPublic))
	})

	t.Run("a restart policy is matched by its name, whatever count it carries", func(t *testing.T) {
		t.Parallel()

		assert.True(t, capabilities.SupportsRestartPolicy("on-failure:3"))
		assert.True(t, capabilities.SupportsRestartPolicy("always"))
		assert.False(t, capabilities.SupportsRestartPolicy("unless-stopped"))
	})

	t.Run("asking for no restart policy is asking for nothing", func(t *testing.T) {
		t.Parallel()

		assert.True(t, Capabilities{}.SupportsRestartPolicy(""))
		assert.True(t, Capabilities{}.SupportsRestartPolicy("no"))
	})
}

func TestCapacity(t *testing.T) {
	t.Parallel()

	t.Run("two nodes' capacity is the sum of theirs", func(t *testing.T) {
		t.Parallel()

		a := Capacity{CPU: 8, AllocatedCPU: 1.5, Memory: 12 << 30, AllocatedMemory: 512 << 20}
		b := Capacity{CPU: 4, AllocatedCPU: 0.5, Memory: 4 << 30, AllocatedMemory: 256 << 20, Reserved: true}

		assert.Equal(t, Capacity{
			CPU:             12,
			AllocatedCPU:    2,
			Memory:          16 << 30,
			AllocatedMemory: 768 << 20,
			Reserved:        true,
		}, a.Add(b))
	})
}

// TestOfferWire pins the names an offer travels under, since nodes and the
// control plane may run different versions of this code during a deploy.
func TestOfferWire(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(Offer{
		Class:   Firecracker,
		Driver:  "microvm",
		Version: "1",
		Healthy: true,
		Capabilities: Capabilities{
			Isolation:       IsolationMicroVM,
			NetworkPolicies: []network.Policy{network.PolicyNone},
			RestartPolicies: []string{"no"},
			Architectures:   []string{"amd64"},
		},
		Capacity: Capacity{CPU: 8, Memory: 1 << 30, Reserved: true},
	})
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"class": "firecracker",
		"driver": "microvm",
		"version": "1",
		"healthy": true,
		"capabilities": {
			"isolation": "microvm",
			"network_policies": ["none"],
			"stack_networks": false,
			"read_only_root": false,
			"disk_limit": false,
			"tty": false,
			"restart_policies": ["no"],
			"min_memory": 0,
			"max_memory": 0,
			"max_cpu": 0,
			"architectures": ["amd64"]
		},
		"capacity": {"cpu": 8, "allocated_cpu": 0, "memory": 1073741824, "allocated_memory": 0, "reserved": true}
	}`, string(encoded))
}
