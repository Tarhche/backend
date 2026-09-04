// Package access answers what somebody may do to one particular thing.
//
// Two permissions cover the same action: one over everybody's things, and one
// over one's own. Holding the first is enough on its own; holding the second is
// enough only for what belongs to you. A route admits either, so this is where
// the difference between them is actually made.
package access

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
)

type Guard struct {
	authorizer domain.Authorizer
}

func NewGuard(authorizer domain.Authorizer) *Guard {
	return &Guard{authorizer: authorizer}
}

// May reports whether the person asking may act on something owned by
// ownerUUID.
//
// A thing nobody owns — a container the code runner started, say — is nobody's
// own, so only the permission over everybody's reaches it.
func (g *Guard) May(ctx context.Context, userUUID string, all string, own string, ownerUUID string) (bool, error) {
	allowed, err := g.authorizer.Authorize(ctx, userUUID, all)
	if err != nil || allowed {
		return allowed, err
	}

	if len(userUUID) == 0 || userUUID != ownerUUID {
		return false, nil
	}

	return g.authorizer.Authorize(ctx, userUUID, own)
}
