package api

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// guestSide is the host's end of a connection to a machine's agent. A vsock
// passes no half-close on, and ends the whole connection instead, so one
// passed on here is a stream cut short inside the guest.
type guestSide struct {
	net.Conn

	halfClosed atomic.Bool
}

func (c *guestSide) CloseWrite() error {
	c.halfClosed.Store(true)

	return c.Conn.Close()
}

// clientPair is the two ends of a client's connection to vmhost, on a real
// unix socket, so that the client can say it is done sending.
func clientPair(t *testing.T) (*net.UnixConn, net.Conn) {
	t.Helper()

	dir, err := os.MkdirTemp("", "vmh")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	listener, err := net.Listen("unix", filepath.Join(dir, "s"))
	require.NoError(t, err)
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	client, err := net.Dial("unix", listener.Addr().String())
	require.NoError(t, err)

	select {
	case vmhost := <-accepted:
		return client.(*net.UnixConn), vmhost
	case <-time.After(5 * time.Second):
		t.Fatal("the client was never accepted")

		return nil, nil
	}
}

func TestSplice(t *testing.T) {
	t.Parallel()

	t.Run("a client done sending is not passed on to the guest, which answers all the same", func(t *testing.T) {
		t.Parallel()

		client, vmhost := clientPair(t)
		defer client.Close()

		host, guest := net.Pipe()
		agent := &guestSide{Conn: host}

		spliced := make(chan struct{})
		go func() {
			defer close(spliced)
			splice(vmhost, vmhost, agent)
		}()

		_, err := client.Write([]byte("GET / HTTP/1.0\r\n\r\n"))
		require.NoError(t, err)
		require.NoError(t, client.CloseWrite(), "the client is done sending, and waits for the answer")

		request := make([]byte, len("GET / HTTP/1.0\r\n\r\n"))
		_, err = io.ReadFull(guest, request)
		require.NoError(t, err)
		assert.Equal(t, "GET / HTTP/1.0\r\n\r\n", string(request))

		// the task answers after the client was done sending, and ends the
		// stream once it has.
		go func() {
			_, _ = guest.Write([]byte("HTTP/1.0 200 OK\r\n\r\nhello"))
			_ = guest.Close()
		}()

		answer, err := io.ReadAll(client)
		require.NoError(t, err)
		assert.Equal(t, "HTTP/1.0 200 OK\r\n\r\nhello", string(answer))
		assert.False(t, agent.halfClosed.Load(), "no half-close reached the guest")

		select {
		case <-spliced:
		case <-time.After(5 * time.Second):
			t.Fatal("the stream did not end with the guest's")
		}
	})

	t.Run("a client that goes away takes the guest's side with it", func(t *testing.T) {
		t.Parallel()

		client, vmhost := clientPair(t)

		host, guest := net.Pipe()
		agent := &guestSide{Conn: host}

		spliced := make(chan struct{})
		go func() {
			defer close(spliced)
			splice(vmhost, vmhost, agent)
		}()

		// gone, which on a socket reads as finished sending: the guest's
		// side stays open for an answer, and the guest finds out it has
		// nobody to give one to the next time it sends anything.
		require.NoError(t, client.Close())

		_, _ = guest.Write([]byte("are you there?"))

		select {
		case <-spliced:
		case <-time.After(5 * time.Second):
			t.Fatal("the stream outlived its client")
		}

		_, err := guest.Write([]byte("hello"))
		assert.Error(t, err, "the guest's side was let go of")
		assert.False(t, agent.halfClosed.Load())
	})

	t.Run("what the client sends reaches the guest in order, and the guest's answer the client", func(t *testing.T) {
		t.Parallel()

		client, vmhost := clientPair(t)
		defer client.Close()

		host, guest := net.Pipe()

		go splice(vmhost, vmhost, host)

		go func() {
			_, _ = io.Copy(guest, guest)
		}()

		for _, message := range []string{"one", "two", "three"} {
			_, err := client.Write([]byte(message))
			require.NoError(t, err)

			echoed := make([]byte, len(message))
			_, err = io.ReadFull(client, echoed)
			require.NoError(t, err)
			assert.Equal(t, message, string(echoed))
		}
	})
}
