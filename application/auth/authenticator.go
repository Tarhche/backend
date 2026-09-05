package auth

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

var (
	// ErrUnauthenticated means the token did not identify anyone: it was
	// missing, expired, forged, for another purpose, or names a user who is no
	// longer there.
	ErrUnauthenticated = errors.New("the token does not identify a user")

	// ErrBanned means the token identified someone who may no longer act.
	ErrBanned = errors.New("the user is banned")
)

// Authenticator turns an access token into the user it stands for.
//
// It is the one place that decides what a token proves, so the HTTP middleware
// and everything else that has to authenticate outside a request — a websocket
// handshake carries no Authorization header, so a token reaches those over the
// connection itself — agree on the answer.
type Authenticator struct {
	jwt            *jwt.JWT
	userRepository user.Repository
}

func NewAuthenticator(j *jwt.JWT, userRepository user.Repository) *Authenticator {
	return &Authenticator{
		jwt:            j,
		userRepository: userRepository,
	}
}

// Authenticate reports who a token stands for. The audience is checked as well
// as the signature, so a refresh or a registration token cannot be used to act
// as its subject.
func (a *Authenticator) Authenticate(ctx context.Context, token string) (user.User, error) {
	if len(token) == 0 {
		return user.User{}, ErrUnauthenticated
	}

	claims, err := a.jwt.Verify(ctx, token)
	if err != nil {
		return user.User{}, ErrUnauthenticated
	}

	audiences, err := claims.GetAudience()
	if err != nil || len(audiences) == 0 || audiences[0] != AccessToken {
		return user.User{}, ErrUnauthenticated
	}

	userUUID, err := claims.GetSubject()
	if err != nil {
		return user.User{}, ErrUnauthenticated
	}

	u, err := a.userRepository.GetOne(ctx, userUUID)
	if err != nil {
		return user.User{}, ErrUnauthenticated
	}

	if u.IsBanned() {
		return user.User{}, ErrBanned
	}

	return u, nil
}
