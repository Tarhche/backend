package ingress

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
)

// asked records what the transport dialled for, which is the whole of what it
// decides.
type asked struct {
	orchestrator string
	target       tunnel.Target
}

func (a *asked) Dial(_ context.Context, orchestrator string, target tunnel.Target) (net.Conn, error) {
	a.orchestrator = orchestrator
	a.target = target

	left, right := net.Pipe()
	right.Close()

	return left, nil
}

func TestNewTransport(t *testing.T) {
	t.Run("the address names a runner, and the port on it means nothing", func(t *testing.T) {
		tests := []struct {
			address string
			want    string
		}{
			{address: "runner-orchestrator-01:80", want: "runner-orchestrator-01"},
			{address: "runner-orchestrator-01", want: "runner-orchestrator-01"},
			{address: "runner-orchestrator-01:8080", want: "runner-orchestrator-01"},
		}

		for _, test := range tests {
			t.Run(test.address, func(t *testing.T) {
				var recorded asked

				transport := NewTransport(&recorded, "api", time.Minute)

				conn, err := transport.DialContext(t.Context(), "tcp", test.address)
				require.NoError(t, err)
				defer conn.Close()

				assert.Equal(t, test.want, recorded.orchestrator)
				assert.Equal(t, tunnel.Target{Service: "api"}, recorded.target,
					"a runner is asked for a service it offers, not for an address")
			})
		}
	})

	t.Run("the service it asks for is the one it was built with", func(t *testing.T) {
		var recorded asked

		transport := NewTransport(&recorded, "ssh", time.Minute)

		conn, err := transport.DialContext(t.Context(), "tcp", "runner-orchestrator-01:22")
		require.NoError(t, err)
		defer conn.Close()

		assert.Equal(t, tunnel.Target{Service: "ssh"}, recorded.target)
	})

	t.Run("connections are let go after the time it was given", func(t *testing.T) {
		transport := NewTransport(&asked{}, "api", 42*time.Second)

		assert.Equal(t, 42*time.Second, transport.IdleConnTimeout)
	})
}
