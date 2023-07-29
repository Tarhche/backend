package commands

import (
	"context"
	"testing"

	"github.com/khanzadimahdi/testproject.git/infrastructure/console"
)

func TestServe(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		command := NewServeCommand()

		want := "serve"
		got := command.Name()

		if want != got {
			t.Errorf("want command name %q got %q", want, got)
		}
	})

	t.Run("run", func(t *testing.T) {
		command := NewServeCommand()
		ctx := context.Background()

		if exitStatus := command.Run(ctx); exitStatus != console.ExitSuccess {
			t.Error("unexpected exit code")
		}
	})
}
