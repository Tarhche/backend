//go:build microsandbox

package microsandbox

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/danceable/console"
	"github.com/danceable/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	checkhealth "github.com/khanzadimahdi/testproject/application/app/checkHealth"
	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	fakes "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/microsandbox"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/microsandbox/api"
	healthAPI "github.com/khanzadimahdi/testproject/presentation/http/health"
	microsandboxAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/microsandbox"
)

func TestServe(t *testing.T) {
	t.Run("name, description and usage", func(t *testing.T) {
		command := NewServeCommand(nil)

		assert.Equal(t, "serve-workload-microsandbox", command.Name())
		assert.Equal(t, "serves microsandbox's VMs to the workload orchestrators.", command.Description())
		assert.Equal(t, "serve-workload-microsandbox [arguments]", command.Usage())
	})

	t.Run("configure", func(t *testing.T) {
		command := NewServeCommand(nil)

		flagSet := console.NewFlagSet(command.Name(), io.Discard)
		command.Configure(flagSet)

		port := flagSet.Lookup("port")
		require.NotNil(t, port)
		assert.Equal(t, "SERVER_PORT", port.Env())
		assert.Equal(t, 8443, command.configs.Port)

		require.NoError(t, flagSet.Parse([]string{"--port", "9443", "--state-dir", "/state"}))

		assert.Equal(t, 9443, command.configs.Port)
		assert.Equal(t, "/state", command.configs.StateDir)
	})

	t.Run("the providers end with the service's own, after the one that binds microsandbox", func(t *testing.T) {
		sandboxes := &fakeProvider{}

		command := NewServeCommand(sandboxes)

		list := command.Providers()

		assert.Same(t, command, list[len(list)-1])
		assert.Same(t, sandboxes, list[len(list)-3])
	})

	t.Run("run serves the API under mutual TLS and the healthcheck on loopback, and stops every run on its way out", func(t *testing.T) {
		credentials, client := testCertificates(t)

		fake := fakes.NewFakeSandboxes()
		fake.CacheImage("busybox", runs.ImageConfig{Cmd: []string{"sh"}})

		command := NewServeCommand(nil)
		command.configs.Port = findAvailablePort(t)
		command.configs.Authority = credentials.Authority
		command.configs.Certificate = credentials.Certificate
		command.configs.Key = credentials.PrivateKey
		command.healthAddress = fmt.Sprintf("127.0.0.1:%d", findAvailablePort(t))
		command.logger = slog.New(slog.DiscardHandler)

		config := runs.DefaultConfig()
		config.Budget = 1 << 30
		config.StopGrace = 50 * time.Millisecond

		command.supervisor = runs.New(fake, fakes.NewMemoryRecords(), fakes.NewMemoryJournal(), fakes.NewMemoryHostPorts(20000, 20100), config, command.logger)
		command.handler = microsandboxAPI.NewHandler(command.supervisor, command.logger)
		command.health = healthAPI.NewHealthHandler(checkhealth.NewUseCase(checkhealth.Dependency{Name: "microsandbox", Pinger: command.supervisor}))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		exited := make(chan console.ExitStatus, 1)
		go func() { exited <- command.Run(ctx) }()

		require.Eventually(t, func() bool {
			response, err := http.Get("http://" + command.healthAddress + "/health")
			if err != nil {
				return false
			}
			defer response.Body.Close()

			return response.StatusCode == http.StatusOK
		}, 5*time.Second, 10*time.Millisecond, "the service never became ready")

		base := fmt.Sprintf("https://127.0.0.1:%d", command.configs.Port)

		response, err := client.Get(base + "/v1/info")
		require.NoError(t, err)
		defer response.Body.Close()

		var info api.Info
		require.NoError(t, json.NewDecoder(response.Body).Decode(&info))
		assert.True(t, info.Ready)

		run, err := command.supervisor.Create(context.Background(), api.RunSpec{
			Node: "orchestrator-1", Name: "web", Image: "busybox", Memory: 64 << 20, Network: api.NetworkIsolated, RestartPolicy: "always",
		})
		require.NoError(t, err)

		_, err = command.supervisor.Start(context.Background(), run.ID)
		require.NoError(t, err)

		anonymous := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
		_, err = anonymous.Get(base + "/v1/info")
		assert.Error(t, err, "a client without a certificate is not let in")

		cancel()

		select {
		case status := <-exited:
			assert.Equal(t, console.ExitSuccess, status)
		case <-time.After(10 * time.Second):
			t.Fatal("the service did not stop")
		}

		stopped, err := command.supervisor.Get(run.ID)
		require.NoError(t, err)

		assert.Equal(t, api.StateExited, stopped.State)
		assert.Equal(t, runs.ReasonServiceRestarted, stopped.Error)
	})

	t.Run("certificates it cannot use stop it before it listens", func(t *testing.T) {
		command := NewServeCommand(nil)
		command.configs.Port = findAvailablePort(t)
		command.logger = slog.New(slog.DiscardHandler)

		assert.Equal(t, console.ExitFailure, command.Run(context.Background()))
	})
}

// testCertificates signs an authority, the service's certificate and an
// orchestrator's under it, and is the service's credentials and a client
// holding the orchestrator's.
func testCertificates(t *testing.T) (certificate.Credentials, *http.Client) {
	t.Helper()

	authority, err := certificate.GenerateCA("test authority", 0)
	require.NoError(t, err)

	authorityPEM := string(certificate.EncodeCertificate(authority.Certificate))

	serverCertificate, serverKey, err := authority.GenerateServerCertificate(certificate.Request{
		Name:        "workload-microsandbox",
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	})
	require.NoError(t, err)

	clientCertificate, clientKey, err := authority.GenerateClientCertificate(certificate.Request{Name: "orchestrator-1"})
	require.NoError(t, err)

	serverKeyPEM, err := certificate.EncodePrivateKey(serverKey)
	require.NoError(t, err)

	clientKeyPEM, err := certificate.EncodePrivateKey(clientKey)
	require.NoError(t, err)

	clientTLS, err := certificate.ClientTLSConfig(certificate.Credentials{
		Authority:   authorityPEM,
		Certificate: string(certificate.EncodeCertificate(clientCertificate)),
		PrivateKey:  string(clientKeyPEM),
		ServerName:  "workload-microsandbox",
	})
	require.NoError(t, err)

	return certificate.Credentials{
		Authority:   authorityPEM,
		Certificate: string(certificate.EncodeCertificate(serverCertificate)),
		PrivateKey:  string(serverKeyPEM),
	}, &http.Client{
		Transport: &http.Transport{TLSClientConfig: clientTLS},
		Timeout:   5 * time.Second,
	}
}

func findAvailablePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port
}

// fakeProvider stands in for the provider that binds microsandbox.
type fakeProvider struct{}

func (p *fakeProvider) Register(context.Context, provider.Container) error { return nil }
func (p *fakeProvider) Boot(context.Context, provider.Container) error     { return nil }
func (p *fakeProvider) Terminate(context.Context) error                    { return nil }
