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

	t.Run("register commands", func(t *testing.T) {
		console := NewConsole()
		ctx := context.Background()

		commands := []Command{
			&MockCommand{},
			&MockCommand{},
			&MockCommand{},
			&MockCommand{},
			&MockCommand{},
		}

		for i := range commands {
			console.Register(commands[i])
		}

		if exitCode := console.Run(ctx); exitCode != 0 {
			t.Error("unexpected exit code")
		}

		for i := range commands {
			if commands[i].(*MockCommand).count != 1 {
				t.Errorf("command at index %d not invoked", i)
			}
		}
	})

	t.Run("non zero status code", func(t *testing.T) {
		console := NewConsole()
		ctx := context.Background()

		commands := []Command{
			&MockCommand{},
			&MockCommand{
				exitCode: 1,
			},
			&MockCommand{},
		}

		for i := range commands {
			console.Register(commands[i])
		}

		if exitCode := console.Run(ctx); exitCode != 1 {
			t.Error("unexpected exit code")
		}

		if commands[0].(*MockCommand).count != 1 {
			t.Error("command at index 0 not invoked")
		}

		if commands[1].(*MockCommand).count != 1 {
			t.Error("command at index 1 not invoked")
		}

		if commands[2].(*MockCommand).count != 0 {
			t.Errorf("command at index 2 should not be invoked")
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
