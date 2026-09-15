package tunnel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
)

// pki is an authority and what it signs, the way a deployment has one.
type pki struct {
	authority   *certificate.Authority
	credentials certificate.Credentials
}

func newPKI(t *testing.T) *pki {
	t.Helper()

	authority, err := certificate.GenerateCA("test authority", 0)
	require.NoError(t, err)

	return &pki{
		authority:   authority,
		credentials: certificate.Credentials{Authority: string(certificate.EncodeCertificate(authority.Certificate))},
	}
}

// issue signs a certificate under the authority and returns what to configure
// an end with. A validity in the past or the future is how the expiry tests are
// made.
func (p *pki) issue(t *testing.T, name string, server bool, request certificate.Request) certificate.Credentials {
	t.Helper()

	request.Name = name

	issue := p.authority.GenerateClientCertificate
	if server {
		issue = p.authority.GenerateServerCertificate
	}

	issued, key, err := issue(request)
	require.NoError(t, err)

	keyPEM, err := certificate.EncodePrivateKey(key)
	require.NoError(t, err)

	return certificate.Credentials{
		Authority:   p.credentials.Authority,
		Certificate: string(certificate.EncodeCertificate(issued)),
		PrivateKey:  string(keyPEM),
	}
}

// hubFiles and agentFiles are the ordinary cases.
func (p *pki) hubFiles(t *testing.T) certificate.Credentials {
	return p.issue(t, "hub.example.internal", true, certificate.Request{})
}

func (p *pki) agentFiles(t *testing.T, name string) certificate.Credentials {
	files := p.issue(t, name, false, certificate.Request{})
	files.ServerName = "hub.example.internal"

	return files
}

// mtlsTunnel stands a hub up on real mTLS and returns how to reach it.
func mtlsTunnel(t *testing.T, p *pki, hub certificate.Credentials, options ...HubOption) (*Hub, string) {
	t.Helper()

	config := testConfig()

	auth := NewCertificateAuthenticator(certificate.SubjectAlternativeName(""), AllowSignedAgents())

	server, err := NewHub(config, auth, discardLogger(), options...)
	require.NoError(t, err)

	tlsConfig, err := ServerTLS(hub)
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

// connect dials a hub the way an agent does, and reports what happened.
func connect(t *testing.T, address string, files certificate.Credentials) (net.Conn, error) {
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
	t.Run("an agent the authority signed for gets in, and its streams work", func(t *testing.T) {
		p := newPKI(t)
		hub, address := mtlsTunnel(t, p, p.hubFiles(t))

		config := testConfig()
		agent, err := NewAgent("agent-001", []string{address}, config,
			TLSDialer(mustClientTLS(t, p.agentFiles(t, "agent-001")), config.DialTimeout),
			NewServiceTargets(map[string]string{"echo": echoServer(t, "")}), discardLogger())
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(func() { cancel(); agent.Close() })

		go agent.Run(ctx)

		waitFor(t, "the agent to get in", func() bool { return len(hub.Agents()) == 1 })

		// 21. many streams over the one mutually authenticated connection
		var conns []net.Conn
		defer func() {
			for _, conn := range conns {
				conn.Close()
			}
		}()

		for i := range 8 {
			conn, err := hub.Dial(t.Context(), "agent-001", Target{Service: "echo"})
			require.NoError(t, err)

			conns = append(conns, conn)

			message := fmt.Sprintf("stream-%d", i)
			assert.Equal(t, message, roundTrip(t, conn, message))
		}
	})

	t.Run("the handshake happens before smux, so an unauthenticated peer never speaks it", func(t *testing.T) {
		p := newPKI(t)
		hub, address := mtlsTunnel(t, p, p.hubFiles(t))

		// a plain tcp connection, offering no certificate at all
		conn, err := net.DialTimeout("tcp", address, 5*time.Second)
		require.NoError(t, err)
		defer conn.Close()

		require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))

		// it speaks the protocol above TLS, which is not the one being spoken
		_, err = conn.Write([]byte(`{"version":1,"agent":"agent-001"}` + "\n"))
		require.NoError(t, err, "the bytes reach the socket; what answers them is the question")

		// what comes back is a TLS alert and then the connection, rather than
		// anything the registration would have been answered with
		answer, _ := io.ReadAll(conn)
		assert.NotContains(t, string(answer), `"ok"`, "it was answered as if it had registered")

		time.Sleep(200 * time.Millisecond)
		assert.Empty(t, hub.Agents(), "a peer that did not complete the handshake registered anyway")
	})
}

