//go:build linux && microvm

package firecracker

// These tests boot real machines, and need what vmhost needs: root, KVM, a
// firecracker, the kernel machines boot (WORKLOAD_TEST_KERNEL), the agent they
// boot as their init (WORKLOAD_TEST_GUEST, a static build of
// cmd/workload-guest) and an image with a shell in it to boot
// (WORKLOAD_TEST_IMAGE, a squashfs). In systemd mode they also need the
// host's systemd, its setpriv, and iproute2 to make a network namespace with.
//
//	sudo -E env WORKLOAD_TEST_KERNEL=/opt/vmlinux WORKLOAD_TEST_GUEST=/opt/workload-guest \
//	  WORKLOAD_TEST_IMAGE=/opt/alpine.squashfs go test -tags microvm -run TestMicroVM \
//	  ./infrastructure/workload/vmm/firecracker/

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	guestProtocol "github.com/khanzadimahdi/testproject/domain/workload/guest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/guest"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/initrd"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

const (
	// testFirstUID is where machines' users count up from here: clear of the
	// subordinate ids a development VM gives its own user, which the
	// default range is not.
	testFirstUID = 2_000_000_000
	testUIDs     = 8

	// testSlice is the slice the tests' units run in, apart from any other.
	testSlice = "wkvmtest.slice"

	// testHostname is what the tests' machines call themselves.
	testHostname = "s4a-test"
)

// testMachines is what the tests boot machines from.
type testMachines struct {
	firecracker string
	kernel      string
	guest       string
	image       string
}

// machines says what the tests boot machines from, and skips them where no
// machine can be booted.
func machines(t *testing.T) testMachines {
	t.Helper()

	if os.Geteuid() != 0 {
		t.Skip("booting machines takes root")
	}

	if _, err := os.Stat(kvmDevice); err != nil {
		t.Skip("booting machines takes KVM")
	}

	m := testMachines{
		firecracker: os.Getenv("WORKLOAD_TEST_FIRECRACKER"),
		kernel:      os.Getenv("WORKLOAD_TEST_KERNEL"),
		guest:       os.Getenv("WORKLOAD_TEST_GUEST"),
		image:       os.Getenv("WORKLOAD_TEST_IMAGE"),
	}

	if len(m.firecracker) == 0 {
		m.firecracker = "/usr/local/bin/firecracker"
	}

	if len(m.kernel) == 0 || len(m.guest) == 0 || len(m.image) == 0 {
		t.Skip("WORKLOAD_TEST_KERNEL, WORKLOAD_TEST_GUEST and WORKLOAD_TEST_IMAGE name what machines boot")
	}

	return m
}

// stage makes a data directory, somewhere a unit sees as vmhost does, and
// puts in it what machines boot, as vmhost does: the kernel and the image
// everybody's to read, and the initramfs made from the agent.
func (m testMachines) stage(t *testing.T) (string, string, string, string) {
	t.Helper()

	dataDir, err := os.MkdirTemp("/var/lib", "wkvm-test-")
	require.NoError(t, err)

	t.Cleanup(func() { _ = os.RemoveAll(dataDir) })

	require.NoError(t, layout.Prepare(dataDir))

	kernel := filepath.Join(layout.Boot(dataDir), "vmlinux-test")
	copyShared(t, m.kernel, kernel)

	initramfs, err := initrd.Ensure(layout.Boot(dataDir), m.guest)
	require.NoError(t, err)

	image := filepath.Join(layout.Images(dataDir), "test", "rootfs.squashfs")
	require.NoError(t, os.MkdirAll(filepath.Dir(image), 0o700))
	copyShared(t, m.image, image)

	return dataDir, kernel, initramfs, image
}

// copyShared copies a file every machine reads, everybody's to read.
func copyShared(t *testing.T, source string, target string) {
	t.Helper()

	copyFile(t, source, target)
	require.NoError(t, os.Chmod(target, 0o644))
}

// scratch makes a machine's scratch disk, as vmhost's image store does.
func scratch(t *testing.T, dataDir string, id string) string {
	t.Helper()

	path := layout.Scratch(dataDir, id)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.Truncate(createFile(t, path), 256<<20))

	output, err := exec.Command("mkfs.ext4", "-q", "-F", path).CombinedOutput()
	require.NoError(t, err, string(output))

	return path
}

func createFile(t *testing.T, path string) string {
	t.Helper()

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	return path
}

// machineID is a machine's ID, never one booted before.
func machineID(t *testing.T) string {
	t.Helper()

	var id [8]byte
	_, err := io.ReadFull(randomSource(), id[:])
	require.NoError(t, err)

	return fmt.Sprintf("%x", id)
}

func randomSource() io.Reader {
	file, err := os.Open("/dev/urandom")
	if err != nil {
		panic(err)
	}

	return file
}

