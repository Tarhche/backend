package console

import (
	"context"
	"testing"
)

func TestConsole(t *testing.T) {
	console := NewConsole()

	ctx := context.Background()
	exitCode := console.Run(ctx)

	if exitCode != 0 {
		t.Error("unexpected exit code")
	}
}
