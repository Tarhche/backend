package providers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danceable/container"
	"github.com/danceable/provider"
	"github.com/danceable/provider/adapters/danceable"
	"github.com/docker/docker/api"
	"github.com/docker/docker/api/types/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	networkContract "github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
)

// loggers binds the named loggers the providers resolve, as the
// OpenTelemetry provider does, without sending anything anywhere.
type loggers struct{}

func (loggers) Register(ctx context.Context, c provider.Container) error {
	return c.Bind(func(name string) *slog.Logger { return slog.New(slog.DiscardHandler) }, provider.Lazy())
}

func (loggers) Boot(ctx context.Context, c provider.Container) error { return nil }

func (loggers) Terminate(ctx context.Context) error { return nil }

// daemon is a docker daemon that says what it is and nothing else.
func daemon(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Api-Version", api.DefaultVersion)

		if strings.HasSuffix(r.URL.Path, "/info") {
			rw.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(rw).Encode(system.Info{ServerVersion: "28.5.2", NCPU: 4, MemTotal: 8 << 30, Architecture: "x86_64"})

			return
		}

		rw.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	return server
}

// booted runs the runtimes provider with an orchestrator's configuration, and
// hands what it bound to inspect before it is terminated.
func booted(t *testing.T, orchestrator *configs.WorkloadOrchestrator, inspect func(c provider.Container)) error {
	t.Helper()

	manager := provider.New(danceable.New(container.New()))
	manager.Register(NewConfigsProvider(orchestrator))
	manager.Register(loggers{})
	manager.Register(NewRuntimesProvider())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return manager.Run(ctx, provider.WithCallback(func(ctx context.Context, c provider.Container) {
		defer cancel()

		inspect(c)
	}))
}

func TestRuntimesProvider(t *testing.T) {
	t.Run("an orchestrator configured as it always was offers sysbox on its docker daemon", func(t *testing.T) {
		server := daemon(t)

		orchestrator := configs.NewWorkloadOrchestrator()
		orchestrator.Name = "workload-orchestrator-01"
		orchestrator.DockerHost = "tcp://" + server.Listener.Addr().String()

		var (
			offers   []runtime.Offer
			resolved = map[string]error{}
		)

		err := booted(t, orchestrator, func(c provider.Container) {
			var drivers driver.Set
			resolved["driver.Set"] = c.Resolve(&drivers)

			var tasks task.Runtime
			resolved["task.Runtime"] = c.Resolve(&tasks)

			var dialer task.Dialer
			resolved["task.Dialer"] = c.Resolve(&dialer)

			var networks networkContract.Manager
			resolved["network.Manager"] = c.Resolve(&networks)

			var nodes node.Manager
			resolved["node.Manager"] = c.Resolve(&nodes)

			if drivers != nil {
				for _, d := range drivers.All() {
					offers = append(offers, d.Offer(context.Background()))
				}
			}
		})
		require.NoError(t, err)

		for contract, err := range resolved {
			assert.NoError(t, err, contract)
		}

		require.Len(t, offers, 1)
		assert.Equal(t, runtime.Sysbox, offers[0].Class)
		assert.Equal(t, "container", offers[0].Driver)
		assert.True(t, offers[0].Healthy, offers[0].Reason)
		assert.Equal(t, []string{"amd64"}, offers[0].Capabilities.Architectures)
	})

	t.Run("one configured with a kind it has no driver for does not start", func(t *testing.T) {
		orchestrator := configs.NewWorkloadOrchestrator()
		orchestrator.Name = "workload-orchestrator-01"
		orchestrator.Runtimes = "kata=kata@unix:///run/kata.sock"

		err := booted(t, orchestrator, func(c provider.Container) {})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `"kata" kind`)
	})
}
