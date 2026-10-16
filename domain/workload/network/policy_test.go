package network

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestPolicy(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		policy   Policy
		valid    bool
		ports    bool
		internet bool
	}{
		{policy: PolicyNone, valid: true, ports: false, internet: false},
		{policy: PolicyIsolated, valid: true, ports: true, internet: false},
		{policy: PolicyPublic, valid: true, ports: true, internet: true},
		{policy: PolicyOutbound, valid: true, ports: false, internet: true},
		{policy: Policy(""), valid: false},
		{policy: Policy("host"), valid: false},
		{policy: Policy("ISOLATED"), valid: false},
	}

	for _, tt := range testcases {
		t.Run(string(tt.policy), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.valid, tt.policy.IsValid())
			assert.Equal(t, tt.ports, tt.policy.AllowsPorts())
			assert.Equal(t, tt.internet, tt.policy.ReachesInternet())
		})
	}
}

func TestPolicy_VMNetwork(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name   string
		policy Policy
		want   vm.Network
	}{
		{
			name:   "no network is nothing either way",
			policy: PolicyNone,
			want:   vm.Network{Ingress: vm.AccessDeny, Egress: vm.AccessDeny},
		},
		{
			name:   "an isolated task serves its ports and calls nothing",
			policy: PolicyIsolated,
			want:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		},
		{
			name:   "a public task calls out as well",
			policy: PolicyPublic,
			want:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessAllow},
		},
		{
			name:   "an outbound task calls out, and nothing reaches it",
			policy: PolicyOutbound,
			want:   vm.Network{Ingress: vm.AccessDeny, Egress: vm.AccessAllow},
		},
		{
			name:   "a policy that names nothing is the default one",
			policy: Policy(""),
			want:   DefaultPolicy.VMNetwork(),
		},
		{
			name:   "a policy nobody knows is given nothing",
			policy: Policy("host"),
			want:   vm.Network{Ingress: vm.AccessDeny, Egress: vm.AccessDeny},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.policy.VMNetwork())
		})
	}
}

func TestPolicyOf(t *testing.T) {
	t.Parallel()

	t.Run("each way a vm's network is open is one policy's network", func(t *testing.T) {
		t.Parallel()

		for _, policy := range []Policy{PolicyNone, PolicyIsolated, PolicyPublic, PolicyOutbound} {
			assert.Equal(t, policy, PolicyOf(policy.VMNetwork()), policy)
		}
	})

	t.Run("a way that says nothing is denied", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, PolicyOutbound, PolicyOf(vm.Network{Egress: vm.AccessAllow}))
		assert.Equal(t, PolicyNone, PolicyOf(vm.Network{}))
	})
}
