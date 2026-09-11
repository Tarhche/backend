package tunnel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
)

// pki is an authority and somewhere to keep what it signs, the way a deployment
// has one.
type pki struct {
	directory string
	authority *certificate.Authority
	files     certificate.TLSFiles
}

func newPKI(t *testing.T) *pki {
	t.Helper()

	directory := t.TempDir()

	authority, err := certificate.GenerateCA("test authority", 0)
	require.NoError(t, err)

	files := certificate.AuthorityFiles(filepath.Join(directory, "ca"))
	require.NoError(t, certificate.Write(files, authority.Certificate, authority.PrivateKey, false))

	return &pki{
		directory: directory,
		authority: authority,
		files:     certificate.TLSFiles{Authority: files.Certificate},
	}
}

// issue writes a certificate under the authority and returns what to load it
// with. A validity in the past or the future is how the expiry tests are made.
func (p *pki) issue(t *testing.T, name string, server bool, request certificate.Request) certificate.TLSFiles {
	t.Helper()

	request.Name = name

	issue := p.authority.GenerateClientCertificate
	if server {
		issue = p.authority.GenerateServerCertificate
	}

	issued, key, err := issue(request)
	require.NoError(t, err)

	files := certificate.IdentityFiles(filepath.Join(p.directory, name))
	require.NoError(t, certificate.Write(files, issued, key, true))

	return certificate.TLSFiles{
		Authority:   p.files.Authority,
		Certificate: files.Certificate,
		PrivateKey:  files.PrivateKey,
	}
}

// ingressFiles and workerFiles are the ordinary cases.
func (p *pki) ingressFiles(t *testing.T) certificate.TLSFiles {
	return p.issue(t, "ingress.example.internal", true, certificate.Request{})
}

func (p *pki) workerFiles(t *testing.T, name string) certificate.TLSFiles {
	files := p.issue(t, name, false, certificate.Request{})
	files.ServerName = "ingress.example.internal"

	return files
}

// mtlsTunnel stands an ingress up on real mTLS and returns how to reach it.
func mtlsTunnel(t *testing.T, p *pki, ingress certificate.TLSFiles, options ...IngressOption) (*Ingress, string) {
	t.Helper()

	config := testConfig()

	auth := NewCertificateAuthenticator(certificate.SubjectAlternativeName(""), AllowSignedWorkers())

	server, err := NewIngress(config, auth, discardLogger(), options...)
	require.NoError(t, err)

	tlsConfig, err := ServerTLS(ingress)
	require.NoError(t, err)

	listener, err := Listen("127.0.0.1:0", tlsConfig)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})

	go func() {
		defer close(done)

		_ = server.Serve(ctx, listener)
	}()

	t.Cleanup(func() {
		cancel()
		server.Close()
		<-done
	})

	return server, listener.Addr().String()
}

// connect dials an ingress the way a worker does, and reports what happened.
func connect(t *testing.T, address string, files certificate.TLSFiles) (net.Conn, error) {
	t.Helper()

	config, err := ClientTLS(files)
	if err != nil {
		return nil, err
	}

	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: config}

	return dialer.DialContext(t.Context(), "tcp", address)
}

