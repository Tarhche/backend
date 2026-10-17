//go:build e2e

package e2e

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// terminal is a VM's terminal, opened through the ingress the way the
// dashboard's is: a websocket carrying the shell's bytes both ways.
type terminal struct {
	conn *websocket.Conn

	mu     sync.Mutex
	output bytes.Buffer
	more   chan struct{}
	closed chan struct{}
	err    error
}

// markers numbers what a command is followed by, so its end can be told from
// the shell echoing the line it was typed on.
var markers atomic.Int64

// openTerminal opens a terminal in one of the run's VMs, as its owner.
func openTerminal(t testing.TB, vmUUID string) *terminal {
	t.Helper()

	conn, status, err := dialTerminal(t, user, vmUUID)
	if err != nil {
		t.Fatalf("opening a terminal in vm %s: %v (%d)", vmUUID, err, status)
	}

	term := &terminal{conn: conn, more: make(chan struct{}, 1), closed: make(chan struct{})}
	go term.read()

	t.Cleanup(func() { _ = conn.Close() })

	return term
}

// dialTerminal opens the websocket of a VM's terminal through the ingress, as
// whoever s is signed in as, and says what the ingress answered when it would
// not.
func dialTerminal(t testing.TB, s *session, vmUUID string) (*websocket.Conn, int, error) {
	t.Helper()

	token, err := s.token(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	address := "ws" + strings.TrimPrefix(env.ingressURL, "http") + "/vms/" + vmUUID + "/attach"

	conn, response, err := websocket.DefaultDialer.DialContext(t.Context(), address, http.Header{
		"Authorization": {"Bearer " + token},
	})

	status := 0
	if response != nil {
		status = response.StatusCode
	}

	return conn, status, err
}

func (term *terminal) read() {
	defer close(term.closed)

	for {
		kind, data, err := term.conn.ReadMessage()
		if err != nil {
			term.mu.Lock()
			term.err = err
			term.mu.Unlock()

			return
		}

		if kind != websocket.BinaryMessage {
			continue
		}

		term.mu.Lock()
		term.output.Write(data)
		term.mu.Unlock()

		select {
		case term.more <- struct{}{}:
		default:
		}
	}
}

// run types line into the shell and waits until it has run, answering with
// what the shell printed meanwhile.
func (term *terminal) run(t testing.TB, line string, timeout time.Duration) string {
	t.Helper()

	// the shell prints the arithmetic's answer only once the command is
	// done; the line as typed, which it echoes, holds the sum instead. It is
	// a line of its own, so a command sent to the background ends as well as
	// any other.
	n := markers.Add(1)
	marker := fmt.Sprintf("e2e-done-%d", 1000+n)
	typed := fmt.Sprintf("%s\necho e2e-done-$((1000+%d))\n", line, n)

	term.mu.Lock()
	start := term.output.Len()
	term.mu.Unlock()

	if err := term.conn.WriteMessage(websocket.BinaryMessage, []byte(typed)); err != nil {
		t.Fatalf("typing %q: %v", line, err)
	}

	deadline := time.After(timeout)

	for {
		term.mu.Lock()
		printed := term.output.String()[start:]
		term.mu.Unlock()

		if strings.Contains(printed, marker) {
			return printed
		}

		select {
		case <-term.more:
		case <-term.closed:
			t.Fatalf("the terminal closed while %q ran: %v; it printed %q", line, term.err, printed)
		case <-deadline:
			t.Fatalf("%q did not finish in %s; the terminal printed %q", line, timeout, printed)
		}
	}
}

// resize tells the shell how big its terminal is, as the dashboard does when
// the page changes size.
func (term *terminal) resize(t testing.TB, rows, cols uint) {
	t.Helper()

	message := fmt.Sprintf(`{"type":"resize","rows":%d,"cols":%d}`, rows, cols)
	if err := term.conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
		t.Fatalf("resizing the terminal: %v", err)
	}
}
