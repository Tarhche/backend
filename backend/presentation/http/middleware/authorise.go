package middleware

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type Authorise struct {
	next           http.Handler
	j              *jwt.JWT
	userRepository user.Repository
}

var _ http.Handler = &Authorise{}

func NewAuthoriseMiddleware(next http.Handler, j *jwt.JWT, userRepository user.Repository) *Authorise {
	return &Authorise{
		j:              j,
		userRepository: userRepository,
		next:           next,
	}
}

func (a *Authorise) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	token := a.bearerToken(r)
	claims, err := a.j.Verify(token)
	if err != nil {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	if audiences, err := claims.GetAudience(); err != nil || len(audiences) == 0 || audiences[0] != auth.AccessToken {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	userUUID, err := claims.GetSubject()
	if err != nil {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	user, err := a.userRepository.GetOne(userUUID)
	if err != nil {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	a.next.ServeHTTP(rw, r.WithContext(auth.ToContext(r.Context(), &user)))
}

func (a *Authorise) bearerToken(r *http.Request) string {
	offset := len("bearer ")
	h := r.Header.Get("authorization")
	if len(h) <= offset {
		return ""
	}

	return (" " + h[offset:])[1:]
}
