//go:build microvm && linux

package vmhost_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost"
	"github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/vmstate"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/fabric"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/firecracker"
	guestClient "github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/guest"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/image"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/initrd"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/output"
)

// These boot real machines with vmhost's real parts: they need /dev/kvm,
// firecracker, mke2fs and sqfstar, a kernel at WORKLOAD_TEST_KERNEL, a
// registry to pull alpine and busybox from, and root, since machines run as
// users of their own and are plugged into networks. They run at integration,
// in a Lima VM with nested virtualisation (05-implementation-map.md, appendix
// B):
//
//	sudo -E env WORKLOAD_TEST_KERNEL=/tmp/vmlinux go test -tags microvm ./application/workload/vmhost/
//
// WORKLOAD_TEST_PROCESS_MODE=systemd boots machines as host units instead of
// as children of the test.

const testNode = "orchestrator-test"

// host is a data directory with everything a machine boots from in it, and
// the parts that boot it.
type host struct {
	dataDir string
	config  vmhost.Config
	parts   func(t *testing.T) vmhost.Parts
	logger  *slog.Logger
}

func newHost(t *testing.T) *host {
	t.Helper()

	kernel := os.Getenv("WORKLOAD_TEST_KERNEL")
	if len(kernel) == 0 {
		t.Skip("WORKLOAD_TEST_KERNEL names no kernel to boot")
	}

	if os.Geteuid() != 0 {
		t.Skip("machines run as users of their own and are plugged into networks, which takes root")
	}

	if _, err := os.Stat("/dev/kvm"); err != nil {
		t.Skip("there is no /dev/kvm to run machines with")
	}

	firecrackerBinary := os.Getenv("WORKLOAD_TEST_FIRECRACKER")
	if len(firecrackerBinary) == 0 {
		found, err := exec.LookPath("firecracker")
		if err != nil {
			t.Skip("there is no firecracker to run machines with")
		}

		firecrackerBinary = found
	}

	mode := firecracker.ModeChild
	if os.Getenv("WORKLOAD_TEST_PROCESS_MODE") == string(firecracker.ModeSystemd) {
		mode = firecracker.ModeSystemd
	}

	// short on purpose: a socket's path is at most 108 bytes, and a
	// machine's sockets are several directories down. Under /var/lib, as
	// vmhost's own is, rather than /tmp: a machine's unit has a /tmp of its
	// own (PrivateTmp), and would not find its directory in the host's.
	dataDir, err := os.MkdirTemp("/var/lib", "vh")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dataDir) })

	require.NoError(t, layout.Prepare(dataDir))

	binary, err := layout.Install(layout.Bin(dataDir), firecrackerBinary, "firecracker", 0o755)
	require.NoError(t, err)

	bootKernel, err := layout.Install(layout.Boot(dataDir), kernel, "vmlinux", 0o644)
	require.NoError(t, err)

	agent := filepath.Join(dataDir, "workload-guest")
	build := exec.Command("go", "build", "-o", agent, "./cmd/workload-guest")
	build.Dir = moduleRoot(t)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	built, err := build.CombinedOutput()
	require.NoError(t, err, string(built))

	initramfs, err := initrd.Ensure(layout.Boot(dataDir), agent)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if testing.Verbose() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}

	// clear of the subordinate ids a host gives its users, which on a Lima
	// VM (524288-1074266111) cover vmhost's default first machine user, and
	// which the hypervisor refuses to share. Clear too of the firecracker
	// package's tests (2000000000 on), which go test runs at the same time:
	// a vmhost takes every machine of its users for its own, and would end
	// theirs as orphans. And clear of the local stack's vmhost (2000000000
	// and the 65536 after it).
	const (
		firstUID = 2_000_100_000
		uids     = 64
	)

	_, pool, err := net.ParseCIDR("10.251.0.0/16")
	require.NoError(t, err)

	h := &host{
		dataDir: dataDir,
		logger:  logger,
		config: vmhost.Config{
			Version:        "test",
			ProcessMode:    string(mode),
			Architecture:   runtime.GOARCH,
			Kernel:         bootKernel,
			Initrd:         initramfs,
			ScratchPath:    func(id string) string { return layout.Scratch(dataDir, id) },
			MaxMemory:      4 << 30,
			MinMemory:      128 << 20,
			MemoryOverhead: 64 << 20,
			MaxVMMemory:    1 << 30,
			MaxVMCPU:       2,
			HostCPUs:       runtime.NumCPU(),
			CPUOvercommit:  4,
			DiskReserve:    1 << 30,
			DiskOvercommit: 3,
			FirstUID:       firstUID,
			UIDs:           uids,
			Nameservers:    []string{"1.1.1.1"},
		},
	}

	h.parts = func(t *testing.T) vmhost.Parts {
		t.Helper()

		hypervisor, err := firecracker.New(firecracker.Config{
			DataDir:          dataDir,
			Binary:           binary,
			Mode:             mode,
			Slice:            "workload-vm-test.slice",
			NetworkNamespace: fmt.Sprintf("/proc/%d/ns/net", os.Getpid()),
			FirstUID:         firstUID,
			UIDs:             uids,
			MemoryOverhead:   64 << 20,
		}, logger)
		require.NoError(t, err)

		images, err := image.NewStore(image.Config{Dir: layout.Images(dataDir)}, logger)
		require.NoError(t, err)

		networks, err := fabric.New(fabric.Config{Dir: layout.Fabric(dataDir), Pool: pool, FirstUID: firstUID, UIDs: uids}, logger)
		require.NoError(t, err)

		states, err := vmstate.Open(dataDir)
		require.NoError(t, err)

		return vmhost.Parts{
			Hypervisor: hypervisor,
			Images:     images,
			Fabric:     networks,
			Guests:     guestClient.NewConnector(),
			States:     states,
			Logs:       output.NewStore(dataDir, 32<<20),
			FreeSpace:  func() (uint64, error) { return layout.FreeSpace(dataDir) },
		}
	}

	return h
}