// 4. A successful mTLS handshake, and 20/21. smux and many streams over it
func TestMutualTLS(t *testing.T) {
	t.Run("a worker the authority signed for gets in, and its streams work", func(t *testing.T) {
		p := newPKI(t)
		ingress, address := mtlsTunnel(t, p, p.ingressFiles(t))

		config := testConfig()
		worker, err := NewWorker("worker-001", []string{address}, config,
			TLSDialer(mustClientTLS(t, p.workerFiles(t, "worker-001")), config.DialTimeout),
			NewServiceTargets(map[string]string{"echo": echoServer(t, "")}), discardLogger())
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(func() { cancel(); worker.Close() })

		go worker.Run(ctx)

		waitFor(t, "the worker to get in", func() bool { return len(ingress.Workers()) == 1 })

		// 21. many streams over the one mutually authenticated connection
		var conns []net.Conn
		defer func() {
			for _, conn := range conns {
				conn.Close()
			}
		}()

		for i := range 8 {
			conn, err := ingress.Dial(t.Context(), "worker-001", Target{Service: "echo"})
			require.NoError(t, err)

			conns = append(conns, conn)

			message := fmt.Sprintf("stream-%d", i)
			assert.Equal(t, message, roundTrip(t, conn, message))
		}
	})

	t.Run("the handshake happens before smux, so an unauthenticated peer never speaks it", func(t *testing.T) {
		p := newPKI(t)
		ingress, address := mtlsTunnel(t, p, p.ingressFiles(t))

		// a plain tcp connection, offering no certificate at all
		conn, err := net.DialTimeout("tcp", address, 5*time.Second)
		require.NoError(t, err)
		defer conn.Close()

		require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

		// it speaks the protocol above TLS, which is not the one being spoken
		_, err = conn.Write([]byte(`{"version":1,"worker":"worker-001"}` + "\n"))
		require.NoError(t, err, "the bytes reach the socket; what answers them is the question")

		// what comes back is a TLS alert and then the connection, rather than
		// anything the registration would have been answered with
		answer, _ := io.ReadAll(conn)
		assert.NotContains(t, string(answer), `"ok"`, "it was answered as if it had registered")

		time.Sleep(200 * time.Millisecond)
		assert.Empty(t, ingress.Workers(), "a peer that did not complete the handshake registered anyway")
	})
}

// 5 & 16. An unknown authority, from either side
func TestUnknownAuthority(t *testing.T) {
	t.Run("a worker signed by another authority is refused", func(t *testing.T) {
		ours := newPKI(t)
		theirs := newPKI(t)

		_, address := mtlsTunnel(t, ours, ours.ingressFiles(t))

		// the stranger's certificate, but our authority to check the ingress
		stranger := theirs.workerFiles(t, "worker-001")
		stranger.Authority = ours.files.Authority

		conn, err := connect(t, address, stranger)
		if err == nil {
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			_, err = io.ReadFull(conn, make([]byte, 1))
			conn.Close()
		}

		assert.Error(t, err, "a certificate the authority did not sign should not get in")
	})

	t.Run("an ingress signed by another authority is not talked to", func(t *testing.T) {
		ours := newPKI(t)
		theirs := newPKI(t)

		// an ingress holding a certificate from an authority the worker does
		// not trust
		_, address := mtlsTunnel(t, theirs, theirs.issue(t, "ingress.example.internal", true, certificate.Request{}))

		files := ours.workerFiles(t, "worker-001")

		_, err := connect(t, address, files)
		assert.Error(t, err, "a worker should not hand its credentials to an ingress it cannot verify")
		assert.Contains(t, err.Error(), "certificate")
	})
}

// 6 & 7. Expired certificates, either end
func TestExpiredCertificates(t *testing.T) {
	t.Run("an expired worker certificate is refused", func(t *testing.T) {
		p := newPKI(t)
		_, address := mtlsTunnel(t, p, p.ingressFiles(t))

		// a validity so short it has already passed by the time it is used
		expired := p.issue(t, "worker-expired", false, certificate.Request{Validity: time.Nanosecond})
		expired.ServerName = "ingress.example.internal"

		conn, err := connect(t, address, expired)
		if err == nil {
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			_, err = io.ReadFull(conn, make([]byte, 1))
			conn.Close()
		}

		assert.Error(t, err)
	})

	t.Run("an expired ingress certificate is not talked to", func(t *testing.T) {
		p := newPKI(t)

		expired := p.issue(t, "ingress.example.internal", true, certificate.Request{Validity: time.Nanosecond})
		_, address := mtlsTunnel(t, p, expired)

		_, err := connect(t, address, p.workerFiles(t, "worker-001"))

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expired")
	})
}

// 8 & 9. The wrong extended key usage, either end
func TestExtendedKeyUsage(t *testing.T) {
	t.Run("a worker offering a server certificate is refused", func(t *testing.T) {
		p := newPKI(t)
		_, address := mtlsTunnel(t, p, p.ingressFiles(t))

		// signed by the right authority, but for serving rather than connecting
		wrong := p.issue(t, "worker-server", true, certificate.Request{})
		wrong.ServerName = "ingress.example.internal"

		conn, err := connect(t, address, wrong)
		if err == nil {
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			_, err = io.ReadFull(conn, make([]byte, 1))
			conn.Close()
		}

		assert.Error(t, err, "clientAuth is what a client certificate is for")
	})

	t.Run("an ingress offering a client certificate is not talked to", func(t *testing.T) {
		p := newPKI(t)

		wrong := p.issue(t, "ingress.example.internal", false, certificate.Request{})
		_, address := mtlsTunnel(t, p, wrong)

		_, err := connect(t, address, p.workerFiles(t, "worker-001"))

		assert.Error(t, err, "serverAuth is what a server certificate is for")
	})
}

