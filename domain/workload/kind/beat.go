package kind

import (
	"context"
	"time"
)

// beatKey is where a context carries the heartbeat it is asked for.
type beatKey struct{}

// WithBeat is ctx asking for a node's heartbeat taken at a moment: what every
// kind's State says is said as of then. Kinds that read the same things, the
// building blocks of a Docker VM say, may share one read among them, made
// since the beat began, and none made before it, which would say less than
// the heartbeat claims to.
func WithBeat(ctx context.Context, at time.Time) context.Context {
	return context.WithValue(ctx, beatKey{}, at)
}

// BeatOf is when the heartbeat ctx asks for was taken, and whether it asks
// for one: a query for one resource's state does not.
func BeatOf(ctx context.Context) (time.Time, bool) {
	at, beating := ctx.Value(beatKey{}).(time.Time)

	return at, beating
}
