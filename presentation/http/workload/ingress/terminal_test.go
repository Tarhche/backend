package ingress

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// terminalNode is a node standing in for the far end of a tunnel: it answers
// the routes the ingress carries terminals to, by echoing, and records which
// it was asked for.
type terminalNode struct {
	server *httptest.Server
	path   string
}

func newTerminalNode(t *testing.T) *terminalNode {
	t.Helper()

	n := &terminalNode{}
	upgrader := websocket.Upgrader{}

	echo := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		n.path = r.URL.Path

		conn, err := upgrader.Upgrade(rw, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}

		_ = conn.WriteMessage(websocket.TextMessage, append([]byte("echo: "), message...))
	})

	mux := http.NewServeMux()
	mux.Handle("GET /api/tasks/{uuid}/attach", echo)
	mux.Handle("GET /api/vms/{uuid}/attach", echo)
	mux.Handle("GET /api/lamps/{uuid}/attach", echo)

	n.server = httptest.NewServer(mux)
	t.Cleanup(n.server.Close)

	return n
}

func tunnelTo(nodes map[string]*terminalNode) (connectedWorkloads, http.RoundTripper) {
	connected := make(connectedWorkloads, len(nodes))
	for name := range nodes {
		connected[name] = name
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			name, _, _ := net.SplitHostPort(address)

			n, ok := nodes[name]
			if !ok {
				return nil, domain.ErrNotExists
			}

			return (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(n.server.URL, "http://"))
		},
	}

	return connected, transport
}

// talk opens a terminal through front and has one exchange on it.
func talk(t *testing.T, front *httptest.Server, path string) string {
	t.Helper()

	conn, _, err := websocket.DefaultDialer.Dial("ws://"+strings.TrimPrefix(front.URL, "http://")+path, nil)
	require.NoError(t, err)
	defer conn.Close()

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("ls")))

	_, message, err := conn.ReadMessage()
	require.NoError(t, err)

	return string(message)
}

func TestTerminalHandler(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	t.Run("a terminal in a vm is carried to its node's own route for it, at the url it always had", func(t *testing.T) {
		n := newTerminalNode(t)
		connected, transport := tunnelTo(map[string]*terminalNode{"workload-orchestrator-02": n})

		mux := http.NewServeMux()
		require.NoError(t, RouteKinds(mux, vmsIn(t, exposing("box", "workload-orchestrator-02")), connected, transport, logger))

		front := httptest.NewServer(mux)
		defer front.Close()

		assert.Equal(t, "echo: ls", talk(t, front, "/vms/vm-box/attach?token=abc"))
		assert.Equal(t, "/api/vms/vm-box/attach", n.path)
	})

	t.Run("a terminal in a task is carried as it always was", func(t *testing.T) {
		n := newTerminalNode(t)
		connected, transport := tunnelTo(map[string]*terminalNode{"workload-orchestrator-01": n})

		resolver := &fakeResolver{tasks: map[string]task.Task{"web": {UUID: "task-uuid", NodeName: "workload-orchestrator-01"}}}

		mux := http.NewServeMux()
		mux.Handle("GET /tasks/{uuid}/attach", NewTerminalHandler(resolver, connected, transport, logger))

		front := httptest.NewServer(mux)
		defer front.Close()

		assert.Equal(t, "echo: ls", talk(t, front, "/tasks/task-uuid/attach"))
		assert.Equal(t, "/api/tasks/task-uuid/attach", n.path)
	})

	for name, tt := range map[string]struct {
		vms    []vmKind.VM
		status int
		says   string
	}{
		"a vm that is not there": {
			status: http.StatusNotFound,
			says:   "no such vm",
		},
		"a vm on no node": {
			vms:    []vmKind.VM{exposing("box", "")},
			status: http.StatusServiceUnavailable,
			says:   "not been scheduled",
		},
		"a vm whose node is not connected": {
			vms:    []vmKind.VM{exposing("box", "workload-orchestrator-09")},
			status: http.StatusServiceUnavailable,
			says:   "not connected",
		},
	} {
		t.Run("no terminal in "+name, func(t *testing.T) {
			connected, transport := tunnelTo(map[string]*terminalNode{"workload-orchestrator-01": newTerminalNode(t)})

			mux := http.NewServeMux()
			require.NoError(t, RouteKinds(mux, vmsIn(t, tt.vms...), connected, transport, logger))

			rw := httptest.NewRecorder()
			mux.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "/vms/vm-box/attach", nil))

			assert.Equal(t, tt.status, rw.Code)
			assert.Contains(t, rw.Body.String(), tt.says)
		})
	}
}
