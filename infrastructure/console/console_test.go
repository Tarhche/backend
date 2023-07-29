package console

import (
	"context"
	"log"
	"testing"
)

func TestConsole(t *testing.T) {
	t.Run("run with no registered command", func(t *testing.T) {
		console := NewConsole()

		ctx := context.Background()
		exitCode := console.Run(ctx, []string{""})

		if exitCode != 0 {
			t.Error("unexpected exit code")
		}
	})

	t.Run("register commands", func(t *testing.T) {
		console := NewConsole()
		ctx := context.Background()

		commands := []Command{
			&MockCommand{
				name: "test-1",
			},
			&MockCommand{},
			&MockCommand{
				name: "test-1",
			},
			&MockCommand{
				name: "test-2",
			},
			&MockCommand{},
		}

		for i := range commands {
			console.Register(commands[i])
		}

		const cmdName string = "test-1"
		arguments := []string{"test-1"}
		if exitCode := console.Run(ctx, arguments); exitCode != 0 {
			t.Errorf("unexpected exit code %d", exitCode)
		}

		for i := range commands {
			cmd := commands[i].(*MockCommand)
			log.Println("in test", i, cmd.Name(), cmdName, cmd.Name() == cmdName, cmd.count != 1)

			if cmd.Name() == cmdName && cmd.count != 1 {
				t.Errorf("command at index %d should be invoked exactly 1 but invoked %d", i, cmd.count)
			}

			if cmd.Name() != cmdName && cmd.count > 0 {
				t.Errorf("command at index %d should not be invoked but invoked %d", i, cmd.count)
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

		if exitCode := console.Run(ctx, []string{}); exitCode != 2 {
			t.Error("unexpected exit code")
		}

		if exitCode := console.Run(ctx, []string{""}); exitCode != 1 {
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
	name     string
	exitCode int
	count    int
}

func (c *MockCommand) Name() string {
	return c.name
}

func (c *MockCommand) Run(ctx context.Context) int {
	c.count++

	return c.exitCode
}
