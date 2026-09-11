package tunnel

import (
	"context"
	"errors"
	"net"
)

var (
	// ErrUnauthenticated is a connection that did not prove what it claimed.
	ErrUnauthenticated = errors.New("tunnel: authentication failed")

	// ErrUnauthorized is a worker that proved who it is and still may not do
	// what it asked.
	ErrUnauthorized = errors.New("tunnel: not allowed")
)

// Identity is a worker as the ingress came to know it: who it said it was, and
// what was settled about it while it was being let in.
type Identity struct {
	// Worker is the name the worker is addressed by.
	Worker string

	// MaxSessions and MaxStreams are what this worker in particular may hold,
	// zero meaning the ingress's own limits apply. They are here so that
	// authorisation can differ per worker without the rest of the ingress
	// knowing that it can.
	MaxSessions int
	MaxStreams  int
}

// Authenticator decides whether a connection claiming to be a worker is one.
//
// It is given the connection as well as the claim, which is what lets the
// answer come from the peer's certificate rather than from anything it said —
// see CertificateAuthenticator, which is what production uses. Returning an
// Identity is what lets authentication also settle authorisation, in one place,
// before anything is registered.
type Authenticator interface {
	Authenticate(ctx context.Context, conn net.Conn, worker string, token string) (Identity, error)
}

// AuthenticatorFunc adapts a function to an Authenticator.
type AuthenticatorFunc func(ctx context.Context, conn net.Conn, worker string, token string) (Identity, error)

func (f AuthenticatorFunc) Authenticate(ctx context.Context, conn net.Conn, worker string, token string) (Identity, error) {
	return f(ctx, conn, worker, token)
}

// AllowAll takes any connection that names a worker, leaving who it is entirely
// to the transport. It is for tests, and for a tunnel whose mutual TLS has
// already settled identity by other means.
func AllowAll() Authenticator {
	return AuthenticatorFunc(func(_ context.Context, _ net.Conn, worker string, _ string) (Identity, error) {
		if len(worker) == 0 {
			return Identity{}, errors.Join(ErrUnauthenticated, errors.New("no worker name"))
		}

		return Identity{Worker: worker}, nil
	})
}
