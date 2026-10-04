package providers

import (
	"context"
	"log/slog"
	"reflect"
	"testing"

	"github.com/danceable/container"
	"github.com/danceable/provider"
	"github.com/danceable/provider/adapters/danceable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	networkContract "github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/configs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	infraContainer "github.com/khanzadimahdi/testproject/infrastructure/workload/container"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/client"
	infraNetwork "github.com/khanzadimahdi/testproject/infrastructure/workload/network"
	infraNode "github.com/khanzadimahdi/testproject/infrastructure/workload/node"
)

// discardingLoggers binds the named loggers providers resolve, as the
// OpenTelemetry provider does, without sending anything anywhere.
type discardingLoggers struct{}

func (discardingLoggers) Register(ctx context.Context, c provider.Container) error {
	return c.Bind(func(name string) *slog.Logger { return slog.New(slog.DiscardHandler) }, provider.Lazy())
}

func (discardingLoggers) Boot(ctx context.Context, c provider.Container) error { return nil }

func (discardingLoggers) Terminate(ctx context.Context) error { return nil }

// wired is what a provider bound for the three contracts an orchestrator's use
// cases run tasks with: the type of each implementation, or why it could not be
// resolved.
type wired struct {
	err      string
	runtime  reflect.Type
	networks reflect.Type
	nodes    reflect.Type
}

// wiring boots one runtime provider with an orchestrator's configuration, on a
// container of its own, and says what it bound.
func wiring(t *testing.T, orchestrator *configs.WorkloadOrchestrator, runtimeProvider provider.Provider) wired {
	t.Helper()

	manager := provider.New(danceable.New(container.New()))
	manager.Register(NewConfigsProvider(orchestrator))
	manager.Register(discardingLoggers{})
	manager.Register(runtimeProvider)

	var got wired

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})

	err := manager.Run(ctx, provider.WithTerminationDelay(0), provider.WithCallback(func(ctx context.Context, c provider.Container) {
		defer close(done)
		defer cancel()

		var runtime task.Runtime
		if err := c.Resolve(&runtime); err == nil {
			got.runtime = reflect.TypeOf(runtime)
		}

		var networks networkContract.Manager
		if err := c.Resolve(&networks); err == nil {
			got.networks = reflect.TypeOf(networks)
		}

		var nodes node.Manager
		if err := c.Resolve(&nodes); err == nil {
			got.nodes = reflect.TypeOf(nodes)
		}
	}))
	if err != nil {
		got.err = err.Error()

		return got
	}

	<-done

	return got
}

// tunnelCredentials are an orchestrator's tunnel certificate and key, and the
// authority that signed them, as PEM.
func tunnelCredentials(t *testing.T) (authority string, cert string, key string) {
	t.Helper()

	ca, err := certificate.GenerateCA("workload test authority", 0)
	require.NoError(t, err)

	issued, private, err := ca.GenerateClientCertificate(certificate.Request{Name: "workload-orchestrator-01"})
	require.NoError(t, err)

	encodedKey, err := certificate.EncodePrivateKey(private)
	require.NoError(t, err)

	return string(certificate.EncodeCertificate(ca.Certificate)), string(certificate.EncodeCertificate(issued)), string(encodedKey)
}

