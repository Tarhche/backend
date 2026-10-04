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
		name   string
		policy Policy
		want   []Attachment
	}{
		{
			name:   "an isolated task joins the shared internal network",
			policy: PolicyIsolated,
			want:   []Attachment{{Name: IsolatedNetworkName}},
		},
		{
			name:   "a public task also joins the bridge, which is what routes out",
			policy: PolicyPublic,
			want:   []Attachment{{Name: IsolatedNetworkName}, {Name: PublicNetworkName, Gateway: true}},
		},
		{
			name:   "a task with no network joins nothing",
			policy: PolicyNone,
			want:   []Attachment{{Name: NoNetworkName}},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, Attachments(tt.policy))
		})
	}
}

func TestAttachmentsGateway(t *testing.T) {
	t.Parallel()

	// only one network can provide the default route, and it has to be the one
	// that routes out — the isolated network deliberately does not.
	var gateways []string
	for _, attachment := range Attachments(PolicyPublic) {
		if attachment.Gateway {
			gateways = append(gateways, attachment.Name)
		}
	}

	assert.Equal(t, []string{PublicNetworkName}, gateways)

	// a task that cannot reach the internet needs no default route at all.
	for _, attachment := range Attachments(PolicyIsolated) {
		assert.False(t, attachment.Gateway)
	}
}
