//go:build linux

package agent

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/khanzadimahdi/testproject/infrastructure/runner/firecracker/guest"
)

// dialTimeout is how long reaching one of the task's ports may take. The task
// is on the same machine, so it answers at once or not at all.
const dialTimeout = 5 * time.Second

// dial connects to one of the task's ports and turns the connection it was
// asked on into a pipe to it. The port is reached on the task's own address
// rather than on loopback, as its neighbours would reach it: something the
// task serves only to itself stays its own.
func (a *Agent) dial(rw http.ResponseWriter, r *http.Request) {
	port, err := strconv.ParseUint(r.URL.Query().Get("port"), 10, 16)
	if err != nil || port == 0 {
		http.Error(rw, "that is not a port", http.StatusBadRequest)

		return
	}

	a.lock.Lock()
	running := a.status.State == guest.StateRunning

	var interfaces []guest.Interface
	if a.config != nil {
		interfaces = a.config.Interfaces
	}
	a.lock.Unlock()

	if !running {
		http.Error(rw, "the task is not running", http.StatusConflict)

		return
	}

	address, found := primaryAddress(interfaces)
	if !found {
		http.Error(rw, "the task is on no network, so nothing reaches its ports", http.StatusNotFound)

		return
	}

	upstream, err := net.DialTimeout("tcp", net.JoinHostPort(address, strconv.FormatUint(port, 10)), dialTimeout)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadGateway)

		return
	}

	conn, buffered, err := hijack(rw, guest.UpgradeDial, nil)
	if err != nil {
		upstream.Close()

		return
	}

	go pipe(conn, buffered, upstream)
}

// pipe carries bytes both ways until both ways are done. Each way ending is
// passed on as a half-close, so a peer that finished sending can still read
// the answer.
func pipe(conn net.Conn, buffered io.Reader, upstream net.Conn) {
	defer conn.Close()
	defer upstream.Close()

	var both sync.WaitGroup
	both.Add(2)

	go func() {
		defer both.Done()

		_, _ = io.Copy(upstream, buffered)
		closeWrite(upstream)
	}()

	go func() {
		defer both.Done()

		_, _ = io.Copy(conn, upstream)
		closeWrite(conn)
	}()

	both.Wait()
}

func closeWrite(conn net.Conn) {
	if closer, ok := conn.(interface{ CloseWrite() error }); ok {
		_ = closer.CloseWrite()

		return
	}

	_ = conn.Close()
}