// netns makes a network namespace of the tests' own, for machines' units to
// join, and takes it away with the test.
func netns(t *testing.T) string {
	t.Helper()

	name := "wkvm-test-" + strconv.Itoa(os.Getpid())

	output, err := exec.Command("ip", "netns", "add", name).CombinedOutput()
	require.NoError(t, err, string(output))

	t.Cleanup(func() { _ = exec.Command("ip", "netns", "del", name).Run() })

	return filepath.Join("/run/netns", name)
}

// handoff is what the test that boots a machine for another process is given.
type handoff struct {
	Config Config
	Spec   vm.MachineSpec
}

func TestMicroVM(t *testing.T) {
	m := machines(t)

	t.Run("child", func(t *testing.T) {
		testMachinesIn(t, m, ModeChild, "")
	})

	t.Run("systemd", func(t *testing.T) {
		testMachinesIn(t, m, ModeSystemd, netns(t))
	})
}

// TestMicroVMBootForAnother boots a machine and leaves it running, for the test
// that runs it as a process of its own to find the machine again once this
// process is gone.
func TestMicroVMBootForAnother(t *testing.T) {
	path := os.Getenv("WORKLOAD_TEST_HANDOFF")
	if len(path) == 0 {
		t.Skip("only TestMicroVM runs it, as another process")
	}

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var given handoff
	require.NoError(t, json.Unmarshal(content, &given))

	h, err := New(given.Config, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	defer h.Close()

	_, err = h.Boot(t.Context(), given.Spec)
	require.NoError(t, err)
}

func testMachinesIn(t *testing.T, m testMachines, mode Mode, namespace string) {
	dataDir, kernel, initramfs, image := m.stage(t)

	config := Config{
		DataDir:          dataDir,
		Binary:           m.firecracker,
		Mode:             mode,
		Slice:            testSlice,
		NetworkNamespace: namespace,
		FirstUID:         testFirstUID,
		UIDs:             testUIDs,
		MemoryOverhead:   64 << 20,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	h, err := New(config, logger)
	require.NoError(t, err)
	t.Cleanup(func() { _ = h.Close() })

	assert.Equal(t, "v1.17.0", h.Version())

	spec := func(id string, uid int) vm.MachineSpec {
		return vm.MachineSpec{
			ID:         id,
			VCPUs:      1,
			CPU:        0.5,
			MemoryMiB:  256,
			Kernel:     kernel,
			Initrd:     initramfs,
			KernelArgs: guestProtocol.KernelArgs,
			Drives:     []vm.Drive{{Path: image, ReadOnly: true}, {Path: scratch(t, dataDir, id)}},
			UID:        uid,
		}
	}

	// whatever becomes of the test, no machine is left running.
	var booted []string
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		for _, id := range booted {
			_ = h.Terminate(ctx, id)
		}
	})

	first, second := spec(machineID(t), testFirstUID), spec(machineID(t), testFirstUID+1)
	booted = append(booted, first.ID, second.ID)

	a, err := h.Boot(t.Context(), first)
	require.NoError(t, err)

	b, err := h.Boot(t.Context(), second)
	require.NoError(t, err)

	t.Run("a machine's agent comes up and runs its task, again and again", func(t *testing.T) {
		drive(t, a)
	})

	t.Run("a machine's firecracker runs as its user, with nothing more", func(t *testing.T) {
		for machine, uid := range map[vm.Machine]int{a: testFirstUID, b: testFirstUID + 1} {
			assert.True(t, machine.Running)
			assert.Equal(t, uid, machine.UID)

			status := processStatus(t, machine.PID)
			assert.Equal(t, fmt.Sprintf("%d\t%d\t%d\t%d", uid, uid, uid, uid), status["Uid"], "every one of its uids is its own")
			assert.Equal(t, fmt.Sprintf("%d\t%d\t%d\t%d", uid, uid, uid, uid), status["Gid"])
			assert.Equal(t, "0000000000000000", status["CapEff"])
			assert.Equal(t, "0000000000000000", status["CapPrm"])

			assert.Equal(t, namespaceOf(t, "/proc/"+strconv.Itoa(machine.PID)+"/ns/net"), namespaceOf(t, expectedNamespace(namespace)), "it runs in the machines' network namespace")

			if mode == ModeSystemd {
				assert.Equal(t, "1", status["NoNewPrivs"])
				assert.Equal(t, "0000000000000000", status["CapBnd"], "it can never be given a capability")
				assert.Equal(t, unitName(machine.ID), machine.Unit)

				memoryMax, err := os.ReadFile(filepath.Join(machine.Cgroup, "memory.max"))
				require.NoError(t, err)
				assert.Equal(t, strconv.Itoa(256<<20+64<<20), strings.TrimSpace(string(memoryMax)), "its guest's memory and the VMM's overhead")

				cpuMax, err := os.ReadFile(filepath.Join(machine.Cgroup, "cpu.max"))
				require.NoError(t, err)
				assert.Equal(t, "50000 100000", strings.TrimSpace(string(cpuMax)), "half of its CPU")
			} else {
				assert.Empty(t, machine.Unit)
				assert.Empty(t, machine.Cgroup)
			}
		}
	})

	t.Run("one machine's user cannot read another's directory or disks", func(t *testing.T) {
		otherRoot := layout.MachineRoot(dataDir, a.ID)

		for _, path := range []string{
			otherRoot,
			filepath.Join(otherRoot, driveName(1)),
			layout.Machine(dataDir, a.ID),
			layout.Console(dataDir, a.ID),
			layout.Scratch(dataDir, a.ID),
			layout.VsockSocket(dataDir, a.ID),
		} {
			output, err := runAs(testFirstUID+1, "/bin/cat", path)
			assert.Error(t, err, path)
			assert.Contains(t, output, "Permission denied", path)
		}

		output, err := runAs(testFirstUID, "/bin/ls", otherRoot)
		require.NoError(t, err, "a machine's user reads its own: %s", output)
		assert.Contains(t, output, driveName(1))
	})

	t.Run("a vmhost that starts again finds every machine where it left it", func(t *testing.T) {
		again, err := New(config, logger)
		require.NoError(t, err)
		defer again.Close()

		found, err := again.Machines(t.Context())
		require.NoError(t, err)

		byID := make(map[string]vm.Machine)
		for _, machine := range found {
			byID[machine.ID] = machine
		}

		assert.Equal(t, a, byID[a.ID])
		assert.Equal(t, b, byID[b.ID])
	})

	t.Run("a machine booted by a process that is gone is found and driven from another", func(t *testing.T) {
		other := spec(machineID(t), testFirstUID+2)
		booted = append(booted, other.ID)

		given, err := json.Marshal(handoff{Config: config, Spec: other})
		require.NoError(t, err)

		path := filepath.Join(t.TempDir(), "handoff.json")
		require.NoError(t, os.WriteFile(path, given, 0o600))

		self, err := os.Executable()
		require.NoError(t, err)

		command := exec.Command(self, "-test.run=^TestMicroVMBootForAnother$", "-test.count=1", "-test.v")
		command.Env = append(os.Environ(), "WORKLOAD_TEST_HANDOFF="+path)

		output, err := command.CombinedOutput()
		require.NoError(t, err, string(output))
		require.Contains(t, string(output), "--- PASS: TestMicroVMBootForAnother", string(output))

		again, err := New(config, logger)
		require.NoError(t, err)
		defer again.Close()

		machine, err := again.Machine(t.Context(), other.ID)
		require.NoError(t, err)
		require.True(t, machine.Running, "the machine outlived the process that booted it")

		drive(t, machine)

		require.NoError(t, again.Terminate(t.Context(), other.ID))
	})

	t.Run("a machine that powers off has ended, and is held until it is terminated", func(t *testing.T) {
		client := guest.NewClient(a.VsockPath)
		defer client.Close()

		require.NoError(t, client.PowerOff(t.Context()))

		require.Eventually(t, func() bool {
			machine, err := h.Machine(t.Context(), a.ID)

			return err == nil && !machine.Running
		}, 30*time.Second, 100*time.Millisecond, "its firecracker ends with it")

		console, err := os.ReadFile(layout.Console(dataDir, a.ID))
		require.NoError(t, err)
		assert.NotEmpty(t, console, "what it said is kept beside its directory")

		require.NoError(t, h.Terminate(t.Context(), a.ID))

		_, err = h.Machine(t.Context(), a.ID)
		assert.ErrorIs(t, err, vm.ErrNotFound)
	})

	t.Run("a machine terminated while it runs ends at once, and leaves nothing behind", func(t *testing.T) {
		require.NoError(t, h.Terminate(t.Context(), b.ID))

		assert.Eventually(t, func() bool { return syscall.Kill(b.PID, 0) == syscall.ESRCH }, 10*time.Second, 10*time.Millisecond, "its firecracker is gone")

		_, err := os.Stat(layout.Machine(dataDir, b.ID))
		assert.ErrorIs(t, err, os.ErrNotExist)

		machines, err := h.Machines(t.Context())
		require.NoError(t, err)
		assert.Empty(t, machines)

		_, err = os.Stat(layout.Scratch(dataDir, b.ID))
		assert.NoError(t, err, "its disks are vmhost's to let go of")
	})
}

