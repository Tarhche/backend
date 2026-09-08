package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestAttachments(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name        string
		policy      Policy
		stackSlug   string
		serviceName string
		want        []Attachment
	}{
		{
			name:   "a standalone isolated container joins the shared internal network",
			policy: PolicyIsolated,
			want:   []Attachment{{Name: IsolatedNetworkName}},
		},
		{
			name:   "a standalone public container also joins the bridge, which is what routes out",
			policy: PolicyPublic,
			want:   []Attachment{{Name: IsolatedNetworkName}, {Name: PublicNetworkName, Gateway: true}},
		},
		{
			name:   "a container with no network joins nothing",
			policy: PolicyNone,
			want:   []Attachment{{Name: NoNetworkName}},
		},
		{
			name:        "a service joins its own stack's network under its service name",
			policy:      PolicyIsolated,
			stackSlug:   "myapp-xkfqz",
			serviceName: "api",
			want:        []Attachment{{Name: "runner-stack-myapp-xkfqz", Aliases: []string{"api"}}},
		},
		{
			name:        "a public service reaches its stack and the internet",
			policy:      PolicyPublic,
			stackSlug:   "myapp-xkfqz",
			serviceName: "web",
			want: []Attachment{
				{Name: "runner-stack-myapp-xkfqz", Aliases: []string{"web"}},
				{Name: PublicNetworkName, Gateway: true},
			},
		},
		{
			name:        "a service with no network is cut off from its own stack too",
			policy:      PolicyNone,
			stackSlug:   "myapp-xkfqz",
			serviceName: "worker",
			want:        []Attachment{{Name: NoNetworkName}},
		},
		{
			name:      "a stack service with no name of its own still joins the stack",
			policy:    PolicyIsolated,
			stackSlug: "myapp-xkfqz",
			want:      []Attachment{{Name: "runner-stack-myapp-xkfqz"}},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, Attachments(tt.policy, tt.stackSlug, tt.serviceName))
		})
	}
}

func TestAttachmentsGateway(t *testing.T) {
	t.Parallel()

	// only one network can provide the default route, and it has to be the one
	// that routes out — the private networks deliberately do not.
	for _, stackSlug := range []string{"", "myapp-xkfqz"} {
		attachments := Attachments(PolicyPublic, stackSlug, "web")

		var gateways []string
		for _, attachment := range attachments {
			if attachment.Gateway {
				gateways = append(gateways, attachment.Name)
			}
		}

		assert.Equal(t, []string{PublicNetworkName}, gateways)
	}

	// a container that cannot reach the internet needs no default route at all.
	for _, attachment := range Attachments(PolicyIsolated, "myapp-xkfqz", "web") {
		assert.False(t, attachment.Gateway)
	}
}

func TestStackNetworkName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "runner-stack-myapp-xkfqz", StackNetworkName("myapp-xkfqz"))

	// two stacks never share a network, so a service called "db" in one is not
	// reachable from the other.
	assert.NotEqual(t, StackNetworkName("a-xkfqz"), StackNetworkName("b-xkfqz"))
}
