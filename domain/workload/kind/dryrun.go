package kind

import "context"

// dryRunKey is what marks the context of a dry run.
type dryRunKey struct{}

// DryRun is ctx as that of an admission asked as a dry run: what it would
// admit is answered, and nothing is kept, sent or taken for it. A strategy's
// Admit holds nothing anyway, a slug or a node it gives being held only by
// the record that keeps it; what it does beside, it does not do in a dry run:
// what it would make for what it admits is admitted as a dry run too, the
// Docker VM made for a stack, and what it would pick in turn, a node for a
// task, is not picked, so that nobody's turn is taken.
func DryRun(ctx context.Context) context.Context {
	return context.WithValue(ctx, dryRunKey{}, true)
}

// IsDryRun reports whether ctx is that of an admission asked as a dry run
// (DryRun).
func IsDryRun(ctx context.Context) bool {
	dry, _ := ctx.Value(dryRunKey{}).(bool)

	return dry
}