// engine is a vmhost on the host, as it is when it starts: it takes back
// what an earlier one left running before anything is asked of it.
func (h *host) engine(t *testing.T) *vmhost.Engine {
	t.Helper()

	engine, err := vmhost.New(h.config, h.parts(t), h.logger)
	require.NoError(t, err)

	require.NoError(t, engine.Reconcile(context.Background()))

	return engine
}

func closeEngine(t *testing.T, engine *vmhost.Engine) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	require.NoError(t, engine.Close(ctx))
}

func moduleRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "the module's root is not above here")
		dir = parent
	}
}

// spec is a task on no network, as the orchestrator asks for one.
func spec(name string, command ...string) vm.Spec {
	return vm.Spec{
		Name:      name,
		Hostname:  name,
		Image:     "alpine:3.20",
		Labels:    map[string]string{"node.name": testNode, "task.slug": name},
		Command:   command,
		Resources: vm.Resources{CPU: 1, Memory: 128 << 20, Disk: 128 << 20},
	}
}

// waitFor waits for a VM to be what until says, and says what it was.
func waitFor(t *testing.T, engine *vmhost.Engine, id string, until func(vm.VM) bool) vm.VM {
	t.Helper()

	deadline := time.Now().Add(60 * time.Second)

	for {
		v, err := engine.VM(context.Background(), id)
		require.NoError(t, err)

		if until(v) {
			return v
		}

		require.True(t, time.Now().Before(deadline), "the vm never got there: %+v", v)
		time.Sleep(50 * time.Millisecond)
	}
}

func ended(v vm.VM) bool {
	return v.State == vm.StateExited || v.State == vm.StateDead
}

func outputOf(t *testing.T, engine *vmhost.Engine, id string) string {
	t.Helper()

	var kept strings.Builder
	require.NoError(t, engine.Logs(context.Background(), id, 0, time.Time{}, false, func(line vm.LogLine) error {
		kept.WriteString(line.Content + "\n")

		return nil
	}))

	return kept.String()
}

