package console

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestConsole(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		t.Run("console help without registered command", func(t *testing.T) {
			testCases := []struct {
				name               string
				arguments          []string
				registeredCommands []Command
				exitStatus         int
				outputErr          string
			}{
				{
					name:      "-h flag",
					arguments: []string{"", "-h"},
				},
				{
					name:      "-help",
					arguments: []string{"", "-h"},
				},
			}

			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					var errWriter bytes.Buffer
					console := NewConsole("Test", "Test description", &errWriter)

					if exitStatus := console.Run(context.Background(), testCase.arguments); exitStatus != 0 {
						t.Errorf("unexpected exit code, want %d got %d", 0, exitStatus)
					}

					want := testdata(t, "help-without-commands.txt")
					got := errWriter.String()
					if diff := cmp.Diff(want, got); diff != "" {
						t.Errorf("console error output mismatch (-want +got):\n%s", diff)
					}
				})
			}
		})

		t.Run("console help with registered command", func(t *testing.T) {
			testCases := []struct {
				name               string
				arguments          []string
				registeredCommands []Command
				exitStatus         int
				outputErr          string
			}{
				{
					name:      "-h flag",
					arguments: []string{"", "-h"},
				},
				{
					name:      "-help",
					arguments: []string{"", "-h"},
				},
			}

			for _, testCase := range testCases {
				t.Run(testCase.name, func(t *testing.T) {
					var errWriter bytes.Buffer
					console := NewConsole("Test", "Test description", &errWriter)

					var (
						boolArg bool
						intArg  int
					)

					command := NewSpyCommand(
						"test-command",
						"this is a test description",
						"this is a test usage",
						0,
						func(fs *flag.FlagSet) {
							fs.BoolVar(&boolArg, "boolArg", false, "test bool argument")
							fs.IntVar(&intArg, "intArg", 666, "test int argument")
						},
					)

					console.Register(command)
					if exitStatus := console.Run(context.Background(), testCase.arguments); exitStatus != 0 {
						t.Errorf("unexpected exit code, want %d got %d", 0, exitStatus)
					}

					want := testdata(t, "help-with-commands.txt")
					got := errWriter.String()
					if diff := cmp.Diff(want, got); diff != "" {
						t.Errorf("console error output mismatch (-want +got):\n%s", diff)
					}
				})
			}
		})
	})

	t.Run("invalid attempts", func(t *testing.T) {
		testCases := []struct {
			name               string
			arguments          []string
			registeredCommands []Command
			exitStatus         int
			outputErr          string
		}{
			{
				name:       "no arguments",
				arguments:  []string{},
				exitStatus: 2,
				outputErr:  testdata(t, "help-without-commands.txt"),
			},
			{
				name:       "0-either only binary or command",
				arguments:  []string{""},
				exitStatus: 2,
				outputErr:  testdata(t, "help-without-commands.txt"),
			},
			{
				name:       "1-either only binary or command",
				arguments:  []string{"command"},
				exitStatus: 2,
				outputErr:  testdata(t, "help-without-commands.txt"),
			},
			{
				name:       "not registered command (help)",
				arguments:  []string{"", "help"},
				exitStatus: 2,
				outputErr:  "\"help\" is not a command, See \"Test -help\".\n",
			},
			{
				name:       "not registered command with -h flag",
				arguments:  []string{"", "command", "-h"},
				exitStatus: 2,
				outputErr:  "\"command\" is not a command, See \"Test -help\".\n",
			},
			{
				name:       "not registered command with -help flag",
				arguments:  []string{"binary", "command", "-help"},
				exitStatus: 2,
				outputErr:  "\"command\" is not a command, See \"Test -help\".\n",
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				var errWriter bytes.Buffer
				console := NewConsole("Test", "Test description", &errWriter)

				if exitStatus := console.Run(context.Background(), testCase.arguments); exitStatus != testCase.exitStatus {
					t.Errorf("unexpected exit code, want %d got %d", testCase.exitStatus, exitStatus)
				}

				want := testCase.outputErr
				got := errWriter.String()
				if diff := cmp.Diff(want, got); diff != "" {
					t.Errorf("console error output mismatch (-want +got):\n%s", diff)
				}
			})
		}
	})

	t.Run("exit status will be returned to caller", func(t *testing.T) {
		statuses := []int{0, 1, 2, 3, 4}

		for _, status := range statuses {
			var errWriter bytes.Buffer
			console := NewConsole("Test", "Test description", &errWriter)

			command := NewSpyCommand(
				"command",
				"this is a test description",
				"this is a test usage",
				status,
				nil,
			)

			console.Register(command)

			if exitStatus := console.Run(context.Background(), []string{"binary-name", "command"}); exitStatus != status {
				t.Errorf("unexpected exit code, want %d got %d", status, exitStatus)
			}

			want := ""
			got := errWriter.String()
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("console error output mismatch (-want +got):\n%s", diff)
			}

			if command.NameCount != 1 {
				t.Errorf("%q method should be called once", "Name")
			}

			if command.DescriptionCount != 0 {
				t.Errorf("%q method should not be called", "Description")
			}

			if command.UsageCount != 0 {
				t.Errorf("%q method should not be called", "Usage")
			}

			if command.RunCount != 1 {
				t.Errorf("%q method should be called once", "Run")
			}

			if command.ConfigureCount != 1 {
				t.Errorf("%q method should be called once", "Configure")
			}
		}
	})

	t.Run("test command arguments", func(t *testing.T) {
		var errWriter bytes.Buffer
		console := NewConsole("Test", "Test description", &errWriter)

		var (
			boolArg bool
			intArg  int
		)

		command := NewSpyCommand(
			"test-command",
			"this is a test description",
			"this is a test usage",
			0,
			func(fs *flag.FlagSet) {
				fs.BoolVar(&boolArg, "boolArg", false, "test bool argument")
				fs.IntVar(&intArg, "intArg", 666, "test int argument")
			},
		)

		console.Register(command)

		t.Run("args should be filled with provided values", func(t *testing.T) {
			errWriter.Reset()
			arguments := []string{"binary-name", "test-command", "-intArg", "100", "-boolArg", "true"}
			if exitStatus := console.Run(context.Background(), arguments); exitStatus != 0 {
				t.Errorf("unexpected exit code, want %d got %d", 0, exitStatus)
			}

			if command.RunCount != 1 {
				t.Errorf("%q method should be called once", "Run")
			}

			if command.ConfigureCount != 1 {
				t.Errorf("%q method should be called once", "Configure")
			}

			want := ""
			got := errWriter.String()
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("console error output mismatch (-want +got):\n%s", diff)
			}

			if boolArg != true {
				t.Errorf("unexpected argument, want true got false")
			}

			if intArg != 100 {
				t.Errorf("unexpected argument, want %d got %d", 100, intArg)
			}
		})

		t.Run("flag type mismatch", func(t *testing.T) {
			errWriter.Reset()
			arguments := []string{"binary-name", "test-command", "-intArg", "100.2", "-boolArg", "true"}
			if exitStatus := console.Run(context.Background(), arguments); exitStatus != 2 {
				t.Errorf("unexpected exit code, want %d got %d", 0, exitStatus)
			}

			want := testdata(t, "flag-type-mismatch.txt")
			got := errWriter.String()
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("console error output mismatch (-want +got):\n%s", diff)
			}

			if boolArg != false {
				t.Errorf("unexpected argument, want false got true")
			}

			if intArg != 666 {
				t.Errorf("unexpected argument, want %d got %d", 666, intArg)
			}
		})

		t.Run("non existing arg", func(t *testing.T) {
			errWriter.Reset()
			arguments := []string{"binary-name", "test-command", "-nonexisting", "abc", "-intArg", "100"}
			if exitStatus := console.Run(context.Background(), arguments); exitStatus != ExitUsageError {
				t.Errorf("unexpected exit code, want %d got %d", ExitUsageError, exitStatus)
			}

			want := testdata(t, "flag-provided-but-not-defined.txt")
			got := errWriter.String()
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("console error output mismatch (-want +got):\n%s", diff)
			}

			if boolArg != false {
				t.Errorf("unexpected argument, want false got true")
			}

			if intArg != 666 {
				t.Errorf("unexpected argument, want %d got %d", 666, intArg)
			}
		})
	})
}