// 10 & 11. The wrong name
func TestServerName(t *testing.T) {
	t.Run("an ingress answering for another name is not talked to", func(t *testing.T) {
		p := newPKI(t)
		_, address := mtlsTunnel(t, p, p.issue(t, "somebody-else.example.internal", true, certificate.Request{}))

		files := p.workerFiles(t, "worker-001") // expects ingress.example.internal

		_, err := connect(t, address, files)
		assert.Error(t, err)
	})

	t.Run("a worker that was told no name will not connect at all", func(t *testing.T) {
		p := newPKI(t)

		files := p.workerFiles(t, "worker-001")
		files.ServerName = ""

		_, err := ClientTLS(files)
		assert.Error(t, err, "without a name a worker would trust anything the authority signed, including another worker")
	})

	t.Run("an address in the certificate is answered for", func(t *testing.T) {
		p := newPKI(t)

		files := p.issue(t, "ingress.example.internal", true, certificate.Request{
			IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		})

		_, address := mtlsTunnel(t, p, files)

		worker := p.workerFiles(t, "worker-001")
		worker.ServerName = "127.0.0.1"

		conn, err := connect(t, address, worker)
		require.NoError(t, err)
		conn.Close()
	})
}

// 12, 13 & 14. Missing and mismatched files
func TestBadFiles(t *testing.T) {
	p := newPKI(t)
	files := p.workerFiles(t, "worker-001")

	t.Run("a certificate that is not there", func(t *testing.T) {
		broken := files
		broken.Certificate = filepath.Join(t.TempDir(), "missing.crt")

		_, err := ClientTLS(broken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing.crt")
	})

	t.Run("a private key that is not there", func(t *testing.T) {
		broken := files
		broken.PrivateKey = filepath.Join(t.TempDir(), "missing.key")

		_, err := ClientTLS(broken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing.key")
	})

	t.Run("an authority that is not there", func(t *testing.T) {
		broken := files
		broken.Authority = filepath.Join(t.TempDir(), "missing.crt")

		_, err := ClientTLS(broken)
		assert.Error(t, err)
	})

	t.Run("a key that belongs to another certificate", func(t *testing.T) {
		other := p.workerFiles(t, "worker-002")

		broken := files
		broken.PrivateKey = other.PrivateKey

		_, err := ClientTLS(broken)
		assert.Error(t, err, "a key that does not match its certificate is the commonest misconfiguration there is")
	})
}

// 17 & 18. Several workers, told apart by their certificates
func TestManyWorkersByCertificate(t *testing.T) {
	p := newPKI(t)
	ingress, address := mtlsTunnel(t, p, p.ingressFiles(t))

	config := testConfig()
	config.MinSessions = 1

	for _, name := range []string{"worker-001", "worker-002", "worker-003"} {
		worker, err := NewWorker(name, []string{address}, config,
			TLSDialer(mustClientTLS(t, p.workerFiles(t, name)), config.DialTimeout),
			NewServiceTargets(map[string]string{"echo": echoServer(t, name+":")}), discardLogger())
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(func() { cancel(); worker.Close() })

		go worker.Run(ctx)
	}

	waitFor(t, "all three workers", func() bool { return len(ingress.Workers()) == 3 })

	// each is known by the name in its own certificate, and reaches its own target
	for _, name := range []string{"worker-001", "worker-002", "worker-003"} {
		conn, err := ingress.Dial(t.Context(), name, Target{Service: "echo"})
		require.NoError(t, err)

		greeting := make([]byte, len(name)+1)
		require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
		_, err = io.ReadFull(conn, greeting)
		require.NoError(t, err)

		assert.Equal(t, name+":", string(greeting))
		conn.Close()
	}
}

// A worker cannot claim to be a different worker, however valid its certificate
func TestIdentityIsTheCertificate(t *testing.T) {
	p := newPKI(t)
	ingress, address := mtlsTunnel(t, p, p.ingressFiles(t))

	config := testConfig()
	config.MinSessions = 1

	// it holds worker-001's certificate and registers as worker-002
	worker, err := NewWorker("worker-002", []string{address}, config,
		TLSDialer(mustClientTLS(t, p.workerFiles(t, "worker-001")), config.DialTimeout),
		NewServiceTargets(map[string]string{"echo": echoServer(t, "")}), discardLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(func() { cancel(); worker.Close() })

	go worker.Run(ctx)

	time.Sleep(500 * time.Millisecond)

	assert.Empty(t, ingress.Workers(), "a worker holding one certificate should not be able to register as another")
}

// 15 & 19. Authorization, which is a separate question from authentication
func TestWorkerAuthorization(t *testing.T) {
	t.Run("only the workers named are let in", func(t *testing.T) {
		p := newPKI(t)

		config := testConfig()
		config.MinSessions = 1

		auth := NewCertificateAuthenticator(
			certificate.SubjectAlternativeName(""),
			AllowWorkers("worker-001"),
		)

		ingress, err := NewIngress(config, auth, discardLogger())
		require.NoError(t, err)

		tlsConfig, err := ServerTLS(p.ingressFiles(t))
		require.NoError(t, err)

		listener, err := Listen("127.0.0.1:0", tlsConfig)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})

		go func() {
			defer close(done)

			_ = ingress.Serve(ctx, listener)
		}()

		t.Cleanup(func() { cancel(); ingress.Close(); <-done })

		for _, name := range []string{"worker-001", "worker-002"} {
			worker, err := NewWorker(name, []string{listener.Addr().String()}, config,
				TLSDialer(mustClientTLS(t, p.workerFiles(t, name)), config.DialTimeout),
				NewServiceTargets(map[string]string{"echo": echoServer(t, "")}), discardLogger())
			require.NoError(t, err)

			workerCtx, workerCancel := context.WithCancel(t.Context())
			t.Cleanup(func() { workerCancel(); worker.Close() })

			go worker.Run(workerCtx)
		}

		waitFor(t, "the allowed worker", func() bool { return len(ingress.Workers()) == 1 })

		time.Sleep(300 * time.Millisecond)

		workers := ingress.Workers()
		require.Len(t, workers, 1, "a worker the authority signed for is still not automatically allowed")
		assert.Equal(t, "worker-001", workers[0].Worker)
	})

	t.Run("an authorizer that refuses says so", func(t *testing.T) {
		err := AllowWorkers("worker-001").Authorize(t.Context(), Identity{Worker: "worker-002"})

		assert.ErrorIs(t, err, ErrUnauthorized)
		assert.Contains(t, err.Error(), "worker-002")
	})

	t.Run("every worker the authority signed for, when none are named", func(t *testing.T) {
		assert.NoError(t, AllowSignedWorkers().Authorize(t.Context(), Identity{Worker: "anybody"}))
	})
}

// 22. A worker reconnecting after the ingress went, with the same certificate
func TestReconnectOverMutualTLS(t *testing.T) {
	p := newPKI(t)
	ingressFiles := p.ingressFiles(t)

	config := testConfig()
	config.MinSessions = 1

	tlsConfig, err := ServerTLS(ingressFiles)
	require.NoError(t, err)

	listener, err := Listen("127.0.0.1:0", tlsConfig)
	require.NoError(t, err)

	address := listener.Addr().String()

	first, err := NewIngress(config, NewCertificateAuthenticator(nil, nil), discardLogger())
	require.NoError(t, err)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)

		_ = first.Serve(t.Context(), listener)
	}()

	worker, err := NewWorker("worker-001", []string{address}, config,
		TLSDialer(mustClientTLS(t, p.workerFiles(t, "worker-001")), config.DialTimeout),
		NewServiceTargets(map[string]string{"echo": echoServer(t, "")}), discardLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(func() { cancel(); worker.Close() })

	go worker.Run(ctx)

	waitFor(t, "the worker", func() bool { return len(first.Workers()) == 1 })

	listener.Close()
	first.Close()
	<-firstDone

	// the ingress comes back with the same certificate, and the worker finds it
	again, err := Listen(address, tlsConfig)
	require.NoError(t, err)

	second, err := NewIngress(config, NewCertificateAuthenticator(nil, nil), discardLogger())
	require.NoError(t, err)

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)

		_ = second.Serve(t.Context(), again)
	}()

	t.Cleanup(func() { second.Close(); <-secondDone })

	waitFor(t, "the worker to reconnect", func() bool { return len(second.Workers()) == 1 })

	conn, err := second.Dial(t.Context(), "worker-001", Target{Service: "echo"})
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "after the restart", roundTrip(t, conn, "after the restart"))
}

