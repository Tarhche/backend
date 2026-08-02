package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

const (
	authenticationHeaderName   = "authorization"
	authenticationHeaderPrefix = "bearer "
)

type Authenticate struct {
	next           http.Handler
	j              *jwt.JWT
	userRepository user.Repository
	translator     translator.Translator
}

var _ http.Handler = &Authenticate{}

func NewAuthenticateMiddleware(next http.Handler, j *jwt.JWT, userRepository user.Repository, translator translator.Translator) *Authenticate {
	return &Authenticate{
		j:              j,
		userRepository: userRepository,
		translator:     translator,
		next:           next,
	}
}

func (a *Authenticate) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	token := a.bearerToken(r)
	claims, err := a.j.Verify(r.Context(), token)
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

	user, err := a.userRepository.GetOne(r.Context(), userUUID)
	if err != nil {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	// A ban takes effect immediately, including for tokens handed out before it,
	// so every authenticated request is checked. 403 rather than 401: the
	// credentials are fine, the account is not, and retrying with a fresh token
	// would change nothing.
	if user.IsBanned() {
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusForbidden)
		json.NewEncoder(rw).Encode(auth.NewBannedResponse(a.bannedMessage(r, &user)))

		return
	}

	a.next.ServeHTTP(rw, r.WithContext(auth.ToContext(r.Context(), &user)))
}

// bannedMessage translates the refusal the same way the Localize middleware
// would, except that it runs before it: an explicitly requested language wins,
// otherwise the user's own, otherwise the translator's default.
func (a *Authenticate) bannedMessage(r *http.Request, u *user.User) string {
	locale := r.Header.Get(languageCodeHeader)
	if len(locale) == 0 {
		locale = u.LanguageCode
	}

	if len(locale) == 0 {
		return a.translator.Translate(auth.BannedTranslationKey)
	}

	return a.translator.Translate(auth.BannedTranslationKey, translator.WithLocale(locale))
}

func (a *Authenticate) bearerToken(r *http.Request) string {
	offset := len(authenticationHeaderPrefix)
	h := r.Header.Get(authenticationHeaderName)
	if len(h) <= offset {
		return ""
	}

	return (" " + h[offset:])[1:]
}
