package worker

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
	workerHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/beatHeart"
	taskHeartbeat "github.com/khanzadimahdi/testproject/application/runner/worker/task/beatHeart"
	shipLogs "github.com/khanzadimahdi/testproject/application/runner/worker/task/shipLogs"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/container"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestServe(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		command := NewServeCommand()

		want := "serve-runner-worker"
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

		want := "serve-runner-worker [arguments]"
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

	t.Run("configure the worker name", func(t *testing.T) {
		testCases := []struct {
			name      string
			arguments []string
			env       string
			want      string
		}{
			{
				name:      "long flag",
				arguments: []string{"--name", "runner-worker-01"},
				want:      "runner-worker-01",
			},
			{
				name:      "short flag",
				arguments: []string{"-n", "runner-worker-01"},
				want:      "runner-worker-01",
			},
			{
				name: "environment variable",
				env:  "runner-worker-01",
				want: "runner-worker-01",
			},
			{
				name:      "the flag wins over the environment variable",
				arguments: []string{"--name", "runner-worker-02"},
				env:       "runner-worker-01",
				want:      "runner-worker-02",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				if testCase.env != "" {
					t.Setenv("RUNNER_WORKER_NAME", testCase.env)
				}

				command := NewServeCommand()

				flagSet := console.NewFlagSet(command.Name(), io.Discard)

				command.Configure(flagSet)

				if err := flagSet.Parse(testCase.arguments); err != nil {
					t.Errorf("unexpected parsing error: %q", err)
				}

				if command.configs.Name != testCase.want {
					t.Errorf("unexpected name flag value, want %q got %q", testCase.want, command.configs.Name)
				}
			})
		}
	})

	t.Run("run", func(t *testing.T) {
		ctx := t.Context()

		consumerName := "01"

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
		consumer.On("Produce", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		defer consumer.AssertExpectations(t)

		var nodeManager containers.MockNodeManager
		nodeManager.On("Stats", mock.Anything, mock.Anything).Return(node.Stats{}, nil).Maybe()

		var containerManager containers.MockContainerManager
		containerManager.On("GetByLabel", mock.Anything, mock.Anything, mock.Anything).
			Return([]container.Container{}, nil).Maybe()

		command := NewServeCommand()
		command.configs.Name = consumerName
		command.configs.Port = findAvailablePort()
		command.handler = handler
		command.consumer = &consumer
		command.consumers = subscribers
		command.logger = slog.New(slog.NewTextHandler(io.Discard, nil))

		// the background work a running worker does. It is set here because
		// Run starts it, and a command assembled by hand has to be assembled
		// completely.
		command.logShipper = shipLogs.NewUseCase(&containerManager, &consumer, consumerName, command.logger)
		command.taskHeartBeat = taskHeartbeat.NewUseCase(&containerManager, &consumer, consumerName, "docker", command.logger)
		command.workerHeartBeat = workerHeartbeat.NewUseCase(&consumer, &nodeManager, consumerName, "worker:80")

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
