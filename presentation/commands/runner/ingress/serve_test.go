package ingress

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/tunnel"
)

func TestServe(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		command := NewServeCommand()

		want := "serve-runner-ingress"
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

		want := "serve-runner-ingress [arguments]"
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

		files := testCertificates(t)

		command := NewServeCommand()
		command.configs.Port = findAvailablePort()
		command.configs.TunnelPort = findAvailablePort()
		command.configs.TunnelAuthority = files.Authority
		command.configs.TunnelCertificate = files.Certificate
		command.configs.TunnelKey = files.PrivateKey
		command.handler = handler
		command.logger = slog.New(slog.DiscardHandler)

		tunnelIngress, err := tunnel.NewIngress(tunnel.DefaultConfig(), tunnel.NewCertificateAuthenticator(nil, nil), command.logger)
		assert.NoError(t, err)
		command.tunnel = tunnelIngress

		forwards, err := command.configs.Forwards()
		assert.NoError(t, err)

		forwarder, err := tunnel.NewForwarder(tunnelIngress, command.logger, forwards...)
		assert.NoError(t, err)
		command.forwarder = forwarder

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

	t.Run("keys it cannot use stop it before it listens", func(t *testing.T) {
		command := NewServeCommand()
		command.configs.Port = findAvailablePort()
		command.configs.TunnelPort = findAvailablePort()
		command.logger = slog.New(slog.DiscardHandler)

		assert.Equal(t, console.ExitFailure, command.Run(t.Context()))
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

// testCertificates writes an authority and an ingress certificate under it, so
// the tunnel can listen without anything having to exist beforehand.
func testCertificates(t *testing.T) certificate.TLSFiles {
	t.Helper()

	directory := t.TempDir()

	authority, err := certificate.GenerateCA("test authority", 0)
	assert.NoError(t, err)

	authorityFiles := certificate.AuthorityFiles(filepath.Join(directory, "ca"))
	assert.NoError(t, certificate.Write(authorityFiles, authority.Certificate, authority.PrivateKey, false))

	issued, key, err := authority.GenerateServerCertificate(certificate.Request{Name: "runner-ingress"})
	assert.NoError(t, err)

	issuedFiles := certificate.IdentityFiles(filepath.Join(directory, "ingress"))
	assert.NoError(t, certificate.Write(issuedFiles, issued, key, false))

	return certificate.TLSFiles{
		Authority:   authorityFiles.Certificate,
		Certificate: issuedFiles.Certificate,
		PrivateKey:  issuedFiles.PrivateKey,
	}
}
