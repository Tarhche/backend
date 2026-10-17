package controlplane

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/danceable/console"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/reconcile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	executionsMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/executions"
	leasesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/leases"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
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

		serverStartedListening := make(chan struct{})

		go func() {
			serverStartedListening <- struct{}{}
			command.Run(ctx)
		}()

		<-serverStartedListening
		time.Sleep(50 * time.Millisecond) // wait for server to start serving

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://0.0.0.0:%d", command.configs.Port), nil)
		assert.NoError(t, err)

		c := http.Client{
			Timeout: 1 * time.Second,
		}

		resp, err := c.Do(req)
		if !assert.NoError(t, err) {
			return
		}
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

	})
}

func TestServe_graphs(t *testing.T) {
	t.Parallel()

	t.Run("the graphs' workers and their resync run until the context is done, and then the workers stop", func(t *testing.T) {
		t.Parallel()

		command := NewServeCommand()
		command.configs.GraphResync = time.Millisecond
		command.logger = slog.New(slog.DiscardHandler)
		command.reconcileGraphs = graphs(t)

		ctx, stop := context.WithCancel(context.Background())

		stopped := command.graphs(ctx)

		select {
		case <-stopped:
			require.Fail(t, "the workers stopped before their context was done")
		case <-time.After(20 * time.Millisecond):
		}

		stop()

		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			require.Fail(t, "the workers never stopped")
		}
	})

	t.Run("with no graphs to keep, there is nothing to wait for", func(t *testing.T) {
		t.Parallel()

		select {
		case <-NewServeCommand().graphs(context.Background()):
		default:
			require.Fail(t, "it waits for workers there are none of")
		}
	})
}

// graphs is a reconciler of the graphs of no kind of root, over nothing.
func graphs(t *testing.T) *reconcile.Reconciler {
	t.Helper()

	executions := executionsMemory.NewRepository()

	executor, err := execute.New(nil, nil, executions, nil)
	require.NoError(t, err)

	r, err := reconcile.New(reconcile.Dependencies{
		Registry:   kind.NewRegistry[kind.ControlPlaneBinding](),
		Resources:  resourcesMemory.NewRepository(),
		Planner:    graph.NewPlanner(nil, graph.DefaultPolicy()),
		Executor:   executor,
		Executions: executions,
		Leases:     leasesMemory.NewRepository(nil),
		Holder:     "one",
		Logger:     slog.New(slog.DiscardHandler),
	}, reconcile.DefaultConfig())
	require.NoError(t, err)

	return r
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