// drive speaks to a machine's agent: it comes up, is told what it is, runs
// its task, writes on its scratch disk, takes commands beside it, is stopped
// and runs its task again.
func drive(t *testing.T, machine vm.Machine) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	client := guest.NewClient(machine.VsockPath)
	defer client.Close()

	require.NoError(t, client.Ready(ctx))

	require.NoError(t, client.Configure(ctx, guestProtocol.Config{
		Now:      time.Now(),
		Hostname: testHostname,
		Root:     guestProtocol.Root{Image: guestProtocol.ImageDevice, Scratch: guestProtocol.ScratchDevice},
	}))

	status, err := client.Start(ctx, guestProtocol.Process{Args: []string{"/bin/sh", "-c", "echo hi; echo oops >&2; echo written > /root/file; cat /root/file; exec sleep 600"}})
	require.NoError(t, err)
	assert.Equal(t, guestProtocol.StateRunning, status.State)

	var lines []guestProtocol.LogLine
	require.Eventually(t, func() bool {
		lines = nil
		_ = client.Logs(ctx, 0, false, func(line guestProtocol.LogLine) error {
			lines = append(lines, line)

			return nil
		})

		return len(lines) >= 3
	}, 30*time.Second, 100*time.Millisecond, "its task writes")

	written := make(map[string]string)
	for _, line := range lines {
		written[line.Content] = line.Stream
	}

	assert.Equal(t, map[string]string{"hi": guestProtocol.StreamStdout, "oops": guestProtocol.StreamStderr, "written": guestProtocol.StreamStdout}, written)

	stats, err := client.Stats(ctx)
	require.NoError(t, err)
	assert.Positive(t, stats.PIDs)

	id, conn, err := client.Exec(ctx, guestProtocol.Exec{Process: guestProtocol.Process{Args: []string{"/bin/sh", "-c", "read line; echo got $line; hostname; cat /root/file"}}})
	require.NoError(t, err)

	require.NoError(t, guestProtocol.WriteFrame(conn, guestProtocol.FrameStdin, []byte("ping\n")))
	require.NoError(t, guestProtocol.WriteFrame(conn, guestProtocol.FrameCloseStdin, nil))

	var output bytes.Buffer
	code := -1

	_ = conn.SetDeadline(time.Now().Add(time.Minute))
	for code < 0 {
		frame, err := guestProtocol.ReadFrame(conn)
		require.NoError(t, err)

		switch frame.Type {
		case guestProtocol.FrameStdout, guestProtocol.FrameStderr:
			output.Write(frame.Payload)
		case guestProtocol.FrameExit:
			code, err = guestProtocol.ParseExit(frame.Payload)
			require.NoError(t, err)
		}
	}

	conn.Close()

	assert.Equal(t, 0, code)
	assert.Equal(t, "got ping\n"+testHostname+"\nwritten\n", output.String(), "a command beside the task sees what it sees")

	_, err = client.EndExec(ctx, id, guestProtocol.EndExec{Grace: time.Second, KillGrace: time.Second})
	require.NoError(t, err)

	stopped, err := client.Stop(ctx, 2*time.Second)
	require.NoError(t, err)
	assert.Equal(t, guestProtocol.StateExited, stopped.State)
	assert.Equal(t, 128+int(syscall.SIGTERM), stopped.ExitCode, "it was told to go, and went")

	again, err := client.Start(ctx, guestProtocol.Process{Args: []string{"/bin/sh", "-c", "cat /root/file; exit 3"}})
	require.NoError(t, err)
	assert.Greater(t, again.Generation, status.Generation)

	ended, err := client.Wait(ctx, again.Generation)
	require.NoError(t, err)
	assert.Equal(t, 3, ended.ExitCode)
	assert.Equal(t, guestProtocol.StateExited, ended.State)
}

// processStatus is what /proc says of a process, by field.
func processStatus(t *testing.T, pid int) map[string]string {
	t.Helper()

	content, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	require.NoError(t, err)

	status := make(map[string]string)
	for line := range strings.Lines(string(content)) {
		name, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if found {
			status[name] = strings.TrimSpace(value)
		}
	}

	return status
}

// namespaceOf is which namespace a path names.
func namespaceOf(t *testing.T, path string) uint64 {
	t.Helper()

	var stat unix.Stat_t
	require.NoError(t, unix.Stat(path, &stat))

	return stat.Ino
}

// expectedNamespace is the network namespace machines run in: the one named,
// or the tests' own.
func expectedNamespace(namespace string) string {
	if len(namespace) == 0 {
		return "/proc/self/ns/net"
	}

	return namespace
}

// runAs runs a command as a machine's user, and says what it said.
func runAs(uid int, name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(uid), Groups: []uint32{}}}

	output, err := command.CombinedOutput()

	return string(output), err
}
