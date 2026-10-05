package vmhost

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/danceable/console"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	vmhostProviders "github.com/khanzadimahdi/testproject/infrastructure/ioc/providers/workload/vmhost"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/vmhost"
	vmhostAPI "github.com/khanzadimahdi/testproject/presentation/http/workload/vmhost"
)

const settle = 10 * time.Second

// engine is the memory engine, with the shutdown a vmhost's engine has.
type engine struct {
	*memory.Engine

	lock     sync.Mutex
	shutdown []time.Duration
}

func (e *engine) Shutdown(ctx context.Context) error {
	deadline, _ := ctx.Deadline()

	e.lock.Lock()
	defer e.lock.Unlock()

	e.shutdown = append(e.shutdown, time.Until(deadline))

	return nil
}

// shutdowns are how long each shutdown it was asked for was given.
func (e *engine) shutdowns() []time.Duration {
	e.lock.Lock()
	defer e.lock.Unlock()

	return append([]time.Duration(nil), e.shutdown...)
}

// socketPath is a socket of the test's own, short enough to be one.
func socketPath(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "vmhost")
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})

	return filepath.Join(dir, "run", "vmhost.sock")
}

// command is a serve command assembled by hand, as the console would boot it,
// over the memory engine.
func command(t *testing.T, signals ...os.Signal) (*ServeCommand, *engine) {
	t.Helper()

	e := &engine{Engine: memory.New()}
	logger := slog.New(slog.DiscardHandler)

	c := NewServeCommand()
	c.configs.Socket = socketPath(t)
	c.engine = e
	c.server = vmhostAPI.NewServer(e, logger)
	c.logger = logger
	c.group = os.Getgid()
	c.signals = signals

	return c, e
}

// run runs a command until the test is done with it, and says how it ended.
func run(ctx context.Context, c *ServeCommand) <-chan console.ExitStatus {
	ended := make(chan console.ExitStatus, 1)

	go func() {
		ended <- c.Run(ctx)
	}()

	return ended
}

// answering waits for a vmhost to answer on socket.
func answering(t *testing.T, socket string) *vmhost.Client {
	t.Helper()

	client := vmhost.NewClient(socket)

	require.Eventually(t, func() bool {
		_, err := client.Info(t.Context())

		return err == nil
	}, settle, 10*time.Millisecond, "the vmhost never answered")

	return client
}

func TestServeCommand_Describes(t *testing.T) {
	t.Parallel()

	c := NewServeCommand()

	assert.Equal(t, "serve-workload-vmhost", c.Name())
	assert.NotEmpty(t, c.Description())
	assert.Equal(t, "serve-workload-vmhost [arguments]", c.Usage())
	assert.Equal(t, []os.Signal{syscall.SIGTERM}, c.signals, "docker stop sends SIGTERM, and the vmhost stops its vms on it")
	assert.Equal(t, 10001, c.group)
	assert.Len(t, c.Providers(), 5)
}

func TestServeCommand_Configure(t *testing.T) {
	t.Setenv("WORKLOAD_VMHOST_SOCKET", "/run/vmhost/from-the-environment.sock")
	t.Setenv("WORKLOAD_VMHOST_ADVERTISE_HOST", "10.89.1.10")
	t.Setenv("WORKLOAD_VMHOST_ORCHESTRATOR_IP", "10.89.1.2")
	t.Setenv("MSB_HOME", "/data/msb")
	t.Setenv("WORKLOAD_VMHOST_MEMORY", "")

	c := NewServeCommand()

	flagSet := console.NewFlagSet(c.Name(), io.Discard)
	c.Configure(flagSet)

	require.NoError(t, flagSet.Parse([]string{"--port-range", "21000-21999", "--cpus", "6"}))

	assert.Equal(t, "/run/vmhost/from-the-environment.sock", c.configs.Socket)
	assert.Equal(t, "10.89.1.10", c.configs.AdvertiseHost)
	assert.Equal(t, "10.89.1.2", c.configs.OrchestratorAddress)
	assert.Equal(t, "21000-21999", c.configs.PortRange)
	assert.Equal(t, uint(6), c.configs.CPUs)
	assert.Zero(t, c.configs.Memory, "an empty setting keeps the default, which leaves it to the vmhost")
}

func TestServeCommand_Run(t *testing.T) {
	t.Parallel()

	c, e := command(t)

	ctx, stop := context.WithCancel(t.Context())
	ended := run(ctx, c)

	client := answering(t, c.configs.Socket)

	_, err := client.Create(t.Context(), vm.Spec{ID: "vm-1", Kind: vm.KindMachine, Image: "ubuntu:24.04"})
	require.NoError(t, err)

	info, err := os.Stat(c.configs.Socket)
	require.NoError(t, err)
	assert.Equal(t, fs.ModeSocket|0o660, info.Mode(), "the vmhost and its group may connect, and nobody else")
	assert.Equal(t, uint32(os.Getgid()), info.Sys().(*syscall.Stat_t).Gid)

	stop()

	select {
	case status := <-ended:
		assert.Equal(t, console.ExitSuccess, status)
	case <-time.After(settle):
		t.Fatal("the vmhost did not stop")
	}

	shutdowns := e.shutdowns()
	require.Len(t, shutdowns, 1, "every vm is stopped on the way out")
	assert.InDelta(t, vmhostProviders.ShutdownTimeout.Seconds(), shutdowns[0].Seconds(), 5, "and given the time a container's stop grace leaves")

	_, err = os.Lstat(c.configs.Socket)
	assert.ErrorIs(t, err, fs.ErrNotExist, "and the socket goes with it, so the orchestrator finds it gone")
}

