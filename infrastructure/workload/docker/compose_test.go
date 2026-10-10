package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

const composeYAML = "services:\n  web:\n    image: nginx:alpine\n"

// shop is the stack these tests run compose on, in vm-1.
func shop(compose string) stack.Stack {
	return stack.Stack{
		Kind: stack.Name,
		Metadata: kind.Metadata{
			UUID:   "stack-uuid",
			Slug:   "shop-abcde",
			Owners: []kind.Reference{{Kind: stack.Parent, UUID: "vm-1"}},
		},
		Spec: stack.Spec{VM: stack.VMChoice{UUID: "vm-1"}, Compose: compose},
	}
}

// composeRun is what one compose command inside the VM was given.
type composeRun struct {
	command []string
	stdin   string
}

// fakeCompose is docker compose inside a VM: it reads the file from its input,
// and says what it is told to on its two streams.
type fakeCompose struct {
	lock sync.Mutex
	runs []composeRun

	stdout   string
	stderr   string
	exitCode int
}

func (f *fakeCompose) exec(ctx context.Context, _ string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	read, _ := io.ReadAll(stdin)

	f.lock.Lock()
	f.runs = append(f.runs, composeRun{command: options.Command, stdin: string(read)})
	f.lock.Unlock()

	_, _ = io.WriteString(stdout, f.stdout)
	_, _ = io.WriteString(stderr, f.stderr)

	return f.exitCode
}

func composeIn(t *testing.T, fake *fakeCompose) *Compose {
	t.Helper()

	e := memory.New(memory.WithExec(fake.exec))

	_, err := e.Create(t.Context(), vm.Spec{ID: "vm-1", Image: "docker:29-dind"})
	require.NoError(t, err)

	return NewCompose(e)
}

func TestCompose(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name string
		run  func(t *testing.T, c *Compose) (string, error)
		want []string
	}{
		{
			name: "up brings the project up in the background and removes what it no longer has",
			run: func(t *testing.T, c *Compose) (string, error) {
				return c.Up(t.Context(), shop(composeYAML))
			},
			want: []string{"docker", "compose", "-p", "shop-abcde", "-f", "-", "up", "-d", "--remove-orphans"},
		},
		{
			name: "start",
			run: func(t *testing.T, c *Compose) (string, error) {
				return c.Start(t.Context(), shop(composeYAML))
			},
			want: []string{"docker", "compose", "-p", "shop-abcde", "-f", "-", "start"},
		},
		{
			name: "stop",
			run: func(t *testing.T, c *Compose) (string, error) {
				return c.Stop(t.Context(), shop(composeYAML))
			},
			want: []string{"docker", "compose", "-p", "shop-abcde", "-f", "-", "stop"},
		},
		{
			name: "restart",
			run: func(t *testing.T, c *Compose) (string, error) {
				return c.Restart(t.Context(), shop(composeYAML))
			},
			want: []string{"docker", "compose", "-p", "shop-abcde", "-f", "-", "restart"},
		},
		{
			name: "down keeps the project's volumes unless asked not to",
			run: func(t *testing.T, c *Compose) (string, error) {
				return c.Down(t.Context(), shop(composeYAML), false)
			},
			want: []string{"docker", "compose", "-p", "shop-abcde", "-f", "-", "down", "--remove-orphans"},
		},
		{
			name: "down takes the volumes with it when asked to",
			run: func(t *testing.T, c *Compose) (string, error) {
				return c.Down(t.Context(), shop(composeYAML), true)
			},
			want: []string{"docker", "compose", "-p", "shop-abcde", "-f", "-", "down", "--remove-orphans", "--volumes"},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := &fakeCompose{stdout: "Container shop-abcde-web-1  Started\n"}

			output, err := tt.run(t, composeIn(t, fake))
			require.NoError(t, err)

			assert.Equal(t, "Container shop-abcde-web-1  Started\n", output)

			labelled, err := Labelled(composeYAML, "stack-uuid")
			require.NoError(t, err)

			require.Len(t, fake.runs, 1)
			assert.Equal(t, tt.want, fake.runs[0].command)
			assert.Equal(t, labelled, fake.runs[0].stdin, "the YAML, labelled as the stack's, is the file compose reads, on its input")
		})
	}

	t.Run("what compose says on either stream is kept", func(t *testing.T) {
		t.Parallel()

		fake := &fakeCompose{stderr: " Network shop-abcde_default  Created\n"}

		output, err := composeIn(t, fake).Up(t.Context(), shop(composeYAML))
		require.NoError(t, err)
		assert.Equal(t, " Network shop-abcde_default  Created\n", output)
	})

	t.Run("only the last 16 KiB of what compose says is kept", func(t *testing.T) {
		t.Parallel()

		var said strings.Builder
		for n := range 2000 {
			fmt.Fprintf(&said, "line %05d of what compose said\n", n)
		}

		fake := &fakeCompose{stderr: said.String()}

		output, err := composeIn(t, fake).Up(t.Context(), shop(composeYAML))
		require.NoError(t, err)

		assert.Len(t, output, kind.MaxOutput)
		assert.True(t, strings.HasSuffix(output, "line 01999 of what compose said\n"), "the end is what says how it went")
	})

	t.Run("compose failing is an error that keeps what it said", func(t *testing.T) {
		t.Parallel()

		fake := &fakeCompose{stderr: "service \"web\" refers to undefined network backend: invalid compose project\n", exitCode: 15}

		output, err := composeIn(t, fake).Up(t.Context(), shop(composeYAML))
		assert.ErrorContains(t, err, "docker compose up exited with 15")
		assert.Contains(t, output, "undefined network backend")
	})

	t.Run("a file that is not a compose file is not handed to compose", func(t *testing.T) {
		t.Parallel()

		fake := &fakeCompose{}

		_, err := composeIn(t, fake).Up(t.Context(), shop("services: ["))
		assert.ErrorContains(t, err, "its compose file cannot be read")
		assert.Empty(t, fake.runs)
	})

	t.Run("a VM that is not running runs no compose", func(t *testing.T) {
		t.Parallel()

		e := memory.New(memory.WithExec((&fakeCompose{}).exec))

		_, err := e.Create(t.Context(), vm.Spec{ID: "vm-1", Image: "docker:29-dind"})
		require.NoError(t, err)
		require.NoError(t, e.Stop(t.Context(), "vm-1"))

		_, err = NewCompose(e).Up(t.Context(), shop(composeYAML))
		assert.ErrorIs(t, err, vm.ErrNotRunning)
	})

	t.Run("giving up ends compose", func(t *testing.T) {
		t.Parallel()

		ended := make(chan struct{})
		e := memory.New(memory.WithExec(func(ctx context.Context, _ string, _ vm.ExecOptions, stdin io.Reader, _ io.Writer, _ io.Writer) int {
			_, _ = io.ReadAll(stdin)
			<-ctx.Done()
			close(ended)

			return -1
		}))

		_, err := e.Create(t.Context(), vm.Spec{ID: "vm-1", Image: "docker:29-dind"})
		require.NoError(t, err)

		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()

		_, err = NewCompose(e).Up(ctx, shop(composeYAML))
		assert.Error(t, err)

		select {
		case <-ended:
		case <-time.After(5 * time.Second):
			t.Fatal("compose was left running")
		}
	})
}
