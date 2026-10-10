//go:build !microsandbox

package microsandbox

import (
	"context"
	"errors"
)

// ErrNotBuilt is what New answers in a build without the engine. The vmhost
// image is the one build that has it.
var ErrNotBuilt = errors.New("built without microsandbox: build with -tags microsandbox and cgo")

// New refuses: this build has no engine.
func New(ctx context.Context, options Options) (Engine, error) {
	return nil, ErrNotBuilt
}