// TestServeCommand_SIGTERM stops a vmhost the way docker stop does. It sends
// the test's own process a SIGTERM, which the command catches: it does not run
// in parallel, so nothing else is waiting on signals meanwhile.
func TestServeCommand_SIGTERM(t *testing.T) {
	c, e := command(t, syscall.SIGTERM)

	ended := run(t.Context(), c)
	answering(t, c.configs.Socket)

	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))

	select {
	case status := <-ended:
		assert.Equal(t, console.ExitSuccess, status)
	case <-time.After(settle):
		t.Fatal("the vmhost did not stop on SIGTERM")
	}

	assert.Len(t, e.shutdowns(), 1)
}

func TestServeCommand_Socket(t *testing.T) {
	t.Parallel()

	t.Run("a socket a vmhost left behind is taken over", func(t *testing.T) {
		t.Parallel()

		c, _ := command(t)
		require.NoError(t, os.MkdirAll(filepath.Dir(c.configs.Socket), 0o770))

		left, err := net.Listen("unix", c.configs.Socket)
		require.NoError(t, err)
		left.(*net.UnixListener).SetUnlinkOnClose(false)
		require.NoError(t, left.Close())

		ctx, stop := context.WithCancel(t.Context())
		defer stop()

		ended := run(ctx, c)
		answering(t, c.configs.Socket)

		stop()
		assert.Equal(t, console.ExitSuccess, <-ended)
	})

	t.Run("a socket another vmhost serves is not", func(t *testing.T) {
		t.Parallel()

		c, e := command(t)
		require.NoError(t, os.MkdirAll(filepath.Dir(c.configs.Socket), 0o770))

		serving, err := net.Listen("unix", c.configs.Socket)
		require.NoError(t, err)
		t.Cleanup(func() { _ = serving.Close() })

		assert.Equal(t, console.ExitFailure, <-run(t.Context(), c))
		assert.Len(t, e.shutdowns(), 1, "the vms it took over are stopped rather than killed with it")

		_, err = os.Lstat(c.configs.Socket)
		assert.NoError(t, err, "the other vmhost's socket is left alone")
	})

	t.Run("something that is not a socket is not", func(t *testing.T) {
		t.Parallel()

		c, _ := command(t)
		require.NoError(t, os.MkdirAll(filepath.Dir(c.configs.Socket), 0o770))
		require.NoError(t, os.WriteFile(c.configs.Socket, []byte("not a socket"), 0o600))

		assert.Equal(t, console.ExitFailure, <-run(t.Context(), c))

		content, err := os.ReadFile(c.configs.Socket)
		require.NoError(t, err)
		assert.Equal(t, "not a socket", string(content), "and is left as it was")
	})

	t.Run("a group the socket cannot be given to", func(t *testing.T) {
		t.Parallel()

		if os.Getuid() == 0 {
			t.Skip("root may give a file to any group")
		}

		c, _ := command(t)
		c.group = 4242

		assert.Equal(t, console.ExitFailure, <-run(t.Context(), c))
	})
}

func TestCheckCommand(t *testing.T) {
	t.Parallel()

	t.Run("a vmhost that answers", func(t *testing.T) {
		t.Parallel()

		c, _ := command(t)

		ctx, stop := context.WithCancel(t.Context())
		defer stop()

		ended := run(ctx, c)
		answering(t, c.configs.Socket)

		var out, errs bytes.Buffer

		check := NewCheckCommand()
		check.out, check.err = &out, &errs

		flagSet := console.NewFlagSet(check.Name(), io.Discard)
		check.Configure(flagSet)
		require.NoError(t, flagSet.Parse([]string{"--socket", c.configs.Socket}))

		assert.Equal(t, console.ExitSuccess, check.Run(t.Context()))
		assert.Contains(t, out.String(), "memory 1 answers on "+c.configs.Socket)
		assert.Empty(t, errs.String())

		stop()
		<-ended
	})

	t.Run("a vmhost that is not there", func(t *testing.T) {
		t.Parallel()

		var out, errs bytes.Buffer

		check := NewCheckCommand()
		check.socket = socketPath(t)
		check.out, check.err = &out, &errs

		assert.Equal(t, console.ExitFailure, check.Run(t.Context()))
		assert.Contains(t, errs.String(), "not answering")
		assert.Contains(t, errs.String(), check.socket)
	})

	t.Run("its socket by default is the vmhost's", func(t *testing.T) {
		t.Parallel()

		check := NewCheckCommand()

		assert.Equal(t, "/run/vmhost/vmhost.sock", check.socket)
		assert.Equal(t, "check-workload-vmhost", check.Name())
		assert.NotEmpty(t, check.Description())
	})
}
