package console

import (
	"context"
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

	t.Run("run with no arguments", func(t *testing.T) {
		console := NewConsole()
		ctx := context.Background()

		if exitCode := console.Run(ctx, []string{}); exitCode != 2 {
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

			if cmd.Name() == cmdName && cmd.runCount != 1 {
				t.Errorf("command at index %d should be invoked exactly 1 but invoked %d", i, cmd.runCount)
			}

			if cmd.Name() != cmdName && cmd.runCount > 0 {
				t.Errorf("command at index %d should not be invoked but invoked %d", i, cmd.runCount)
			}
		}
	})

	t.Run("will stop if a command returns non zero code", func(t *testing.T) {
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

		if exitCode := console.Run(ctx, []string{""}); exitCode != 1 {
			t.Error("unexpected exit code")
		}

		if commands[0].(*MockCommand).runCount != 1 {
			t.Error("command at index 0 not invoked")
		}

		if commands[1].(*MockCommand).runCount != 1 {
			t.Error("command at index 1 not invoked")
		}

		if commands[2].(*MockCommand).runCount != 0 {
			t.Errorf("command at index 2 should not be invoked")
		}
	})

	t.Run("configure", func(t *testing.T) {
		console := NewConsole()
		ctx := context.Background()

		commands := []Command{
			&MockCommand{},
			&MockCommand{},
		}

		for i := range commands {
			console.Register(commands[i])
		}

		if exitCode := console.Run(ctx, []string{""}); exitCode != 0 {
			t.Errorf("unexpected exit code %d", exitCode)
		}

		for i := range commands {
			cmd := commands[i].(*MockCommand)
			if cmd.configureCount != 1 {
				t.Errorf("command at index %d configure method should be called 1 but is called %d", i, cmd.configureCount)
			}
		}
	})
}

type MockCommand struct {
	name           string
	exitCode       int
	runCount       int
	configureCount int
}

func (c *MockCommand) Name() string {
	return c.name
}

func (c *MockCommand) Configure() {
	c.configureCount++
}

func (c *MockCommand) Run(ctx context.Context) int {
	c.runCount++

	return c.exitCode
}
