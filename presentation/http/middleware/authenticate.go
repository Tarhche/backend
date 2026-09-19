package middleware

import (
	"errors"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

const (
	authenticationHeaderName   = "authorization"
	authenticationHeaderPrefix = "bearer "

	// websocketProtocolHeaderName and websocketBearerProtocol are how a browser
	// carries a token on a connection it cannot put a header on.
	websocketProtocolHeaderName = "sec-websocket-protocol"
	websocketBearerProtocol     = "bearer"
)

// WebSocketBearerProtocol is the subprotocol a client offers alongside its
// token, and the one a server echoes back to accept it.
const WebSocketBearerProtocol = websocketBearerProtocol

type Authenticate struct {
	next          http.Handler
	authenticator *auth.Authenticator
}

var _ http.Handler = &Authenticate{}

func NewAuthenticateMiddleware(next http.Handler, j *jwt.JWT, userRepository user.Repository) *Authenticate {
	return &Authenticate{
		next:          next,
		authenticator: auth.NewAuthenticator(j, userRepository),
	}
}

func (a *Authenticate) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	identity, err := a.authenticator.Authenticate(r.Context(), bearerToken(r))

	switch {
	case errors.Is(err, auth.ErrBanned):
		rw.WriteHeader(http.StatusForbidden)

		return
	case err != nil:
		rw.WriteHeader(http.StatusUnauthorized)

		return
	}

	// a shadow session is indistinguishable from the session of the person it
	// stands for -- that is what it is for -- so the trace is where it says who
	// is behind it, and what was done in their name.
	if len(identity.ImpersonatorUUID) > 0 {
		trace.SpanFromContext(r.Context()).SetAttributes(
			attribute.String("auth.user", identity.User.UUID),
			attribute.String("auth.impersonator", identity.ImpersonatorUUID),
		)
	}

	a.next.ServeHTTP(rw, r.WithContext(auth.IdentityToContext(r.Context(), identity)))
}

// bearerToken takes the token out of the request, and is shared by everything
// here that reads one.
//
// An Authorization header is where it normally is. A websocket opened from a
// browser has no way to set one -- the WebSocket constructor takes a url and a
// list of subprotocols and nothing else -- so a token offered as a subprotocol
// is read too: `new WebSocket(url, ["bearer", token])`. It goes in a header
// either way, so it stays out of the url, and out of every access log and
// proxy trace that records one.
func bearerToken(r *http.Request) string {
	offset := len(authenticationHeaderPrefix)
	if h := r.Header.Get(authenticationHeaderName); len(h) > offset {
		return (" " + h[offset:])[1:]
	}

	return subprotocolToken(r)
}

// subprotocolToken reads a token offered as a websocket subprotocol, which is
// "bearer" followed by the token itself.
func subprotocolToken(r *http.Request) string {
	offered := r.Header.Values(websocketProtocolHeaderName)
	if len(offered) == 0 {
		return ""
	}

	var values []string
	for _, header := range offered {
		for value := range strings.SplitSeq(header, ",") {
			if value = strings.TrimSpace(value); len(value) > 0 {
				values = append(values, value)
			}
		}
	}

	// the marker, then the token. Anything else is a subprotocol that means
	// something to somebody else.
	if len(values) != 2 || !strings.EqualFold(values[0], websocketBearerProtocol) {
		return ""
	}

	return values[1]
}
