// Package grant holds what somebody gave an application, in the moment between
// approving it and the application collecting the session it was given.
//
// A grant is redeemed once. It is destroyed as it is handed over, so a code
// that arrives twice — because it was copied out of a browser's history, a
// referrer or a log — is exchanged at most once, and the second attempt finds
// nothing.
package grant

import (
	"context"
	"time"

	"github.com/khanzadimahdi/testproject/domain/password"
)

// Lifetime is how long a code is worth anything. It only has to survive the
// redirect that carries it back to the application that asked.
const Lifetime = 2 * time.Minute

// ChallengeMethodS256 is the only way a code may be tied to the application
// that asked for it: the plain method is no protection at all.
const ChallengeMethodS256 = "S256"

type Grant struct {
	ID string

	// Secret is the other half of the code the application was handed. The code
	// is "<id>.<secret>": the id says which grant to look at, and the secret is
	// what proves the holder is the one it was handed to.
	Secret password.Hash

	ClientID string
	UserUUID string

	RedirectURI string
	Scope       string

	// CodeChallenge is the proof-key the application will have to answer for,
	// which is what stops a code that was intercepted from being of any use to
	// whoever intercepted it.
	CodeChallenge       string
	CodeChallengeMethod string

	ExpiredAt time.Time
	CreatedAt time.Time
}

// IsExpired reports a grant that waited too long to be collected.
func (g Grant) IsExpired() bool {
	return !time.Now().Before(g.ExpiredAt)
}

type Repository interface {
	Save(ctx context.Context, g *Grant) (id string, err error)

	// Consume hands back a grant and destroys it in the same breath, so two
	// callers presenting the same code cannot both be answered.
	Consume(ctx context.Context, id string) (Grant, error)
}