func TestWorkloadRuntimeProvider(t *testing.T) {
	t.Run("an orchestrator that names no runtime is wired exactly as the docker provider wired it", func(t *testing.T) {
		// what DOCKER_HOST can be: a daemon of its own, the local socket, and
		// nothing at all, which the docker client refuses. Whatever the docker
		// provider made of each, the runtime provider makes the same, its
		// failures included.
		dockerHosts := map[string]string{
			"a daemon of its own":    "tcp://docker:2375",
			"the local socket":       "unix:///var/run/docker.sock",
			"no daemon named at all": "",
		}

		for name, dockerHost := range dockerHosts {
			t.Run(name, func(t *testing.T) {
				orchestrator := func() *configs.WorkloadOrchestrator {
					c := configs.NewWorkloadOrchestrator()
					c.Name = "workload-orchestrator-01"
					c.DockerHost = dockerHost

					return c
				}

				before := wiring(t, orchestrator(), NewDockerProvider())
				after := wiring(t, orchestrator(), NewWorkloadRuntimeProvider())

				assert.Equal(t, before, after)

				if len(dockerHost) == 0 {
					assert.Contains(t, after.err, "unable to parse docker host")

					return
				}

				assert.Empty(t, after.err)
				assert.Equal(t, reflect.TypeOf(&infraContainer.DockerManager{}), after.runtime)
				assert.Equal(t, reflect.TypeOf(&infraNetwork.Manager{}), after.networks)
				assert.Equal(t, reflect.TypeOf(&infraNode.DockerManager{}), after.nodes)
			})
		}
	})

	t.Run("sysbox, named, is the same", func(t *testing.T) {
		c := configs.NewWorkloadOrchestrator()
		c.Name = "workload-orchestrator-01"
		c.DockerHost = "tcp://docker:2375"
		c.Runtime = configs.RuntimeSysbox

		got := wiring(t, c, NewWorkloadRuntimeProvider())

		assert.Empty(t, got.err)
		assert.Equal(t, reflect.TypeOf(&infraContainer.DockerManager{}), got.runtime)
		assert.Equal(t, reflect.TypeOf(&infraNetwork.Manager{}), got.networks)
		assert.Equal(t, reflect.TypeOf(&infraNode.DockerManager{}), got.nodes)
	})

	t.Run("a configuration made without its defaults runs on sysbox too", func(t *testing.T) {
		c := &configs.WorkloadOrchestrator{Name: "workload-orchestrator-01", DockerHost: "tcp://docker:2375"}

		got := wiring(t, c, NewWorkloadRuntimeProvider())

		assert.Empty(t, got.err)
		assert.Equal(t, reflect.TypeOf(&infraContainer.DockerManager{}), got.runtime)
	})

	t.Run("an orchestrator on microsandbox is wired to the service's client, and needs no docker", func(t *testing.T) {
		authority, cert, key := tunnelCredentials(t)

		c := configs.NewWorkloadOrchestrator()
		c.Name = "workload-orchestrator-01"
		c.Runtime = configs.RuntimeMicrosandbox
		c.TunnelAuthority = authority
		c.TunnelCertificate = cert
		c.TunnelKey = key

		got := wiring(t, c, NewWorkloadRuntimeProvider())

		assert.Empty(t, got.err)
		assert.Equal(t, reflect.TypeOf(&client.Runtime{}), got.runtime)
		assert.Equal(t, reflect.TypeOf(&client.NetworkManager{}), got.networks)
		assert.Equal(t, reflect.TypeOf(&client.NodeManager{}), got.nodes)
	})

	t.Run("an orchestrator told to run on anything else does not start", func(t *testing.T) {
		for _, runtime := range []string{"kata", "Sysbox", " microsandbox"} {
			c := configs.NewWorkloadOrchestrator()
			c.Name = "workload-orchestrator-01"
			c.DockerHost = "tcp://docker:2375"
			c.Runtime = runtime

			got := wiring(t, c, NewWorkloadRuntimeProvider())

			assert.Equal(t, `WORKLOAD_ORCHESTRATOR_RUNTIME is "`+runtime+`", and an orchestrator runs its tasks on sysbox or microsandbox`, got.err)
		}
	})

	t.Run("an orchestrator on microsandbox without the tunnel's certificates does not start", func(t *testing.T) {
		c := configs.NewWorkloadOrchestrator()
		c.Name = "workload-orchestrator-01"
		c.Runtime = configs.RuntimeMicrosandbox

		got := wiring(t, c, NewWorkloadRuntimeProvider())

		assert.Equal(t, "workload-microsandbox: certificate: no certificate was given", got.err)
	})

	t.Run("an orchestrator on microsandbox that cannot say where the service is does not start", func(t *testing.T) {
		authority, cert, key := tunnelCredentials(t)

		for _, url := range []string{"", "http://workload-microsandbox:8443", "https://workload-microsandbox:8443/v1", "workload-microsandbox:8443"} {
			c := configs.NewWorkloadOrchestrator()
			c.Name = "workload-orchestrator-01"
			c.Runtime = configs.RuntimeMicrosandbox
			c.MicrosandboxURL = url
			c.TunnelAuthority = authority
			c.TunnelCertificate = cert
			c.TunnelKey = key

			got := wiring(t, c, NewWorkloadRuntimeProvider())

			assert.Contains(t, got.err, client.ErrNotAService.Error(), url)
		}
	})

	t.Run("an orchestrator on microsandbox with no name does not start, since its runs would belong to nobody", func(t *testing.T) {
		authority, cert, key := tunnelCredentials(t)

		c := configs.NewWorkloadOrchestrator()
		c.Runtime = configs.RuntimeMicrosandbox
		c.TunnelAuthority = authority
		c.TunnelCertificate = cert
		c.TunnelKey = key

		got := wiring(t, c, NewWorkloadRuntimeProvider())

		assert.Contains(t, got.err, "was not told which")
	})
}
