package ingress

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/tunnel"
)

// Streams is the part of the tunnel a transport needs: a stream to one named
// worker's service. Asking for no more than this is what lets the transport be
// driven by a double in tests.
type Streams interface {
	Dial(ctx context.Context, worker string, target tunnel.Target) (net.Conn, error)
}

// NewTransport builds an http.Transport that dials nothing.
//
// The address it is handed names a runner rather than a machine, and what comes
// back is a stream on one of the connections that runner already opened,
// carried to the service named here. Nothing below this knows the tunnel is not
// a network, which is what lets an ordinary reverse proxy sit on top of it.
func NewTransport(streams Streams, service string, idleTimeout time.Duration) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, _ string, address string) (net.Conn, error) {
			// a URL host carries a port whether or not one means anything here,
			// and what is left of it is the runner's name.
			worker, _, err := net.SplitHostPort(address)
			if err != nil {
				worker = address
			}

			return streams.Dial(ctx, worker, tunnel.Target{Service: service})
		},
		IdleConnTimeout: idleTimeout,
	}
}
