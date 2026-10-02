//go:build linux

package firecracker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vmm/layout"
)

// TestMain lets the test binary stand in for a machine's firecracker: started
// the way a machine's firecracker is, it takes an API on the socket it is
// given, and takes whatever it is told there.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--api-sock" {
		os.Exit(fakeFirecracker(os.Args[1:]))
	}

	os.Exit(m.Run())
}

// fakeFirecracker serves a fake firecracker API, writing every request it
// takes to api.log in the directory it runs in. A machine whose ID starts
// with dead ends before it takes its API; one whose ID starts with bad has its
// kernel refused.
func fakeFirecracker(args []string) int {
	var socket, id string
	for i := 0; i+1 < len(args); i += 2 {
		switch args[i] {
		case "--api-sock":
			socket = args[i+1]
		case "--id":
			id = args[i+1]
		}
	}

	fmt.Println("fake firecracker for", id)

	if strings.HasPrefix(id, "dead") {
		fmt.Println("boom: this firecracker never takes its API")

		return 1
	}

	log, err := os.OpenFile("api.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 1
	}

	api := &fakeAPI{}
	if strings.HasPrefix(id, "bad") {
		api.refuses = map[string]string{"/boot-source": "the kernel is not one"}
	}

	listener, err := net.Listen("unix", socket)
	if err != nil {
		return 1
	}

	// it never outlives the tests by long, whatever becomes of them.
	time.AfterFunc(2*time.Minute, func() { os.Exit(0) })

	_ = http.Serve(listener, http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		api.ServeHTTP(rw, r)

		calls := api.sent()
		_ = json.NewEncoder(log).Encode(calls[len(calls)-1])
	}))

	return 0
}

// bootExecName is what the fake firecrackers booted here are called, which is
// nobody else's.
const bootExecName = "fc-boot-test"

// booting builds a hypervisor in child mode, every machine running as vmhost
// itself, over a data directory of its own whose firecracker is the test
// binary, and a machine for it to boot from files it holds.
func booting(t *testing.T) (*Hypervisor, vm.MachineSpec) {
	t.Helper()

	dataDir := filepath.Join(shortDir(t), "vmhost")
	require.NoError(t, layout.Prepare(dataDir))

	self, err := os.Executable()
	require.NoError(t, err)

	binary, err := install(self, layout.Bin(dataDir))
	require.NoError(t, err)

	h := &Hypervisor{
		config:   Config{DataDir: dataDir, Binary: self, Mode: ModeChild},
		logger:   slog.New(slog.DiscardHandler),
		binary:   binary,
		execName: bootExecName,
		launcher: &children{dataDir: dataDir, execName: bootExecName},
	}

	s := files(t, dataDir)

	return h, s
}

// terminated terminates a machine when the test ends, whatever became of it.
func terminated(t *testing.T, h *Hypervisor, id string) {
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_ = h.Terminate(ctx, id)
	})
}

// told is what a fake firecracker was told.
func told(t *testing.T, h *Hypervisor, id string) []apiCall {
	t.Helper()

	log, err := os.Open(filepath.Join(h.rootDir(id), "api.log"))
	require.NoError(t, err)
	defer log.Close()

	var calls []apiCall

	scanner := bufio.NewScanner(log)
	for scanner.Scan() {
		var call apiCall
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &call))

		calls = append(calls, call)
	}

	return calls
}

