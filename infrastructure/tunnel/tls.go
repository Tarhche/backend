package tunnel

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"slices"

	"github.com/khanzadimahdi/testproject/infrastructure/crypto/certificate"
)

// ServerTLS is what a hub listens with: TLS 1.3, a certificate of its own,
// and a client certificate required and verified against the authority.
//
// Everything about who the peer is is settled here, before a byte of the
// protocol above is read. Nothing further down re-checks it, and nothing
// further down could overlook it.
func ServerTLS(credentials certificate.Credentials) (*tls.Config, error) {
	return certificate.ServerTLSConfig(credentials)
}

// ClientTLS is what an agent dials with: TLS 1.3, its own certificate, the
// authority to check the hub against, and the name that hub has to
// answer for.
func ClientTLS(credentials certificate.Credentials) (*tls.Config, error) {
	return certificate.ClientTLSConfig(credentials)
}

// Listen returns a listener a hub serves on. The handshake happens as the
// connection is accepted; smux is given a connection that is already mutually
// authenticated.
func Listen(address string, config *tls.Config) (net.Listener, error) {
	return tls.Listen("tcp", address, config)
}

// AgentAuthorizer decides whether an agent that has proved who it is may do
// what it is asking to do.
//
// It is separate from authentication on purpose: TLS settles identity, and
// identity is not permission. An authority that signed a certificate two years
// ago has not thereby agreed to whatever that agent wants today.
type AgentAuthorizer interface {
	Authorize(ctx context.Context, identity Identity) error
}

// AgentAuthorizerFunc adapts a function to a AgentAuthorizer.
type AgentAuthorizerFunc func(ctx context.Context, identity Identity) error

func (f AgentAuthorizerFunc) Authorize(ctx context.Context, identity Identity) error {
	return f(ctx, identity)
}

// AllowSignedAgents lets in every agent the authority vouched for.
//
// It is the right default because the authority is private: something holding a
// certificate it signed is something that was deliberately given one. It is
// also the thing to replace first when that stops being true.
func AllowSignedAgents() AgentAuthorizer {
	return AgentAuthorizerFunc(func(context.Context, Identity) error { return nil })
}

// AllowAgents lets in only the agents named.
func AllowAgents(names ...string) AgentAuthorizer {
	allowed := slices.Clone(names)

	return AgentAuthorizerFunc(func(_ context.Context, identity Identity) error {
		if slices.Contains(allowed, identity.Name) {
			return nil
		}

		return fmt.Errorf("%w: %s is not one of the agents allowed here", ErrUnauthorized, identity.Name)
	})
}

// CertificateAuthenticator takes an agent's identity from the certificate it
// already proved it holds, and asks an authorizer whether that agent may stay.
//
// Nothing is taken from what the agent *said*: the name in the registration is
// checked against the certificate rather than trusted, so an agent holding a
// valid certificate still cannot claim to be a different one.
type CertificateAuthenticator struct {
	identifier certificate.Identifier
	authorizer AgentAuthorizer

	// MaxSessions and MaxStreams are what an agent let in this way may hold.
	MaxSessions int
	MaxStreams  int
}

var _ Authenticator = &CertificateAuthenticator{}

func NewCertificateAuthenticator(identifier certificate.Identifier, authorizer AgentAuthorizer) *CertificateAuthenticator {
	if identifier == nil {
		identifier = certificate.SubjectAlternativeName("")
	}

	if authorizer == nil {
		authorizer = AllowSignedAgents()
	}

	return &CertificateAuthenticator{identifier: identifier, authorizer: authorizer}
}

func (a *CertificateAuthenticator) Authenticate(ctx context.Context, conn net.Conn, agent string, _ string) (Identity, error) {
	secure, ok := conn.(*tls.Conn)
	if !ok {
		return Identity{}, errors.Join(ErrUnauthenticated, errors.New("the connection is not encrypted"))
	}

	state := secure.ConnectionState()
	if !state.HandshakeComplete {
		return Identity{}, errors.Join(ErrUnauthenticated, errors.New("the handshake is not finished"))
	}

	// the chains are what TLS *verified*, not what the peer offered. Reading
	// the offered certificate instead would be reading an unchecked claim.
	if len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 {
		return Identity{}, errors.Join(ErrUnauthenticated, errors.New("no verified client certificate"))
	}

	name, err := a.identifier.Identify(state.VerifiedChains[0][0])
	if err != nil {
		return Identity{}, errors.Join(ErrUnauthenticated, err)
	}

	// what it says it is has to be what its certificate says it is. An agent
	// with a valid certificate is still only that agent.
	if len(agent) > 0 && agent != name {
		return Identity{}, fmt.Errorf("%w: it registered as %s and its certificate says %s", ErrUnauthenticated, agent, name)
	}

	identity := Identity{Name: name, MaxSessions: a.MaxSessions, MaxStreams: a.MaxStreams}

	if err := a.authorizer.Authorize(ctx, identity); err != nil {
		return Identity{}, err
	}

	return identity, nil
}
