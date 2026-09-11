package middleware

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

const (
	authenticationHeaderName   = "authorization"
	authenticationHeaderPrefix = "bearer "
)

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
	user, err := a.authenticator.Authenticate(r.Context(), a.bearerToken(r))

	switch {
	case errors.Is(err, auth.ErrBanned):
		rw.WriteHeader(http.StatusForbidden)

		return
	case err != nil:
		rw.WriteHeader(http.StatusUnauthorized)

		return
	}

	a.next.ServeHTTP(rw, r.WithContext(auth.ToContext(r.Context(), &user)))
}

func (a *Authenticate) bearerToken(r *http.Request) string {
	offset := len(authenticationHeaderPrefix)
	h := r.Header.Get(authenticationHeaderName)
	if len(h) <= offset {
		return ""
	}

	return (" " + h[offset:])[1:]
}