// 23. Rotation: the certificate is read through a callback, so swapping what it
// closes over is all a reload would be. Nothing above holds the certificate.
func TestRotationDesign(t *testing.T) {
	p := newPKI(t)

	t.Run("an ingress asks for its certificate each time rather than holding one", func(t *testing.T) {
		config, err := ServerTLS(p.ingressFiles(t))
		require.NoError(t, err)

		require.NotNil(t, config.GetCertificate, "a certificate read from a field cannot be replaced while running")
		assert.Empty(t, config.Certificates)

		first, err := config.GetCertificate(&tls.ClientHelloInfo{})
		require.NoError(t, err)

		second, err := config.GetCertificate(&tls.ClientHelloInfo{})
		require.NoError(t, err)

		assert.Equal(t, first, second)
	})

	t.Run("a worker asks for its certificate each time too", func(t *testing.T) {
		config, err := ClientTLS(p.workerFiles(t, "worker-001"))
		require.NoError(t, err)

		require.NotNil(t, config.GetClientCertificate)
		assert.Empty(t, config.Certificates)

		certificate, err := config.GetClientCertificate(&tls.CertificateRequestInfo{})
		require.NoError(t, err)
		assert.NotNil(t, certificate)
	})

	t.Run("both ends insist on TLS 1.3", func(t *testing.T) {
		server, err := ServerTLS(p.ingressFiles(t))
		require.NoError(t, err)

		client, err := ClientTLS(p.workerFiles(t, "worker-001"))
		require.NoError(t, err)

		assert.EqualValues(t, tls.VersionTLS13, server.MinVersion)
		assert.EqualValues(t, tls.VersionTLS13, client.MinVersion)
	})

	t.Run("verification is never skipped, and a client certificate is always required", func(t *testing.T) {
		server, err := ServerTLS(p.ingressFiles(t))
		require.NoError(t, err)

		client, err := ClientTLS(p.workerFiles(t, "worker-001"))
		require.NoError(t, err)

		assert.Equal(t, tls.RequireAndVerifyClientCert, server.ClientAuth)
		assert.False(t, server.InsecureSkipVerify)
		assert.False(t, client.InsecureSkipVerify)
		assert.NotNil(t, server.ClientCAs)
		assert.NotNil(t, client.RootCAs)
		assert.NotEmpty(t, client.ServerName)
	})
}

func TestCertificateAuthenticator(t *testing.T) {
	t.Run("a connection that is not encrypted is nobody", func(t *testing.T) {
		_, conn := net.Pipe()
		defer conn.Close()

		_, err := NewCertificateAuthenticator(nil, nil).Authenticate(t.Context(), conn, "worker-001", "")

		assert.ErrorIs(t, err, ErrUnauthenticated)
	})

	t.Run("a certificate with no name identifies nobody", func(t *testing.T) {
		identifier := certificate.IdentifierFunc(func(*x509.Certificate) (string, error) {
			return "", certificate.ErrNoIdentity
		})

		auth := NewCertificateAuthenticator(identifier, nil)

		_, conn := net.Pipe()
		defer conn.Close()

		_, err := auth.Authenticate(t.Context(), conn, "worker-001", "")
		assert.ErrorIs(t, err, ErrUnauthenticated)
	})
}

func mustClientTLS(t *testing.T, files certificate.TLSFiles) *tls.Config {
	t.Helper()

	config, err := ClientTLS(files)
	require.NoError(t, err)

	return config
}
