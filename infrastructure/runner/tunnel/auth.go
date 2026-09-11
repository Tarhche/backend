package tunnel

import (
	"context"
	"crypto/subtle"
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
// It is given the connection as well as the claim, so an implementation can
// look at the peer — a client certificate, an address — rather than only at
// what was said. Returning an Identity is what lets authentication also settle
// authorisation, in one place, before anything is registered.
type Authenticator interface {
	Authenticate(ctx context.Context, conn net.Conn, worker string, token string) (Identity, error)
}

// AuthenticatorFunc adapts a function to an Authenticator.
type AuthenticatorFunc func(ctx context.Context, conn net.Conn, worker string, token string) (Identity, error)

func (f AuthenticatorFunc) Authenticate(ctx context.Context, conn net.Conn, worker string, token string) (Identity, error) {
	return f(ctx, conn, worker, token)
}

// TokenAuthenticator takes a worker that holds the shared token.
//
// It is the least that can be asked for and it is only sound because the
// connection is already encrypted: the token crosses inside TLS, never on its
// own. It says nothing about *which* worker is connecting — anything holding
// the token may claim any name — which is why the door is left open for
// certificates to say that instead, by way of the connection an Authenticator
// is handed.
type TokenAuthenticator struct {
	token []byte

	// MaxSessions and MaxStreams are what any worker let in this way may hold.
	MaxSessions int
	MaxStreams  int
}

var _ Authenticator = &TokenAuthenticator{}

func NewTokenAuthenticator(token string) *TokenAuthenticator {
	return &TokenAuthenticator{token: []byte(token)}
}

func (a *TokenAuthenticator) Authenticate(_ context.Context, _ net.Conn, worker string, token string) (Identity, error) {
	if len(a.token) == 0 {
		return Identity{}, errors.Join(ErrUnauthenticated, errors.New("the ingress was given no token to check against"))
	}

	// compared in constant time: a token checked byte by byte can be guessed
	// byte by byte.
	if subtle.ConstantTimeCompare([]byte(token), a.token) != 1 {
		return Identity{}, ErrUnauthenticated
	}

	if len(worker) == 0 {
		return Identity{}, errors.Join(ErrUnauthenticated, errors.New("no worker name"))
	}

	return Identity{Worker: worker, MaxSessions: a.MaxSessions, MaxStreams: a.MaxStreams}, nil
}

// AllowAll takes anything. It exists for tests and for a tunnel that is already
// protected by mutual TLS, where the certificate has already said who this is.
func AllowAll() Authenticator {
	return AuthenticatorFunc(func(_ context.Context, _ net.Conn, worker string, _ string) (Identity, error) {
		if len(worker) == 0 {
			return Identity{}, errors.Join(ErrUnauthenticated, errors.New("no worker name"))
		}

		return Identity{Worker: worker}, nil
	})
}