// 5 & 16. An unknown authority, from either side
func TestUnknownAuthority(t *testing.T) {
	t.Run("an agent signed by another authority is refused", func(t *testing.T) {
		ours := newPKI(t)
		theirs := newPKI(t)

		_, address := mtlsTunnel(t, ours, ours.hubFiles(t))

		// the stranger's certificate, but our authority to check the hub
		stranger := theirs.agentFiles(t, "agent-001")
		stranger.Authority = ours.credentials.Authority

		conn, err := connect(t, address, stranger)
		if err == nil {
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			_, err = io.ReadFull(conn, make([]byte, 1))
			conn.Close()
		}

		assert.Error(t, err, "a certificate the authority did not sign should not get in")
	})

	t.Run("a hub signed by another authority is not talked to", func(t *testing.T) {
		ours := newPKI(t)
		theirs := newPKI(t)

		// a hub holding a certificate from an authority the agent does
		// not trust
		_, address := mtlsTunnel(t, theirs, theirs.issue(t, "hub.example.internal", true, certificate.Request{}))

		files := ours.agentFiles(t, "agent-001")

		_, err := connect(t, address, files)
		assert.Error(t, err, "an agent should not hand its credentials to a hub it cannot verify")
		assert.Contains(t, err.Error(), "certificate")
	})
}

// 6 & 7. Expired certificates, either end
func TestExpiredCertificates(t *testing.T) {
	t.Run("an expired agent certificate is refused", func(t *testing.T) {
		p := newPKI(t)
		_, address := mtlsTunnel(t, p, p.hubFiles(t))

		// a validity so short it has already passed by the time it is used
		expired := p.issue(t, "agent-expired", false, certificate.Request{Validity: time.Nanosecond})
		expired.ServerName = "hub.example.internal"

		conn, err := connect(t, address, expired)
		if err == nil {
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			_, err = io.ReadFull(conn, make([]byte, 1))
			conn.Close()
		}

		assert.Error(t, err)
	})

	t.Run("an expired hub certificate is not talked to", func(t *testing.T) {
		p := newPKI(t)

		expired := p.issue(t, "hub.example.internal", true, certificate.Request{Validity: time.Nanosecond})
		_, address := mtlsTunnel(t, p, expired)

		_, err := connect(t, address, p.agentFiles(t, "agent-001"))

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expired")
	})
}

// 8 & 9. The wrong extended key usage, either end
func TestExtendedKeyUsage(t *testing.T) {
	t.Run("an agent offering a server certificate is refused", func(t *testing.T) {
		p := newPKI(t)
		_, address := mtlsTunnel(t, p, p.hubFiles(t))

		// signed by the right authority, but for serving rather than connecting
		wrong := p.issue(t, "agent-server", true, certificate.Request{})
		wrong.ServerName = "hub.example.internal"

		conn, err := connect(t, address, wrong)
		if err == nil {
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			_, err = io.ReadFull(conn, make([]byte, 1))
			conn.Close()
		}

		assert.Error(t, err, "clientAuth is what a client certificate is for")
	})

	t.Run("a hub offering a client certificate is not talked to", func(t *testing.T) {
		p := newPKI(t)

		wrong := p.issue(t, "hub.example.internal", false, certificate.Request{})
		_, address := mtlsTunnel(t, p, wrong)

		_, err := connect(t, address, p.agentFiles(t, "agent-001"))

		assert.Error(t, err, "serverAuth is what a server certificate is for")
	})
}

// 10 & 11. The wrong name
func TestServerName(t *testing.T) {
	t.Run("a hub answering for another name is not talked to", func(t *testing.T) {
		p := newPKI(t)
		_, address := mtlsTunnel(t, p, p.issue(t, "somebody-else.example.internal", true, certificate.Request{}))

		files := p.agentFiles(t, "agent-001") // expects hub.example.internal

		_, err := connect(t, address, files)
		assert.Error(t, err)
	})

	t.Run("an agent that was told no name will not connect at all", func(t *testing.T) {
		p := newPKI(t)

		files := p.agentFiles(t, "agent-001")
		files.ServerName = ""

		_, err := ClientTLS(files)
		assert.Error(t, err, "without a name an agent would trust anything the authority signed, including another agent")
	})

	t.Run("an address in the certificate is answered for", func(t *testing.T) {
		p := newPKI(t)

		files := p.issue(t, "hub.example.internal", true, certificate.Request{
			IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		})

		_, address := mtlsTunnel(t, p, files)

		agent := p.agentFiles(t, "agent-001")
		agent.ServerName = "127.0.0.1"

		conn, err := connect(t, address, agent)
		require.NoError(t, err)
		conn.Close()
	})
}

