package console

import (
	"context"
	"testing"
)

func TestConsole(t *testing.T) {
	t.Run("run with no registered command", func(t *testing.T) {
		console := NewConsole()

		ctx := context.Background()
		exitCode := console.Run(ctx)

		if exitCode != 0 {
			t.Error("unexpected exit code")
		}
	})

	t.Run("register a command", func(t *testing.T) {
		console := NewConsole()
		ctx := context.Background()
		cmd := &MockCommand{}

		console.Register(cmd)

		if exitCode := console.Run(ctx); exitCode != 0 {
			t.Error("unexpected exit code")
		}

		if cmd.count != 1 {
			t.Error("command not invoked")
		}

		cmd.count = 0
		cmd.exitCode = 1
		if exitCode := console.Run(ctx); exitCode != 1 {
			t.Error("unexpected exit code")
		}

		if cmd.count != 1 {
			t.Error("command not invoked")
		}
	})
}

type MockCommand struct {
	exitCode int
	count    int
}

func (c *MockCommand) Run(ctx context.Context) int {
	c.count++

	return c.exitCode
}
