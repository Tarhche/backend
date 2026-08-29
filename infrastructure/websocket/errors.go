package websocket

import "github.com/khanzadimahdi/testproject/infrastructure/websocket/transport"

var (
	// ErrClosed is returned when replying on a websocket that is already closed.
	ErrClosed = transport.ErrClosed

	// ErrRequestIDRequired is returned for a reply that names no request.
	ErrRequestIDRequired = transport.ErrRequestIDRequired
)