// 12, 13 & 14. Missing and mismatched credentials
func TestBadFiles(t *testing.T) {
	p := newPKI(t)
	files := p.agentFiles(t, "agent-001")

	t.Run("a certificate that was not given", func(t *testing.T) {
		broken := files
		broken.Certificate = ""

		_, err := ClientTLS(broken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no certificate")
	})

	t.Run("a private key that was not given", func(t *testing.T) {
		broken := files
		broken.PrivateKey = ""

		_, err := ClientTLS(broken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no private key")
	})

	t.Run("an authority that is not a certificate", func(t *testing.T) {
		broken := files
		broken.Authority = "not a certificate"

		_, err := ClientTLS(broken)
		assert.Error(t, err)
	})

	t.Run("a key that belongs to another certificate", func(t *testing.T) {
		other := p.agentFiles(t, "agent-002")

		broken := files
		broken.PrivateKey = other.PrivateKey

		_, err := ClientTLS(broken)
		assert.Error(t, err, "a key that does not match its certificate is the commonest misconfiguration there is")
	})
}

// 17 & 18. Several agents, told apart by their certificates
func TestManyAgentsByCertificate(t *testing.T) {
	p := newPKI(t)
	hub, address := mtlsTunnel(t, p, p.hubFiles(t))

	config := testConfig()
	config.MinSessions = 1

	for _, name := range []string{"agent-001", "agent-002", "agent-003"} {
		agent, err := NewAgent(name, []string{address}, config,
			TLSDialer(mustClientTLS(t, p.agentFiles(t, name)), config.DialTimeout),
			NewServiceTargets(map[string]string{"echo": echoServer(t, name+":")}), discardLogger())
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(func() { cancel(); agent.Close() })

		go agent.Run(ctx)
	}

	waitFor(t, "all three agents", func() bool { return len(hub.Agents()) == 3 })

	// each is known by the name in its own certificate, and reaches its own target
	for _, name := range []string{"agent-001", "agent-002", "agent-003"} {
		conn, err := hub.Dial(t.Context(), name, Target{Service: "echo"})
		require.NoError(t, err)

		greeting := make([]byte, len(name)+1)
		require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
		_, err = io.ReadFull(conn, greeting)
		require.NoError(t, err)

		assert.Equal(t, name+":", string(greeting))
		conn.Close()
	}
}

// An agent cannot claim to be a different agent, however valid its certificate
func TestIdentityIsTheCertificate(t *testing.T) {
	p := newPKI(t)
	hub, address := mtlsTunnel(t, p, p.hubFiles(t))

	config := testConfig()
	config.MinSessions = 1

	// it holds agent-001's certificate and registers as agent-002
	agent, err := NewAgent("agent-002", []string{address}, config,
		TLSDialer(mustClientTLS(t, p.agentFiles(t, "agent-001")), config.DialTimeout),
		NewServiceTargets(map[string]string{"echo": echoServer(t, "")}), discardLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(func() { cancel(); agent.Close() })

	go agent.Run(ctx)

	time.Sleep(500 * time.Millisecond)

	assert.Empty(t, hub.Agents(), "an agent holding one certificate should not be able to register as another")
}

// 15 & 19. Authorization, which is a separate question from authentication
func TestAgentAuthorization(t *testing.T) {
	t.Run("only the agents named are let in", func(t *testing.T) {
		p := newPKI(t)

		config := testConfig()
		config.MinSessions = 1

		auth := NewCertificateAuthenticator(
			certificate.SubjectAlternativeName(""),
			AllowAgents("agent-001"),
		)

		hub, err := NewHub(config, auth, discardLogger())
		require.NoError(t, err)

		tlsConfig, err := ServerTLS(p.hubFiles(t))
		require.NoError(t, err)

		listener, err := Listen("127.0.0.1:0", tlsConfig)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})

		go func() {
			defer close(done)

			_ = hub.Serve(ctx, listener)
		}()

		t.Cleanup(func() { cancel(); hub.Close(); <-done })

		for _, name := range []string{"agent-001", "agent-002"} {
			agent, err := NewAgent(name, []string{listener.Addr().String()}, config,
				TLSDialer(mustClientTLS(t, p.agentFiles(t, name)), config.DialTimeout),
				NewServiceTargets(map[string]string{"echo": echoServer(t, "")}), discardLogger())
			require.NoError(t, err)

			agentCtx, agentCancel := context.WithCancel(t.Context())
			t.Cleanup(func() { agentCancel(); agent.Close() })

			go agent.Run(agentCtx)
		}

		waitFor(t, "the allowed agent", func() bool { return len(hub.Agents()) == 1 })

		time.Sleep(300 * time.Millisecond)

		agents := hub.Agents()
		require.Len(t, agents, 1, "an agent the authority signed for is still not automatically allowed")
		assert.Equal(t, "agent-001", agents[0].Name)
	})

	t.Run("an authorizer that refuses says so", func(t *testing.T) {
		err := AllowAgents("agent-001").Authorize(t.Context(), Identity{Name: "agent-002"})

		assert.ErrorIs(t, err, ErrUnauthorized)
		assert.Contains(t, err.Error(), "agent-002")
	})

	t.Run("every agent the authority signed for, when none are named", func(t *testing.T) {
		assert.NoError(t, AllowSignedAgents().Authorize(t.Context(), Identity{Name: "anybody"}))
	})
}

// 22. An agent reconnecting after the hub went, with the same certificate
func TestReconnectOverMutualTLS(t *testing.T) {
	p := newPKI(t)
	hubFiles := p.hubFiles(t)

	config := testConfig()
	config.MinSessions = 1

	tlsConfig, err := ServerTLS(hubFiles)
	require.NoError(t, err)

	listener, err := Listen("127.0.0.1:0", tlsConfig)
	require.NoError(t, err)

	address := listener.Addr().String()

	first, err := NewHub(config, NewCertificateAuthenticator(nil, nil), discardLogger())
	require.NoError(t, err)

	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)

		_ = first.Serve(t.Context(), listener)
	}()

	agent, err := NewAgent("agent-001", []string{address}, config,
		TLSDialer(mustClientTLS(t, p.agentFiles(t, "agent-001")), config.DialTimeout),
		NewServiceTargets(map[string]string{"echo": echoServer(t, "")}), discardLogger())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(func() { cancel(); agent.Close() })

	go agent.Run(ctx)

	waitFor(t, "the agent", func() bool { return len(first.Agents()) == 1 })

	listener.Close()
	first.Close()
	<-firstDone

	// the hub comes back with the same certificate, and the agent finds it
	again, err := Listen(address, tlsConfig)
	require.NoError(t, err)

	second, err := NewHub(config, NewCertificateAuthenticator(nil, nil), discardLogger())
	require.NoError(t, err)

	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)

		_ = second.Serve(t.Context(), again)
	}()

	t.Cleanup(func() { second.Close(); <-secondDone })

	waitFor(t, "the agent to reconnect", func() bool { return len(second.Agents()) == 1 })

	conn, err := second.Dial(t.Context(), "agent-001", Target{Service: "echo"})
	require.NoError(t, err)
	defer conn.Close()

	assert.Equal(t, "after the restart", roundTrip(t, conn, "after the restart"))
}