// watchWhile does something to a VM, and says every state it read as while
// that was done.
func watchWhile(t *testing.T, engine *vmhost.Engine, id string, do func(context.Context, string) error) []vm.State {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- do(context.Background(), id) }()

	// what it read as before what was asked took hold is not what is watched.
	time.Sleep(100 * time.Millisecond)

	var states []vm.State

	for {
		if v, err := engine.VM(context.Background(), id); err == nil {
			states = append(states, v.State)
		}

		select {
		case err := <-done:
			require.NoError(t, err)

			return slices.Compact(states)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestVMHost(t *testing.T) {
	h := newHost(t)
	engine := h.engine(t)
	defer closeEngine(t, engine)

	ctx := context.Background()

	_, err := engine.PrepareImage(ctx, "alpine:3.20")
	require.NoError(t, err)

	t.Run("a job runs in a machine of its own, and says what it wrote and what it returned", func(t *testing.T) {
		id, err := engine.Create(ctx, spec("job", "/bin/sh", "-c", "echo hello from a microvm; echo and from its stderr >&2; exit 3"))
		require.NoError(t, err)

		started := time.Now()
		require.NoError(t, engine.Start(ctx, id))

		finished := waitFor(t, engine, id, ended)
		t.Logf("the job ran and ended in %s", time.Since(started))

		assert.Equal(t, vm.StateExited, finished.State)
		assert.Equal(t, 3, finished.ExitCode)
		assert.Equal(t, "hello from a microvm\nand from its stderr\n", outputOf(t, engine, id))

		var streams []string
		require.NoError(t, engine.Logs(ctx, id, 0, time.Time{}, false, func(line vm.LogLine) error {
			streams = append(streams, line.Stream)

			return nil
		}))
		assert.Equal(t, []string{guest.StreamStdout, guest.StreamStderr}, streams)

		require.NoError(t, engine.Delete(ctx, id))

		_, err = engine.VM(ctx, id)
		assert.ErrorIs(t, err, vm.ErrNotFound)
	})

	t.Run("a task that fails is started again as often as its policy says, inside its machine", func(t *testing.T) {
		failing := spec("failing", "/bin/sh", "-c", "echo attempt; exit 1")
		failing.RestartPolicy = "on-failure:2"

		id, err := engine.Create(ctx, failing)
		require.NoError(t, err)
		require.NoError(t, engine.Start(ctx, id))

		finished := waitFor(t, engine, id, func(v vm.VM) bool { return ended(v) && v.RestartCount == 2 })

		assert.Equal(t, 1, finished.ExitCode)
		assert.Equal(t, "attempt\nattempt\nattempt\n", outputOf(t, engine, id))

		require.NoError(t, engine.Delete(ctx, id))
	})

	t.Run("a service runs until it is stopped, and a terminal can be opened in it meanwhile", func(t *testing.T) {
		id, err := engine.Create(ctx, spec("service", "/bin/sh", "-c", "echo up; exec sleep 1000"))
		require.NoError(t, err)
		require.NoError(t, engine.Start(ctx, id))

		running := waitFor(t, engine, id, func(v vm.VM) bool { return v.State == vm.StateRunning })
		assert.False(t, running.StartedAt.IsZero())

		stats, err := engine.Stats(ctx, id)
		require.NoError(t, err)
		assert.NotZero(t, stats.PIDs)
		assert.Equal(t, uint64(128<<20), stats.MemoryLimit)

		execID, conn, err := engine.Exec(ctx, id, guest.Exec{Process: guest.Process{Args: []string{"/bin/sh"}}, TTY: true, Rows: 24, Cols: 80})
		require.NoError(t, err)

		require.NoError(t, guest.WriteFrame(conn, guest.FrameResize, guest.ResizePayload(24, 80)))
		require.NoError(t, guest.WriteFrame(conn, guest.FrameStdin, []byte("echo $((6*7)) > /tmp/answer; cat /tmp/answer; stty size\n")))

		answer := make(chan string, 1)
		go func() {
			var seen strings.Builder

			for {
				frame, err := guest.ReadFrame(conn)
				if err != nil {
					answer <- seen.String()

					return
				}

				seen.Write(frame.Payload)

				if strings.Contains(seen.String(), "24 80") {
					answer <- seen.String()

					return
				}
			}
		}()

		select {
		case seen := <-answer:
			assert.Contains(t, seen, "42")
			assert.Contains(t, seen, "24 80")
		case <-time.After(10 * time.Second):
			t.Fatal("the terminal never answered")
		}

		require.NoError(t, conn.Close())

		endCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		_, err = engine.EndExec(endCtx, id, execID, guest.EndExec{Grace: time.Second, KillGrace: time.Second})
		require.NoError(t, err)

		require.NoError(t, engine.Stop(ctx, id, vm.DefaultStopTimeout))

		stopped := waitFor(t, engine, id, ended)
		assert.Equal(t, vm.StateExited, stopped.State)
		assert.Equal(t, 143, stopped.ExitCode, "sleep ends on the TERM a stop sends")
		assert.Contains(t, outputOf(t, engine, id), "up\n")

		// started again from the same disks, and never taken for ended while
		// it boots: what it wrote is still there.
		states := watchWhile(t, engine, id, engine.Start)
		assert.Contains(t, states, vm.StateRestarting, "a stopped vm booting again says so")
		for _, state := range states {
			assert.Contains(t, []vm.State{vm.StateRunning, vm.StateRestarting}, state)
		}

		_, check, err := engine.Exec(ctx, id, guest.Exec{Process: guest.Process{Args: []string{"cat", "/tmp/answer"}}})
		require.NoError(t, err)

		var kept strings.Builder
		for {
			frame, err := guest.ReadFrame(check)
			if err != nil || frame.Type == guest.FrameExit {
				break
			}

			kept.Write(frame.Payload)
		}

		assert.Equal(t, "42\n", kept.String())
		require.NoError(t, check.Close())

		// a restart is never an end, to anybody watching it, whether the vm
		// was running...
		for _, state := range watchWhile(t, engine, id, engine.Restart) {
			assert.Contains(t, []vm.State{vm.StateRunning, vm.StateRestarting}, state)
		}

		// ...or had been stopped, and has a machine to boot before it runs.
		require.NoError(t, engine.Stop(ctx, id, vm.DefaultStopTimeout))
		waitFor(t, engine, id, ended)

		states = watchWhile(t, engine, id, engine.Restart)
		assert.Contains(t, states, vm.StateRestarting)

		require.NoError(t, engine.Kill(ctx, id))

		killed := waitFor(t, engine, id, ended)
		assert.Equal(t, 137, killed.ExitCode)

		require.NoError(t, engine.Delete(ctx, id))
	})

	t.Run("a read-only task can change nothing of its root", func(t *testing.T) {
		readOnly := spec("read-only", "/bin/sh", "-c", "touch /changed")
		readOnly.ReadOnly = true

		id, err := engine.Create(ctx, readOnly)
		require.NoError(t, err)
		require.NoError(t, engine.Start(ctx, id))

		finished := waitFor(t, engine, id, ended)
		assert.NotEqual(t, 0, finished.ExitCode)
		assert.Contains(t, outputOf(t, engine, id), "Read-only file system")

		require.NoError(t, engine.Delete(ctx, id))
	})

	t.Run("a command the image does not have is a task that could not start", func(t *testing.T) {
		id, err := engine.Create(ctx, spec("missing", "no-such-command"))
		require.NoError(t, err)

		assert.Error(t, engine.Start(ctx, id))

		finished, err := engine.VM(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, 127, finished.ExitCode)

		require.NoError(t, engine.Delete(ctx, id))
	})

	t.Run("a task's port is reached through its agent, on the task's own network", func(t *testing.T) {
		_, err := engine.EnsureNetwork(ctx, network.IsolatedNetworkName, false)
		require.NoError(t, err)

		web := spec("web", "/bin/sh", "-c", "mkdir -p /www && echo hello > /www/index.html && exec httpd -f -p 80 -h /www")
		web.Image = "busybox:1.36"
		web.Networks = []vm.Attachment{{Network: network.IsolatedNetworkName}}
		web.ExposedPorts = []uint16{80}

		id, err := engine.Create(ctx, web)
		require.NoError(t, err)
		require.NoError(t, engine.Start(ctx, id))

		running := waitFor(t, engine, id, func(v vm.VM) bool { return v.State == vm.StateRunning })
		require.Equal(t, []uint16{80}, running.Endpoints())

		var body string

		require.Eventually(t, func() bool {
			conn, err := engine.Dial(ctx, id, 80)
			if err != nil {
				return false
			}
			defer conn.Close()

			if _, err := conn.Write([]byte("GET / HTTP/1.0\r\n\r\n")); err != nil {
				return false
			}

			answer, err := io.ReadAll(bufio.NewReader(conn))
			if err != nil {
				return false
			}

			body = string(answer)

			return strings.Contains(body, "hello")
		}, 30*time.Second, 200*time.Millisecond, "the task's port never answered: %q", body)

		require.NoError(t, engine.Delete(ctx, id))
	})
}

func TestVMHostTakesItsVMsBack(t *testing.T) {
	h := newHost(t)
	ctx := context.Background()

	first := h.engine(t)

	id, err := first.Create(ctx, spec("long", "/bin/sh", "-c", "echo before; sleep 2; echo after; exec sleep 1000"))
	require.NoError(t, err)
	require.NoError(t, first.Start(ctx, id))
	waitFor(t, first, id, func(v vm.VM) bool { return v.State == vm.StateRunning })

	// vmhost goes away; the vm does not.
	closeEngine(t, first)

	second := h.engine(t)
	defer closeEngine(t, second)

	// what the task wrote while nobody was looking is kept all the same.
	deadline := time.Now().Add(20 * time.Second)
	for !strings.Contains(outputOf(t, second, id), "after") {
		require.True(t, time.Now().Before(deadline), "what was written meanwhile never arrived: %q", outputOf(t, second, id))
		time.Sleep(100 * time.Millisecond)
	}

	assert.Equal(t, "before\nafter\n", outputOf(t, second, id))

	require.NoError(t, second.Stop(ctx, id, vm.DefaultStopTimeout))
	waitFor(t, second, id, ended)
	require.NoError(t, second.Delete(ctx, id))
}