func TestBoot(t *testing.T) {
	t.Parallel()

	t.Run("a machine is booted, found again, and terminated", func(t *testing.T) {
		t.Parallel()

		h, s := booting(t)
		terminated(t, h, s.ID)

		machine, err := h.Boot(t.Context(), s)
		require.NoError(t, err)

		assert.True(t, machine.Running)
		assert.Positive(t, machine.PID)
		assert.Equal(t, s.ID, machine.ID)
		assert.Equal(t, 0, machine.UID, "it runs as vmhost itself, as it was asked")
		assert.Equal(t, layout.MachineRoot(h.config.DataDir, s.ID), machine.Dir)
		assert.Equal(t, layout.VsockSocket(h.config.DataDir, s.ID), machine.VsockPath)
		assert.Empty(t, machine.Cgroup, "a child of vmhost has no cgroup of its own")
		assert.Empty(t, machine.Unit)

		calls := told(t, h, s.ID)
		require.Len(t, calls, 6, "its size, kernel, two disks, vsock, and the start")
		assert.Equal(t, "/machine-config", calls[0].Path)
		assert.Equal(t, apiCall{Method: http.MethodPut, Path: "/actions", Body: `{"action_type":"InstanceStart"}`}, calls[5])

		console, err := os.ReadFile(layout.Console(h.config.DataDir, s.ID))
		require.NoError(t, err)
		assert.Contains(t, string(console), "fake firecracker for "+s.ID, "what it says goes beside its directory")

		_, err = h.Boot(t.Context(), s)
		assert.ErrorIs(t, err, vm.ErrConflict, "a machine that runs is not booted again")

		// a vmhost that starts again finds what it left running.
		again := &Hypervisor{config: h.config, logger: h.logger, binary: h.binary, execName: bootExecName, launcher: &children{dataDir: h.config.DataDir, execName: bootExecName}}

		found, err := again.Machine(t.Context(), s.ID)
		require.NoError(t, err)
		assert.Equal(t, machine, found)

		machines, err := again.Machines(t.Context())
		require.NoError(t, err)
		assert.Equal(t, []vm.Machine{machine}, machines)

		require.NoError(t, again.Terminate(t.Context(), s.ID))

		// its firecracker is gone, once the vmhost that started it has
		// collected it.
		assert.Eventually(t, func() bool { return syscall.Kill(machine.PID, 0) == syscall.ESRCH }, 5*time.Second, 10*time.Millisecond, "its firecracker is gone")

		_, err = os.Stat(h.machineDir(s.ID))
		assert.ErrorIs(t, err, os.ErrNotExist, "and its directory with it")

		_, err = again.Machine(t.Context(), s.ID)
		assert.ErrorIs(t, err, vm.ErrNotFound)

		require.NoError(t, again.Terminate(t.Context(), s.ID), "a machine that is not there is the outcome asked for")

		_, err = os.Stat(s.Drives[1].Path)
		assert.NoError(t, err, "its disks are vmhost's to let go of")
	})

	t.Run("a machine whose firecracker ended is held until it is terminated, and is booted anew", func(t *testing.T) {
		t.Parallel()

		h, s := booting(t)
		terminated(t, h, s.ID)

		machine, err := h.Boot(t.Context(), s)
		require.NoError(t, err)

		require.NoError(t, h.launcher.stop(t.Context(), s.ID), "as a machine that powered off")

		ended, err := h.Machine(t.Context(), s.ID)
		require.NoError(t, err)
		assert.Equal(t, vm.Machine{ID: s.ID, VsockPath: machine.VsockPath, Dir: machine.Dir}, ended, "it ended, and has not gone")

		booted, err := h.Boot(t.Context(), s)
		require.NoError(t, err)
		assert.True(t, booted.Running)
		assert.NotEqual(t, machine.PID, booted.PID)
		assert.Len(t, told(t, h, s.ID), 6, "a machine booted again is made anew")
	})

	t.Run("a firecracker that ends before it is ready says why, and leaves nothing behind", func(t *testing.T) {
		t.Parallel()

		h, s := booting(t)
		s.ID = "dead0123456789ab"
		terminated(t, h, s.ID)

		_, err := h.Boot(t.Context(), s)

		assert.ErrorContains(t, err, "ended before it was ready")
		assert.ErrorContains(t, err, "boom: this firecracker never takes its API")

		_, err = os.Stat(h.machineDir(s.ID))
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("a machine firecracker refuses says why, and leaves nothing behind", func(t *testing.T) {
		t.Parallel()

		h, s := booting(t)
		s.ID = "bad0123456789abc"
		terminated(t, h, s.ID)

		_, err := h.Boot(t.Context(), s)

		assert.ErrorContains(t, err, "the machine's kernel was refused")
		assert.ErrorContains(t, err, "the kernel is not one")

		machines, err := h.Machines(t.Context())
		require.NoError(t, err)
		assert.Empty(t, machines, "its firecracker was ended, and its directory taken away")
	})

	t.Run("a machine that cannot be booted from what it names is not started at all", func(t *testing.T) {
		t.Parallel()

		h, s := booting(t)
		terminated(t, h, s.ID)

		outside := filepath.Join(t.TempDir(), "vmlinux")
		require.NoError(t, os.WriteFile(outside, nil, 0o644))
		s.Kernel = outside

		_, err := h.Boot(t.Context(), s)
		assert.ErrorIs(t, err, vm.ErrInvalid)

		_, err = os.Stat(h.machineDir(s.ID))
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("what is not a machine is refused before anything is made", func(t *testing.T) {
		t.Parallel()

		h, s := booting(t)
		s.ID = "../../etc"

		_, err := h.Boot(t.Context(), s)
		assert.ErrorIs(t, err, vm.ErrInvalid)

		_, err = h.Machine(t.Context(), s.ID)
		assert.ErrorIs(t, err, vm.ErrNotFound)

		assert.ErrorIs(t, h.Terminate(t.Context(), s.ID), vm.ErrInvalid)
	})
}

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("a child hypervisor installs its firecracker and runs machines in vmhost's own namespace", func(t *testing.T) {
		t.Parallel()

		dataDir := filepath.Join(shortDir(t), "vmhost")

		binary := filepath.Join(t.TempDir(), "firecracker")
		require.NoError(t, os.WriteFile(binary, []byte("#!/bin/sh\necho 'Firecracker v1.17.0'\necho\necho 'Supported snapshot data format versions: v1.0.0'\n"), 0o755))

		h, err := New(Config{DataDir: dataDir, Binary: binary, Mode: ModeChild, NetworkNamespace: "/proc/self/ns/net"}, slog.New(slog.DiscardHandler))
		require.NoError(t, err)
		defer h.Close()

		assert.Equal(t, "firecracker", h.Name())
		assert.Equal(t, "v1.17.0", h.Version())
		assert.Equal(t, layout.Bin(dataDir), filepath.Dir(h.binary), "it is run from the data directory")

		machines, err := h.Machines(t.Context())
		require.NoError(t, err)
		assert.Empty(t, machines)
	})

	t.Run("a child hypervisor refuses a namespace its children cannot be in", func(t *testing.T) {
		t.Parallel()

		other := filepath.Join(t.TempDir(), "netns")
		require.NoError(t, os.WriteFile(other, nil, 0o644))

		_, err := New(Config{DataDir: filepath.Join(shortDir(t), "vmhost"), Binary: "/bin/true", Mode: ModeChild, NetworkNamespace: other}, slog.New(slog.DiscardHandler))
		assert.ErrorContains(t, err, "vmhost's own network namespace")
	})

	t.Run("a firecracker that does not say which it is is of no version", func(t *testing.T) {
		t.Parallel()

		h, err := New(Config{DataDir: filepath.Join(shortDir(t), "vmhost"), Binary: "/bin/true", Mode: ModeChild}, slog.New(slog.DiscardHandler))
		require.NoError(t, err)

		assert.Empty(t, h.Version())
	})

	t.Run("a configuration that cannot work is refused before anything is made", func(t *testing.T) {
		t.Parallel()

		dataDir := filepath.Join(shortDir(t), "vmhost")

		_, err := New(Config{DataDir: dataDir, Binary: "/bin/true", Mode: "jailer"}, slog.New(slog.DiscardHandler))
		assert.Error(t, err)

		_, err = os.Stat(dataDir)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}
