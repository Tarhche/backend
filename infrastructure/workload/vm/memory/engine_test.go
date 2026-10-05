package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// machine is a spec for an instance of the machine kind, reachable on its
// ports.
func machine(id string) vm.Spec {
	return vm.Spec{
		ID:        id,
		Kind:      vm.KindMachine,
		Image:     "ubuntu:24.04",
		Resources: vm.Resources{CPUs: 1, Memory: 256 << 20, Disk: 1 << 30},
		Ports:     []port.Port{8080, 80},
		Network:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		Labels:    map[string]string{vm.LabelPurpose: vm.PurposeVM},
	}
}

func TestEngine_Lifecycle(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name string

		// act does something to an engine holding "vm-1", running.
		act func(t *testing.T, e *Engine) error

		wantErr   error
		wantState vm.InstanceState
		gone      bool
	}{
		{
			name: "a created instance is running",
			act:  func(*testing.T, *Engine) error { return nil },

			wantState: vm.InstanceRunning,
		},
		{
			name: "a stopped instance keeps its disk and says it is stopped",
			act: func(t *testing.T, e *Engine) error {
				return e.Stop(t.Context(), "vm-1")
			},

			wantState: vm.InstanceStopped,
		},
		{
			name: "a stopped instance starts again",
			act: func(t *testing.T, e *Engine) error {
				require.NoError(t, e.Stop(t.Context(), "vm-1"))

				return e.Start(t.Context(), "vm-1")
			},

			wantState: vm.InstanceRunning,
		},
		{
			name: "starting a running instance is what was asked for already",
			act: func(t *testing.T, e *Engine) error {
				return e.Start(t.Context(), "vm-1")
			},

			wantState: vm.InstanceRunning,
		},
		{
			name: "a restarted instance is running",
			act: func(t *testing.T, e *Engine) error {
				return e.Restart(t.Context(), "vm-1")
			},

			wantState: vm.InstanceRunning,
		},
		{
			name: "creating one that exists is refused",
			act: func(t *testing.T, e *Engine) error {
				_, err := e.Create(t.Context(), machine("vm-1"))

				return err
			},

			wantErr:   domain.ErrAlreadyExists,
			wantState: vm.InstanceRunning,
		},
		{
			name: "a deleted instance is not there",
			act: func(t *testing.T, e *Engine) error {
				return e.Delete(t.Context(), "vm-1")
			},

			gone: true,
		},
		{
			name: "deleting one that is not there is what was asked for",
			act: func(t *testing.T, e *Engine) error {
				require.NoError(t, e.Delete(t.Context(), "vm-1"))

				return e.Delete(t.Context(), "vm-1")
			},

			gone: true,
		},
		{
			name: "starting one that is not there says so",
			act: func(t *testing.T, e *Engine) error {
				return e.Start(t.Context(), "nothing")
			},

			wantErr:   domain.ErrNotExists,
			wantState: vm.InstanceRunning,
		},
		{
			name: "stopping one that is not there says so",
			act: func(t *testing.T, e *Engine) error {
				return e.Stop(t.Context(), "nothing")
			},

			wantErr:   domain.ErrNotExists,
			wantState: vm.InstanceRunning,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := New()

			created, err := e.Create(t.Context(), machine("vm-1"))
			require.NoError(t, err)
			assert.Equal(t, vm.InstanceRunning, created.State)

			err = tt.act(t, e)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			instance, err := e.Inspect(t.Context(), "vm-1")
			if tt.gone {
				assert.ErrorIs(t, err, domain.ErrNotExists)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantState, instance.State)
		})
	}
}

func TestEngine_Capacity(t *testing.T) {
	t.Parallel()

	e := New(WithCapacity(2, 1<<30, 10<<30))

	first := machine("vm-1")
	first.Resources = vm.Resources{CPUs: 2, Memory: 768 << 20, Disk: 4 << 30}

	_, err := e.Create(t.Context(), first)
	require.NoError(t, err)

	second := machine("vm-2")
	second.Resources = vm.Resources{CPUs: 2, Memory: 512 << 20, Disk: 1 << 30}

	_, err = e.Create(t.Context(), second)
	assert.ErrorIs(t, err, vm.ErrNoCapacity, "memory is never given twice")

	second.Resources.Memory = 256 << 20

	_, err = e.Create(t.Context(), second)
	assert.NoError(t, err, "CPUs are shared, so they are not what refuses an instance")

	info, err := e.Info(t.Context())
	require.NoError(t, err)

	assert.Equal(t, vm.Info{
		Engine:    Name,
		Version:   Version,
		CPUs:      2,
		Memory:    1 << 30,
		Disk:      10 << 30,
		Allocated: vm.Resources{CPUs: 4, Memory: 1 << 30, Disk: 5 << 30},
	}, info)

	grown := first
	grown.Resources.Disk = 10 << 30

	_, err = e.Reconfigure(t.Context(), grown)
	assert.ErrorIs(t, err, vm.ErrNoCapacity, "an instance grown past the budget is refused too")
}

