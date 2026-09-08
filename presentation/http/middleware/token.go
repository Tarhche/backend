package middleware

import (
	"context"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

// subjectKey carries who a request is from, for a handler that needs to know.
type subjectKey struct{}

// Token establishes who a request is from by its access token, and establishes
// nothing else.
//
// Whether a caller has to carry one at all is the two constructors below: some
// of what a node holds is nobody's and open to everybody.
//
// It verifies the signature and the audience and then takes the subject at its
// word. There is no user looked up, no ban checked, and no permission read,
// because a worker has no database to read any of them from -- what it has is
// the containers themselves, and what it decides it decides from those.
//
// So this says "the estate signed this, recently, for this person". Whether
// that person may do the thing they are asking for is the handler's to work
// out, and on a worker the answer is written on the container: a terminal is
// opened for whoever the container belongs to. A token is therefore worth
// exactly what its holder already owns, and only until it expires.
type Token struct {
	next http.Handler
	j    *jwt.JWT

	// optional lets a request through with nobody attached to it, for a
	// handler that has something to offer an anonymous caller. A token that is
	// there is still verified: presenting a bad one is not the same as
	// presenting none, and is refused.
	optional bool
}

var _ http.Handler = &Token{}

// NewTokenMiddleware refuses anything that does not carry a token this estate
// signed.
func NewTokenMiddleware(next http.Handler, j *jwt.JWT) *Token {
	return &Token{next: next, j: j}
}

// NewOptionalTokenMiddleware says who a request is from when it says, and lets
// it through as nobody when it does not.
//
// It is for what an anonymous caller may reach: a container with no owner is a
// snippet, which belongs to nobody and is therefore open to everybody. The
// handler decides that, because the handler is what knows whose the container
// is; this only establishes whether anyone is asking.
func NewOptionalTokenMiddleware(next http.Handler, j *jwt.JWT) *Token {
	return &Token{next: next, j: j, optional: true}
}

func (t *Token) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)

	// nobody is asking, which is an answer where one is allowed
	if t.optional && len(token) == 0 {
		t.next.ServeHTTP(rw, r)

		return
	}

	claims, err := t.j.Verify(r.Context(), token)
	if err != nil {
		rw.WriteHeader(http.StatusUnauthorized)

		return
	}

	// an access token and not one of the others: a refresh token is not
	// permission to do anything, only to ask for permission again.
	if audiences, err := claims.GetAudience(); err != nil || len(audiences) == 0 || audiences[0] != auth.AccessToken {
		rw.WriteHeader(http.StatusUnauthorized)

		return
	}

	subject, err := claims.GetSubject()
	if err != nil || len(subject) == 0 {
		rw.WriteHeader(http.StatusUnauthorized)

		return
	}

	t.next.ServeHTTP(rw, r.WithContext(WithSubject(r.Context(), subject)))
}

// WithSubject carries who a verified token was for.
func WithSubject(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, subjectKey{}, subject)
}

// Subject is who the verified token was for, and empty when nothing verified
// one.
func Subject(ctx context.Context) string {
	subject, _ := ctx.Value(subjectKey{}).(string)

	return subject
}
