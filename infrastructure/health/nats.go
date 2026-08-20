package health

import (
	"context"

	"github.com/nats-io/nats.go"

	"github.com/khanzadimahdi/testproject/domain"
)

// NatsPinger checks that the messaging server answers. flushing performs a real
// round trip to the server rather than reporting the locally cached connection
// state.
type NatsPinger struct {
	connection *nats.Conn
}

var _ domain.Pinger = &NatsPinger{}

func NewNatsPinger(connection *nats.Conn) *NatsPinger {
	return &NatsPinger{
		connection: connection,
	}
}

func (p *NatsPinger) Ping(ctx context.Context) error {
	// while reconnecting, published messages are buffered and a flush would
	// block until the context expires, so these two states are reported directly
	switch {
	case p.connection.IsClosed():
		return nats.ErrConnectionClosed
	case p.connection.IsReconnecting():
		return nats.ErrDisconnected
	}

	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()

	return p.connection.FlushWithContext(ctx)
}
