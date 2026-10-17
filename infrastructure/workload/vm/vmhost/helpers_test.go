package vmhost_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost"
	vmhostAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/vmhost"
)

// at is when everything in these tests happens, so what crosses the socket
// can be compared to what the engine holds to the nanosecond.
var at = time.Date(2026, 10, 5, 12, 0, 0, 123456789, time.UTC)

// settle is how long anything a test waits for is given.
const settle = 10 * time.Second

// host is a vmhost of the test's own: the server over an engine, on a unix
// socket, as serve-workload-vmhost runs it, and the client an orchestrator
// reaches it with.
type host struct {
	client *vmhost.Client
	server *vmhostAPI.Server
	http   *http.Server
	socket string
}

// serve serves engine on a socket of the test's own until the test ends.
func serve(t *testing.T, engine vm.Engine) *host {
	t.Helper()

	socket := socketPath(t)

	listener, err := net.Listen("unix", socket)
	require.NoError(t, err)

	server := vmhostAPI.NewServer(engine, slog.New(slog.DiscardHandler), vmhostAPI.WithPropagator(propagation.TraceContext{}))
	httpServer := &http.Server{Handler: server, ReadHeaderTimeout: settle}

	go func() {
		_ = httpServer.Serve(listener)
	}()

	t.Cleanup(func() {
		_ = server.Close()
		_ = httpServer.Close()
	})

	return &host{
		client: vmhost.NewClient(socket, vmhost.WithPropagator(propagation.TraceContext{})),
		server: server,
		http:   httpServer,
		socket: socket,
	}
}

// socketPath is a socket of the test's own. It is not in the test's own
// directory, which is named after the test: a unix socket's path is about a
// hundred bytes at most.
func socketPath(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "vmhost")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	return filepath.Join(dir, "v.sock")
}

// machine is a machine VM's spec, reachable on its ports.
func machine(id string) vm.Spec {
	return vm.Spec{
		ID:        id,
		Image:     "ubuntu:24.04",
		Resources: vm.Resources{CPUs: 1, Memory: 256 << 20, Disk: 1 << 30},
		Ports:     []port.Port{80, 8080},
		Network:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		Labels:    map[string]string{vm.LabelPurpose: vm.PurposeVM, vm.LabelVM: id, vm.LabelOwner: "owner-uuid"},
	}
}

// newEngine is a memory engine whose clock stands still at at.
func newEngine(options ...memory.Option) *memory.Engine {
	return memory.New(append([]memory.Option{
		memory.WithClock(func() time.Time { return at }),
		memory.WithHost("10.89.1.10"),
	}, options...)...)
}

// sessions is an engine that keeps the exec sessions it opens, so a test can
// see what the vmhost did to them.
type sessions struct {
	vm.Engine

	lock    sync.Mutex
	opened  []vm.ExecSession
	options []vm.ExecOptions
}

func (s *sessions) Exec(ctx context.Context, id string, options vm.ExecOptions) (vm.ExecSession, error) {
	session, err := s.Engine.Exec(ctx, id, options)
	if err == nil {
		s.lock.Lock()
		s.opened = append(s.opened, session)
		s.options = append(s.options, options)
		s.lock.Unlock()
	}

	return session, err
}

// asked is what the last session was opened with.
func (s *sessions) asked(t *testing.T) vm.ExecOptions {
	t.Helper()

	s.lock.Lock()
	defer s.lock.Unlock()

	require.NotEmpty(t, s.options)

	return s.options[len(s.options)-1]
}

// last is the session opened last, as the memory engine has it.
func (s *sessions) last(t *testing.T) *memory.Session {
	t.Helper()

	s.lock.Lock()
	defer s.lock.Unlock()

	require.NotEmpty(t, s.opened)

	session, ok := s.opened[len(s.opened)-1].(*memory.Session)
	require.True(t, ok)

	return session
}

// commands is what runs inside the test's VMs, by the command's first word.
type commands map[string]memory.ExecFunc

func (c commands) exec(ctx context.Context, id string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	run, ok := c[options.Command[0]]
	if !ok {
		_, _ = io.WriteString(stderr, "sh: "+options.Command[0]+": not found\n")

		return 127
	}

	return run(ctx, id, options, stdin, stdout, stderr)
}

// eventually waits for condition, failing the test after settle.
func eventually(t *testing.T, what string, condition func() bool) {
	t.Helper()

	require.Eventually(t, condition, settle, 5*time.Millisecond, what)
}
