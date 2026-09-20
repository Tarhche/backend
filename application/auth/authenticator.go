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

// Identity is who a request is from: the user whose session it is and, when
// somebody obtained that session to be seen as them, who that somebody is.
type Identity struct {
	User user.User

	// ImpersonatorUUID is who obtained this token to be seen as User. It is
	// empty for an ordinary session, and is what the dashboard shows when it
	// says whose eyes these are.
	ImpersonatorUUID string

	// Permissions are what the token says its holder may do. They are a hint,
	// good enough to decide what to offer somebody; whoever has to refuse a
	// request asks the authorizer, which reads the roles as they are now.
	Permissions []string
}

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
//
// Who the token says is behind it is taken from the token itself and nowhere
// else: only this estate signs one, so a caller cannot claim to be acting for
// somebody who never asked them to.
func (a *Authenticator) Authenticate(ctx context.Context, token string) (Identity, error) {
	if len(token) == 0 {
		return Identity{}, ErrUnauthenticated
	}

	claims, err := a.jwt.Verify(ctx, token)
	if err != nil {
		return Identity{}, ErrUnauthenticated
	}

	audiences, err := claims.GetAudience()
	if err != nil || len(audiences) == 0 || audiences[0] != AccessToken {
		return Identity{}, ErrUnauthenticated
	}

	userUUID, err := claims.GetSubject()
	if err != nil {
		return Identity{}, ErrUnauthenticated
	}

	u, err := a.userRepository.GetOne(ctx, userUUID)
	if err != nil {
		return Identity{}, ErrUnauthenticated
	}

	if u.IsBanned() {
		return Identity{}, ErrBanned
	}

	return Identity{
		User:             u,
		ImpersonatorUUID: jwt.Impersonator(claims),
		Permissions:      jwt.Permissions(claims),
	}, nil
}