func TestEngine_Endpoints(t *testing.T) {
	t.Parallel()

	t.Run("every port is published, lowest first, and keeps its host port", func(t *testing.T) {
		t.Parallel()

		e := New(WithHost("vmhost-01"))

		created, err := e.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)

		assert.Equal(t, []vm.Endpoint{
			{Port: 80, Address: "vmhost-01:20001"},
			{Port: 8080, Address: "vmhost-01:20000"},
		}, created.Endpoints)

		require.NoError(t, e.Restart(t.Context(), "vm-1"))

		restarted, err := e.Inspect(t.Context(), "vm-1")
		require.NoError(t, err)
		assert.Equal(t, created.Endpoints, restarted.Endpoints)
	})

	t.Run("nothing of an instance whose ingress is denied is published", func(t *testing.T) {
		t.Parallel()

		spec := machine("vm-1")
		spec.Network.Ingress = vm.AccessDeny

		created, err := New().Create(t.Context(), spec)
		require.NoError(t, err)

		assert.Empty(t, created.Endpoints)
	})

	t.Run("a reconfigured instance keeps the host ports of the ports it keeps", func(t *testing.T) {
		t.Parallel()

		e := New()

		_, err := e.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)

		changed := machine("vm-1")
		changed.Ports = []port.Port{8080, 9000}

		reconfigured, err := e.Reconfigure(t.Context(), changed)
		require.NoError(t, err)

		assert.Equal(t, []vm.Endpoint{
			{Port: 8080, Address: "vmhost:20000"},
			{Port: 9000, Address: "vmhost:20002"},
		}, reconfigured.Endpoints)
	})
}

func TestEngine_Exec(t *testing.T) {
	t.Parallel()

	// echo says back what it is given, and its arguments on its errors.
	echo := func(ctx context.Context, id string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
		_, _ = io.WriteString(stderr, strings.Join(options.Command, " ")+"\n")
		_, _ = io.Copy(stdout, stdin)

		return 3
	}

	t.Run("a command reads its input and says what it says", func(t *testing.T) {
		t.Parallel()

		e := New(WithExec(echo))

		_, err := e.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)

		session, err := e.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"cat", "-"}})
		require.NoError(t, err)

		errors := make(chan string, 1)
		go func() {
			said, _ := io.ReadAll(session.Stderr())
			errors <- string(said)
		}()

		go func() {
			_, _ = io.WriteString(session.Stdin(), "hello")
			_ = session.Stdin().Close()
		}()

		output, err := io.ReadAll(session.Stdout())
		require.NoError(t, err)
		assert.Equal(t, "hello", string(output))
		assert.Equal(t, "cat -\n", <-errors)

		exitCode, err := session.Wait(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 3, exitCode)
	})

	t.Run("under a TTY everything is on its output", func(t *testing.T) {
		t.Parallel()

		e := New(WithExec(echo))

		_, err := e.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)

		session, err := e.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"sh"}, TTY: true, Rows: 24, Cols: 80})
		require.NoError(t, err)

		require.NoError(t, session.Resize(t.Context(), 50, 132))
		assert.Equal(t, [2]uint{50, 132}, sizeOf(session))

		require.NoError(t, session.Stdin().Close())

		output, err := io.ReadAll(session.Stdout())
		require.NoError(t, err)
		assert.Equal(t, "sh\n", string(output))

		stderr, err := io.ReadAll(session.Stderr())
		require.NoError(t, err)
		assert.Empty(t, stderr)
	})

	t.Run("closing a session ends its command and lets its readers go", func(t *testing.T) {
		t.Parallel()

		ended := make(chan struct{})
		e := New(WithExec(func(ctx context.Context, _ string, _ vm.ExecOptions, _ io.Reader, _ io.Writer, _ io.Writer) int {
			<-ctx.Done()
			close(ended)

			return -1
		}))

		_, err := e.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)

		session, err := e.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"sleep", "infinity"}})
		require.NoError(t, err)

		read := make(chan error, 1)
		go func() {
			_, err := session.Stdout().Read(make([]byte, 1))
			read <- err
		}()

		require.NoError(t, session.Close())

		<-ended
		assert.Error(t, <-read)

		assert.Eventually(t, func() bool { return e.Sessions("vm-1") == 0 }, time.Second, time.Millisecond)
	})

	t.Run("stopping an instance closes the sessions into it", func(t *testing.T) {
		t.Parallel()

		e := New(WithExec(func(ctx context.Context, _ string, _ vm.ExecOptions, _ io.Reader, _ io.Writer, _ io.Writer) int {
			<-ctx.Done()

			return -1
		}))

		_, err := e.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)

		session, err := e.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"sleep", "infinity"}})
		require.NoError(t, err)

		require.NoError(t, e.Stop(t.Context(), "vm-1"))

		_, err = io.ReadAll(session.Stdout())
		assert.Error(t, err)
	})

	t.Run("nothing is exec'd into an instance that is not running", func(t *testing.T) {
		t.Parallel()

		e := New(WithExec(echo))

		_, err := e.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)
		require.NoError(t, e.Stop(t.Context(), "vm-1"))

		_, err = e.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"sh"}})
		assert.ErrorIs(t, err, vm.ErrNotRunning)

		_, err = e.Exec(t.Context(), "nothing", vm.ExecOptions{Command: []string{"sh"}})
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("a command the test did not provide is not found", func(t *testing.T) {
		t.Parallel()

		e := New()

		_, err := e.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)

		session, err := e.Exec(t.Context(), "vm-1", vm.ExecOptions{Command: []string{"docker", "system", "dial-stdio"}})
		require.NoError(t, err)

		exitCode, err := session.Wait(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 127, exitCode)
	})
}

