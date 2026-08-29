// Package websockettest provides sockets for exercising the websocket packages.
package websockettest

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

type hijackableResponseWriter struct {
	*httptest.ResponseRecorder
	conn net.Conn
	bw   *bufio.ReadWriter
}

func (h *hijackableResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.conn, h.bw, nil
}

// StalledClientConn upgrades a websocket over an in-memory pipe and returns the
// server side of it. net.Pipe is unbuffered, so once the client has drained the
// upgrade response and stopped reading, every server write blocks.
func StalledClientConn(t *testing.T) *websocket.Conn {
	t.Helper()

	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})

	upgradeDrained := make(chan struct{})
	go func() {
		defer close(upgradeDrained)

		buf := make([]byte, 4096)
		_, _ = clientConn.Read(buf)
	}()

	recorder := &hijackableResponseWriter{
		ResponseRecorder: httptest.NewRecorder(),
		conn:             serverConn,
		bw: bufio.NewReadWriter(
			bufio.NewReader(serverConn),
			bufio.NewWriter(serverConn),
		),
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Sec-WebSocket-Version", "13")
	request.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

	conn, err := upgrader.Upgrade(recorder, request, nil)
	if err != nil {
		t.Fatal(err)
	}

	<-upgradeDrained

	return conn
}