// 23. Rotation: the certificate is read through a callback, so swapping what it
// closes over is all a reload would be. Nothing above holds the certificate.
func TestRotationDesign(t *testing.T) {
	p := newPKI(t)

	t.Run("a hub asks for its certificate each time rather than holding one", func(t *testing.T) {
		config, err := ServerTLS(p.hubFiles(t))
		require.NoError(t, err)

		require.NotNil(t, config.GetCertificate, "a certificate read from a field cannot be replaced while running")
		assert.Empty(t, config.Certificates)

		first, err := config.GetCertificate(&tls.ClientHelloInfo{})
		require.NoError(t, err)

		second, err := config.GetCertificate(&tls.ClientHelloInfo{})
		require.NoError(t, err)

		assert.Equal(t, first, second)
	})

	t.Run("an agent asks for its certificate each time too", func(t *testing.T) {
		config, err := ClientTLS(p.agentFiles(t, "agent-001"))
		require.NoError(t, err)

		require.NotNil(t, config.GetClientCertificate)
		assert.Empty(t, config.Certificates)

		certificate, err := config.GetClientCertificate(&tls.CertificateRequestInfo{})
		require.NoError(t, err)
		assert.NotNil(t, certificate)
	})

	t.Run("both ends insist on TLS 1.3", func(t *testing.T) {
		server, err := ServerTLS(p.hubFiles(t))
		require.NoError(t, err)

		client, err := ClientTLS(p.agentFiles(t, "agent-001"))
		require.NoError(t, err)

		assert.EqualValues(t, tls.VersionTLS13, server.MinVersion)
		assert.EqualValues(t, tls.VersionTLS13, client.MinVersion)
	})

	t.Run("verification is never skipped, and a client certificate is always required", func(t *testing.T) {
		server, err := ServerTLS(p.hubFiles(t))
		require.NoError(t, err)

		client, err := ClientTLS(p.agentFiles(t, "agent-001"))
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

		_, err := NewCertificateAuthenticator(nil, nil).Authenticate(t.Context(), conn, "agent-001", "")

		assert.ErrorIs(t, err, ErrUnauthenticated)
	})

	t.Run("a certificate with no name identifies nobody", func(t *testing.T) {
		identifier := certificate.IdentifierFunc(func(*x509.Certificate) (string, error) {
			return "", certificate.ErrNoIdentity
		})

		auth := NewCertificateAuthenticator(identifier, nil)

		_, conn := net.Pipe()
		defer conn.Close()

		_, err := auth.Authenticate(t.Context(), conn, "agent-001", "")
		assert.ErrorIs(t, err, ErrUnauthenticated)
	})
}

func mustClientTLS(t *testing.T, files certificate.Credentials) *tls.Config {
	t.Helper()

	config, err := ClientTLS(files)
	require.NoError(t, err)

	return config
}