func sizeOf(session vm.ExecSession) [2]uint {
	rows, cols := session.(*Session).Size()

	return [2]uint{rows, cols}
}

func TestEngine_MainProcess(t *testing.T) {
	t.Parallel()

	t.Run("an instance whose main process exits keeps its exit code and its log", func(t *testing.T) {
		t.Parallel()

		e := New(WithMain(func(ctx context.Context, spec vm.Spec, log func(string)) int {
			log(strings.Join(spec.Command, " "))

			return 7
		}))

		spec := machine("task-1")
		spec.Command = []string{"python", "-c", "exit(7)"}

		_, err := e.Create(t.Context(), spec)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			instance, err := e.Inspect(t.Context(), "task-1")

			return err == nil && instance.State == vm.InstanceExited
		}, time.Second, time.Millisecond)

		instance, err := e.Inspect(t.Context(), "task-1")
		require.NoError(t, err)
		assert.Equal(t, 7, instance.ExitCode)

		lines, err := e.Logs(t.Context(), "task-1", vm.LogOptions{})
		require.NoError(t, err)
		require.Len(t, lines, 1)
		assert.Equal(t, vm.LogSourceMain, lines[0].Source)
		assert.Equal(t, "python -c exit(7)", lines[0].Line)
	})

	t.Run("a main process runs until it is said to have exited", func(t *testing.T) {
		t.Parallel()

		e := New()

		spec := machine("task-1")
		spec.Command = []string{"serve"}

		_, err := e.Create(t.Context(), spec)
		require.NoError(t, err)

		require.NoError(t, e.Exit("task-1", 2))

		instance, err := e.Inspect(t.Context(), "task-1")
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceExited, instance.State)
		assert.Equal(t, 2, instance.ExitCode)

		require.NoError(t, e.Stop(t.Context(), "task-1"))

		stopped, err := e.Inspect(t.Context(), "task-1")
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceExited, stopped.State, "stopping one that exited leaves what it returned")
	})
}

