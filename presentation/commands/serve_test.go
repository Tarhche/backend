package commands

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"testing"
)

func TestServe(t *testing.T) {
	t.Run("name", func(t *testing.T) {
		command := NewServeCommand(nil)

		want := "serve"
		got := command.Name()

		if want != got {
			t.Errorf("want command name %q got %q", want, got)
		}
	})

	t.Run("description", func(t *testing.T) {
		command := NewServeCommand(nil)

		want := "serves a http server."
		got := command.Description()

		if want != got {
			t.Errorf("want command description %q got %q", want, got)
		}
	})

	t.Run("usage", func(t *testing.T) {
		command := NewServeCommand(nil)

		want := "serve [arguments]"
		got := command.Usage()

		if want != got {
			t.Errorf("want command usage %q got %q", want, got)
		}
	})

	t.Run("configure", func(t *testing.T) {
		command := NewServeCommand(nil)

		flagSet := flag.NewFlagSet(command.Name(), flag.ContinueOnError)

		command.Configure(flagSet)

		port := flagSet.Lookup("port")
		if port == nil {
			t.Fatal("port flag has not been configured")
		}

		if port.Usage != "specifies which port server should listen to." {
			t.Error("unexpected port flag usage")
		}

		if command.port != 80 {
			t.Error("unexpected port flag default value")
		}

		if err := flagSet.Parse([]string{"-port", "100"}); err != nil {
			t.Errorf("unexpected parsing error: %q", err)
		}

		if command.port != 100 {
			t.Error("unexpected port flag default value")
		}
	})

	t.Run("run", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.WriteHeader(http.StatusOK)
			fmt.Fprint(rw, "test response")
		})

		command := NewServeCommand(handler)

		_ = ctx
		_ = command
		// if exitStatus := command.Run(ctx); exitStatus != console.ExitSuccess {
		// 	t.Error("unexpected exit code")
		// }
	})
}
