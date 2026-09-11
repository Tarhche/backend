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

// ServerTLS is what an ingress listens with: TLS 1.3, a certificate of its own,
// and a client certificate required and verified against the authority.
//
// Everything about who the peer is is settled here, before a byte of the
// protocol above is read. Nothing further down re-checks it, and nothing
// further down could overlook it.
func ServerTLS(files certificate.TLSFiles) (*tls.Config, error) {
	return certificate.LoadIngressTLSConfig(files)
}

// ClientTLS is what a worker dials with: TLS 1.3, its own certificate, the
// authority to check the ingress against, and the name that ingress has to
// answer for.
func ClientTLS(files certificate.TLSFiles) (*tls.Config, error) {
	return certificate.LoadWorkerTLSConfig(files)
}

// Listen returns a listener an ingress serves on. The handshake happens as the
// connection is accepted; smux is given a connection that is already mutually
// authenticated.
func Listen(address string, config *tls.Config) (net.Listener, error) {
	return tls.Listen("tcp", address, config)
}

// WorkerAuthorizer decides whether a worker that has proved who it is may do
// what it is asking to do.
//
// It is separate from authentication on purpose: TLS settles identity, and
// identity is not permission. An authority that signed a certificate two years
// ago has not thereby agreed to whatever that worker wants today.
type WorkerAuthorizer interface {
	Authorize(ctx context.Context, identity Identity) error
}

// WorkerAuthorizerFunc adapts a function to a WorkerAuthorizer.
type WorkerAuthorizerFunc func(ctx context.Context, identity Identity) error

func (f WorkerAuthorizerFunc) Authorize(ctx context.Context, identity Identity) error {
	return f(ctx, identity)
}

// AllowSignedWorkers lets in every worker the authority vouched for.
//
// It is the right default because the authority is private: something holding a
// certificate it signed is something that was deliberately given one. It is
// also the thing to replace first when that stops being true.
func AllowSignedWorkers() WorkerAuthorizer {
	return WorkerAuthorizerFunc(func(context.Context, Identity) error { return nil })
}

// AllowWorkers lets in only the workers named.
func AllowWorkers(names ...string) WorkerAuthorizer {
	allowed := slices.Clone(names)

	return WorkerAuthorizerFunc(func(_ context.Context, identity Identity) error {
		if slices.Contains(allowed, identity.Worker) {
			return nil
		}

		return fmt.Errorf("%w: %s is not one of the workers allowed here", ErrUnauthorized, identity.Worker)
	})
}

// CertificateAuthenticator takes a worker's identity from the certificate it
// already proved it holds, and asks an authorizer whether that worker may stay.
//
// Nothing is taken from what the worker *said*: the name in the registration is
// checked against the certificate rather than trusted, so a worker holding a
// valid certificate still cannot claim to be a different one.
type CertificateAuthenticator struct {
	identifier certificate.Identifier
	authorizer WorkerAuthorizer

	// MaxSessions and MaxStreams are what a worker let in this way may hold.
	MaxSessions int
	MaxStreams  int
}

var _ Authenticator = &CertificateAuthenticator{}

func NewCertificateAuthenticator(identifier certificate.Identifier, authorizer WorkerAuthorizer) *CertificateAuthenticator {
	if identifier == nil {
		identifier = certificate.SubjectAlternativeName("")
	}

	if authorizer == nil {
		authorizer = AllowSignedWorkers()
	}

	return &CertificateAuthenticator{identifier: identifier, authorizer: authorizer}
}

func (a *CertificateAuthenticator) Authenticate(ctx context.Context, conn net.Conn, worker string, _ string) (Identity, error) {
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

	// what it says it is has to be what its certificate says it is. A worker
	// with a valid certificate is still only that worker.
	if len(worker) > 0 && worker != name {
		return Identity{}, fmt.Errorf("%w: it registered as %s and its certificate says %s", ErrUnauthenticated, worker, name)
	}

	identity := Identity{Worker: name, MaxSessions: a.MaxSessions, MaxStreams: a.MaxStreams}

	if err := a.authorizer.Authorize(ctx, identity); err != nil {
		return Identity{}, err
	}

	return identity, nil
}