func TestEngine_Logs(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }

	e := New(WithClock(clock))

	_, err := e.Create(t.Context(), machine("vm-1"))
	require.NoError(t, err)

	for n, line := range []string{"one", "two", "three", "four"} {
		now = time.Date(2026, 10, 4, 12, 0, n, 0, time.UTC)
		require.NoError(t, e.Log("vm-1", vm.LogSourceKernel, line))
	}

	testcases := []struct {
		name    string
		options vm.LogOptions
		want    []string
	}{
		{name: "all of it", options: vm.LogOptions{}, want: []string{"one", "two", "three", "four"}},
		{name: "since a moment, that moment included", options: vm.LogOptions{Since: time.Date(2026, 10, 4, 12, 0, 2, 0, time.UTC)}, want: []string{"three", "four"}},
		{name: "the last lines", options: vm.LogOptions{Tail: 3}, want: []string{"two", "three", "four"}},
		{name: "the last of what is since", options: vm.LogOptions{Since: time.Date(2026, 10, 4, 12, 0, 1, 0, time.UTC), Tail: 1}, want: []string{"four"}},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lines, err := e.Logs(t.Context(), "vm-1", tt.options)
			require.NoError(t, err)

			got := make([]string, len(lines))
			for n, line := range lines {
				got[n] = line.Line
			}

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEngine_Stats(t *testing.T) {
	t.Parallel()

	e := New()

	_, err := e.Create(t.Context(), machine("vm-1"))
	require.NoError(t, err)

	require.NoError(t, e.SetStats("vm-1", vm.Stats{CPUPercent: 50, MemoryUsed: 64 << 20}))

	stats, err := e.Stats(t.Context(), "vm-1")
	require.NoError(t, err)
	assert.Equal(t, 50.0, stats.CPUPercent)
	assert.Equal(t, uint64(64<<20), stats.MemoryUsed)
	assert.Equal(t, uint64(256<<20), stats.MemoryLimit, "the limit is what it was given")
	assert.Equal(t, uint64(1<<30), stats.DiskTotal)
	assert.False(t, stats.SampledAt.IsZero())

	require.NoError(t, e.Stop(t.Context(), "vm-1"))

	_, err = e.Stats(t.Context(), "vm-1")
	assert.ErrorIs(t, err, vm.ErrNotRunning)
}

func TestEngine_SnapshotAndRestore(t *testing.T) {
	t.Parallel()

	t.Run("a restore as a new instance brings the disk back", func(t *testing.T) {
		t.Parallel()

		e := New()

		_, err := e.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)
		require.NoError(t, e.SetDisk("vm-1", []byte("what was written")))

		var archived bytes.Buffer
		archive, err := e.Snapshot(t.Context(), "vm-1", &archived)
		require.NoError(t, err)

		assert.Equal(t, vm.Archive{
			Engine: "memory/1",
			Kind:   vm.KindMachine,
			Image:  "ubuntu:24.04",
			Disk:   1 << 30,
			Size:   int64(archived.Len()),
		}, archive)

		restored, err := e.Restore(t.Context(), machine("vm-2"), &archived)
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceRunning, restored.State)

		disk, err := e.Disk("vm-2")
		require.NoError(t, err)
		assert.Equal(t, "what was written", string(disk))
	})

	t.Run("a restore onto an instance replaces its disk and keeps its ports", func(t *testing.T) {
		t.Parallel()

		e := New()

		created, err := e.Create(t.Context(), machine("vm-1"))
		require.NoError(t, err)
		require.NoError(t, e.SetDisk("vm-1", []byte("before")))

		var archived bytes.Buffer
		_, err = e.Snapshot(t.Context(), "vm-1", &archived)
		require.NoError(t, err)

		require.NoError(t, e.SetDisk("vm-1", []byte("after")))

		restored, err := e.Restore(t.Context(), machine("vm-1"), &archived)
		require.NoError(t, err)
		assert.Equal(t, created.Endpoints, restored.Endpoints)

		disk, err := e.Disk("vm-1")
		require.NoError(t, err)
		assert.Equal(t, "before", string(disk))
	})

	t.Run("an archive another engine wrote is refused", func(t *testing.T) {
		t.Parallel()

		written, err := json.Marshal(archive{Engine: "microsandbox/0.7.6", Spec: machine("vm-1")})
		require.NoError(t, err)

		_, err = New().Restore(t.Context(), machine("vm-1"), bytes.NewReader(written))
		assert.ErrorIs(t, err, vm.ErrEngineMismatch)
	})

	t.Run("nothing is archived of an instance that is not there", func(t *testing.T) {
		t.Parallel()

		_, err := New().Snapshot(t.Context(), "nothing", io.Discard)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