type SpyCommand struct {
	name          string
	description   string
	usage         string
	exitStatus    int
	configureFunc func(*flag.FlagSet)

	NameCount        int
	DescriptionCount int
	UsageCount       int
	RunCount         int
	ConfigureCount   int
}

func NewSpyCommand(
	name, description,
	usage string,
	exitStatus int,
	configureFunc func(*flag.FlagSet),
) *SpyCommand {
	return &SpyCommand{
		name:          name,
		description:   description,
		usage:         usage,
		exitStatus:    exitStatus,
		configureFunc: configureFunc,
	}
}

func (c *SpyCommand) Name() string {
	c.NameCount++
	return c.name
}

func (c *SpyCommand) Description() string {
	c.DescriptionCount++
	return c.description
}

func (c *SpyCommand) Usage() string {
	c.UsageCount++
	return c.usage
}

func (c *SpyCommand) Configure(flagSet *flag.FlagSet) {
	c.ConfigureCount++

	if c.configureFunc != nil {
		c.configureFunc(flagSet)
	}
}

func (c *SpyCommand) Run(ctx context.Context) ExitStatus {
	c.RunCount++

	return c.exitStatus
}

func testdata(t *testing.T, filename string) string {
	t.Helper()

	b, err := os.ReadFile(fmt.Sprintf("testdata/%s", filename))
	if err != nil {
		t.Errorf("unexpected error: %s", err)
	}

	return string(b)
}
