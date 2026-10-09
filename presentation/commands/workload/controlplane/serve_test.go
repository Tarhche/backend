package controlplane

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danceable/console"
	"github.com/khanzadimahdi/testproject/domain"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestServe(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		command := NewServeCommand()

		want := "serve-workload-controlplane"
		got := command.Name()

		if want != got {
			t.Errorf("want command name %q got %q", want, got)
		}
	})

	t.Run("description", func(t *testing.T) {
		command := NewServeCommand()

		want := "serves a http server."
		got := command.Description()

		if want != got {
			t.Errorf("want command description %q got %q", want, got)
		}
	})

	t.Run("usage", func(t *testing.T) {
		command := NewServeCommand()

		want := "serve-workload-controlplane [arguments]"
		got := command.Usage()

		if want != got {
			t.Errorf("want command usage %q got %q", want, got)
		}
	})

	t.Run("configure", func(t *testing.T) {
		command := NewServeCommand()

		flagSet := console.NewFlagSet(command.Name(), io.Discard)

		command.Configure(flagSet)

		port := flagSet.Lookup("port")
		if port == nil {
			t.Fatal("port flag has not been configured")
		}

		if port.Usage() != "specifies which port server should listen to." {
			t.Error("unexpected port flag usage")
		}

		if port.Short() != "p" {
			t.Error("unexpected port flag short name")
		}

		if port.Env() != "SERVER_PORT" {
			t.Error("unexpected port flag environment variable")
		}

		if command.configs.Port != 80 {
			t.Error("unexpected port flag default value")
		}

		if err := flagSet.Parse([]string{"--port", "100"}); err != nil {
			t.Errorf("unexpected parsing error: %q", err)
		}

		if command.configs.Port != 100 {
			t.Error("unexpected port flag default value")
		}
	})

	t.Run("configure with the short flag", func(t *testing.T) {
		command := NewServeCommand()

		flagSet := console.NewFlagSet(command.Name(), io.Discard)

		command.Configure(flagSet)

		if err := flagSet.Parse([]string{"-p", "100"}); err != nil {
			t.Errorf("unexpected parsing error: %q", err)
		}

		if command.configs.Port != 100 {
			t.Error("unexpected port flag value")
		}
	})

	t.Run("configure from the environment", func(t *testing.T) {
		t.Setenv("SERVER_PORT", "100")

		command := NewServeCommand()

		flagSet := console.NewFlagSet(command.Name(), io.Discard)

		command.Configure(flagSet)

		if err := flagSet.Parse(nil); err != nil {
			t.Errorf("unexpected parsing error: %q", err)
		}

		if command.configs.Port != 100 {
			t.Error("unexpected port flag value")
		}
	})

	t.Run("run", func(t *testing.T) {
		ctx := t.Context()

		handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
			fmt.Fprint(rw, "test response")
		})

		subscribers := map[string]domain.MessageHandler{
			"test1": domain.MessageHandlerFunc(func(ctx context.Context, message []byte) error { return nil }),
			"test2": domain.MessageHandlerFunc(func(ctx context.Context, message []byte) error { return nil }),
			"test3": domain.MessageHandlerFunc(func(ctx context.Context, message []byte) error { return nil }),
		}

		var consumer messaging.MockProduceConsumer
		consumer.On("Consume", ctx, mock.Anything, mock.Anything).Times(len(subscribers)).Return(nil)
		defer consumer.AssertExpectations(t)

		command := NewServeCommand()
		command.configs.Port = findAvailablePort()
		command.handler = handler
		command.consumer = &consumer
		command.consumers = subscribers
		command.migrations = &pendingMigrations{}
		command.prepare = func(context.Context) error { return nil }

		serverStartedListening := make(chan struct{})

		go func() {
			serverStartedListening <- struct{}{}
			command.Run(ctx)
		}()

		<-serverStartedListening

		// nothing is to be migrated, so it starts at once.
		assert.Eventually(t, func() bool {
			return get(command.configs.Port, "/") == http.StatusOK
		}, 2*time.Second, 10*time.Millisecond)
	})

	t.Run("run touches nothing of the workload until what is stored is migrated", func(t *testing.T) {
		ctx := t.Context()

		mux := http.NewServeMux()
		mux.HandleFunc("GET /health", func(rw http.ResponseWriter, r *http.Request) { fmt.Fprint(rw, "ok") })
		mux.HandleFunc("GET /api/vms", func(rw http.ResponseWriter, r *http.Request) { fmt.Fprint(rw, "[]") })

		subscribers := map[string]domain.MessageHandler{
			"workloadVmHeartbeat":     domain.MessageHandlerFunc(func(ctx context.Context, message []byte) error { return nil }),
			"workloadResourceActedOn": domain.MessageHandlerFunc(func(ctx context.Context, message []byte) error { return nil }),
		}

		var consumer messaging.MockProduceConsumer
		consumer.On("Consume", ctx, mock.Anything, mock.Anything).Times(len(subscribers)).Return(nil)
		defer consumer.AssertExpectations(t)

		migrations := &pendingMigrations{pending: []string{"2026-10-09-move-resources-to-workloads"}}

		var prepared atomic.Int32

		command := NewServeCommand()
		command.configs.Port = findAvailablePort()
		command.handler = mux
		command.consumer = &consumer
		command.consumers = subscribers
		command.migrations = migrations
		command.prepare = func(context.Context) error {
			prepared.Add(1)

			return nil
		}
		command.migrationsEvery = 10 * time.Millisecond

		var logged said
		command.logger = slog.New(slog.NewTextHandler(&logged, nil))

		go command.Run(ctx)

		require.Eventually(t, func() bool {
			return get(command.configs.Port, "/health") == http.StatusOK
		}, 2*time.Second, 10*time.Millisecond, "its health is served while it waits")

		require.Eventually(t, func() bool {
			return migrations.looks() >= 3
		}, 2*time.Second, 10*time.Millisecond, "it looks again, and again")

		assert.Equal(t, http.StatusServiceUnavailable, get(command.configs.Port, "/api/vms"), "nothing else is served")
		assert.Zero(t, prepared.Load(), "its stores are not readied")
		consumer.AssertNotCalled(t, "Consume", mock.Anything, mock.Anything, mock.Anything)

		assert.Contains(t, logged.String(), "waiting for `app migrate`", "it says what it waits for")
		assert.Contains(t, logged.String(), "2026-10-09-move-resources-to-workloads")

		migrations.migrate()

		assert.Eventually(t, func() bool {
			return get(command.configs.Port, "/api/vms") == http.StatusOK
		}, 2*time.Second, 10*time.Millisecond, "once it is migrated, it starts")

		assert.Equal(t, int32(1), prepared.Load(), "its stores readied first")
		consumer.AssertNumberOfCalls(t, "Consume", len(subscribers))
		assert.Contains(t, logged.String(), "what is stored is migrated: the control plane starts")
	})

	t.Run("run goes on waiting while what is migrated cannot be read", func(t *testing.T) {
		ctx := t.Context()

		var consumer messaging.MockProduceConsumer
		defer consumer.AssertExpectations(t)

		migrations := &pendingMigrations{failure: errors.New("the database is away")}

		command := NewServeCommand()
		command.configs.Port = findAvailablePort()
		command.handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {})
		command.consumer = &consumer
		command.consumers = map[string]domain.MessageHandler{
			"workloadResourceActedOn": domain.MessageHandlerFunc(func(ctx context.Context, message []byte) error { return nil }),
		}
		command.migrations = migrations
		command.prepare = func(context.Context) error { return errors.New("readied before it was migrated") }
		command.migrationsEvery = 10 * time.Millisecond
		command.logger = slog.New(slog.DiscardHandler)

		go command.Run(ctx)

		require.Eventually(t, func() bool {
			return migrations.looks() >= 3
		}, 2*time.Second, 10*time.Millisecond)

		assert.Equal(t, http.StatusServiceUnavailable, get(command.configs.Port, "/api/vms"))
		assert.Equal(t, http.StatusOK, get(command.configs.Port, "/health"))
	})

	t.Run("run fails when the control plane cannot start once what is stored is migrated", func(t *testing.T) {
		var consumer messaging.MockProduceConsumer
		defer consumer.AssertExpectations(t)

		command := NewServeCommand()
		command.configs.Port = findAvailablePort()
		command.handler = http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {})
		command.consumer = &consumer
		command.consumers = map[string]domain.MessageHandler{
			"workloadResourceActedOn": domain.MessageHandlerFunc(func(ctx context.Context, message []byte) error { return nil }),
		}
		command.migrations = &pendingMigrations{}
		command.prepare = func(context.Context) error { return errors.New("workloads cannot be indexed") }
		command.logger = slog.New(slog.DiscardHandler)

		exited := make(chan console.ExitStatus, 1)

		go func() {
			exited <- command.Run(t.Context())
		}()

		select {
		case status := <-exited:
			assert.Equal(t, console.ExitFailure, status)
		case <-time.After(2 * time.Second):
			t.Fatal("it went on serving")
		}
	})
}

// pendingMigrations are the migrations of a database that is migrated once a
// test says it is, and how many times somebody looked.
type pendingMigrations struct {
	lock    sync.Mutex
	pending []string
	failure error
	looked  int
}

var _ domain.Migrations = &pendingMigrations{}

func (m *pendingMigrations) Pending(context.Context) ([]string, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.looked++

	return slices.Clone(m.pending), m.failure
}

// migrate has every migration applied.
func (m *pendingMigrations) migrate() {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.pending, m.failure = nil, nil
}

// looks is how many times somebody looked at what is pending.
func (m *pendingMigrations) looks() int {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.looked
}

// said is what a logger wrote, read while it may still be writing.
type said struct {
	lock sync.Mutex
	text strings.Builder
}

func (s *said) Write(p []byte) (int, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.text.Write(p)
}

func (s *said) String() string {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.text.String()
}

// get is the status a GET of path on the local port is answered with, or
// none when nothing answers.
func get(port int, path string) int {
	client := http.Client{Timeout: time.Second}

	resp, err := client.Get(fmt.Sprintf("http://0.0.0.0:%d%s", port, path))
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	return resp.StatusCode
}

// findAvailablePort finds an available port to use for testing
func findAvailablePort() int {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 8080 // fallback to default port
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port
}
