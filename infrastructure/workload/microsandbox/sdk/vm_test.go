//go:build microsandbox

package sdk

// The tests in this file boot real microVMs, so they run only where
// microsandbox can: /dev/kvm opens for reading and writing, and the runtime
// resolves, as it does in the workload-microsandbox image through MSB_PATH and
// MSB_LIBKRUNFW_PATH. Anywhere else they skip.
//
// MICROSANDBOX_TEST_IMAGE is the image they boot, busybox:latest when unset;
// it needs sh, httpd, dd, stty and mount, as busybox has. The test of ports
// reached from another container runs only when MICROSANDBOX_TEST_BIND is this
// container's address on a network another container shares, and
// MICROSANDBOX_TEST_PROBE is the URL of a prober in that other container: it
// fetches http://<its query string> and answers with what it got.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/microsandbox/runs"
)

const helperEnv = "MICROSANDBOX_TEST_HELPER"

func TestMain(m *testing.M) {
	// the test binary is also the process that makes a sandbox and exits,
	// for the re-adoption test
	if name := os.Getenv(helperEnv); name != "" {
		os.Exit(helper(name))
	}

	os.Exit(m.Run())
}

var (
	setupOnce sync.Once
	setupErr  error
	usable    bool
)

// vm is the adapter when microsandbox can run here, with the test image
// pulled, and skips the test when it cannot.
func vm(t *testing.T) *Sandboxes {
	t.Helper()

	s := New(nil)
	setupOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if _, err := s.Check(ctx); err != nil {
			setupErr = err
			return
		}
		usable = true
		setupErr = s.Pull(ctx, testImage())
	})

	if !usable {
		t.Skipf("microsandbox cannot run here: %v", setupErr)
	}
	require.NoError(t, setupErr, "pull the test image")

	return s
}

func testImage() string {
	if image := os.Getenv("MICROSANDBOX_TEST_IMAGE"); image != "" {
		return image
	}

	return "busybox:latest"
}

var unsafeName = regexp.MustCompile(`[^a-z0-9]+`)

// spec is a public sandbox of 128 MiB, named for the test, removed when the
// test ends.
func spec(t *testing.T, s *Sandboxes) runs.SandboxSpec {
	t.Helper()

	suffix := make([]byte, 3)
	_, _ = rand.Read(suffix)
	name := strings.Trim(unsafeName.ReplaceAllString(strings.ToLower(t.Name()), "-"), "-")
	if len(name) > 40 {
		name = name[len(name)-40:]
	}
	name = "m2-" + name + "-" + hex.EncodeToString(suffix)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		assert.NoError(t, s.Remove(ctx, name))
	})

	return runs.SandboxSpec{
		Name:              name,
		Image:             testImage(),
		CPUs:              1,
		Memory:            128 << 20,
		Labels:            map[string]string{"m2.test": "sdk", "m2.name": name},
		Network:           "public",
		Nameservers:       []string{"1.1.1.1", "9.9.9.9"},
		MaxTCPConnections: 256,
	}
}

func timeout(t *testing.T, d time.Duration) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)

	return ctx
}

// ran is what a command did: its output and its last event.
type ran struct {
	stdout, stderr string
	last           runs.Event
	kinds          []runs.EventKind
}

// collect reads a process's events until they are closed.
func collect(t *testing.T, p runs.Process, within time.Duration) ran {
	t.Helper()

	var (
		out, errs bytes.Buffer
		result    ran
	)

	deadline := time.After(within)
	for {
		select {
		case event, ok := <-p.Events():
			if !ok {
				result.stdout, result.stderr = out.String(), errs.String()
				require.NotEmpty(t, result.kinds, "no events at all")
				return result
			}
			result.kinds = append(result.kinds, event.Kind)
			result.last = event
			switch event.Kind {
			case runs.EventStdout:
				out.Write(event.Data)
			case runs.EventStderr:
				errs.Write(event.Data)
			}
		case <-deadline:
			t.Fatalf("the command did not end within %v; output %q", within, out.String())
		}
	}
}

// run runs argv in sb and waits for it.
func run(t *testing.T, sb runs.Sandbox, command runs.Command) ran {
	t.Helper()

	p, err := sb.Exec(timeout(t, 30*time.Second), command)
	require.NoError(t, err)
	defer func() { assert.NoError(t, p.Close()) }()

	return collect(t, p, time.Minute)
}

func sh(t *testing.T, sb runs.Sandbox, script string) ran {
	t.Helper()

	return run(t, sb, runs.Command{Argv: []string{"sh", "-c", script}})
}

// waitFor reads a process's events until its stdout so far contains want.
func waitFor(t *testing.T, p runs.Process, out *bytes.Buffer, want string) {
	t.Helper()

	deadline := time.After(30 * time.Second)
	for !strings.Contains(out.String(), want) {
		select {
		case event, ok := <-p.Events():
			require.True(t, ok, "the events ended before %q; output %q", want, out.String())
			if event.Kind == runs.EventStdout || event.Kind == runs.EventStderr {
				out.Write(event.Data)
			}
		case <-deadline:
			t.Fatalf("no %q within 30s; output %q", want, out.String())
		}
	}
}

func find(t *testing.T, s *Sandboxes, name string) (runs.SandboxInfo, bool) {
	t.Helper()

	infos, err := s.List(timeout(t, 30*time.Second), map[string]string{"m2.name": name})
	require.NoError(t, err)
	require.LessOrEqual(t, len(infos), 1)

	if len(infos) == 0 {
		return runs.SandboxInfo{}, false
	}

	return infos[0], true
}

func TestCheckAgainstTheRuntime(t *testing.T) {
	s := vm(t)

	versions, err := s.Check(timeout(t, time.Minute))
	require.NoError(t, err)

	assert.Equal(t, "0.7.6", versions.SDK)
	assert.Equal(t, versions.SDK, versions.Runtime, "the SDK and the msb it drives are one version")
	assert.Equal(t, runtime.GOARCH, versions.Architecture)
}

func TestPullAndImage(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	require.NoError(t, s.Pull(ctx, testImage()), "a cached image is not pulled again")

	config, found, err := s.Image(ctx, testImage())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, runtime.GOARCH, config.Architecture)
	if testImage() == "busybox:latest" {
		assert.Equal(t, []string{"sh"}, config.Cmd)
		assert.Empty(t, config.Entrypoint)
	}

	missing := "docker.io/library/m2-sdk-no-such-image:1"
	_, found, err = s.Image(ctx, missing)
	require.NoError(t, err)
	assert.False(t, found)

	assert.ErrorContains(t, s.Pull(ctx, missing), missing)
	assert.Error(t, s.Pull(ctx, "--help"), "not an image")
}

func TestLifecycle(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	sp := spec(t, s)
	sp.CPUs = 1.5
	sp.Memory = 80<<20 + 1
	sp.Env = map[string]string{"FROM_SPEC": "spec"}
	sp.Workdir = "/tmp"

	sb, err := s.Create(ctx, sp)
	require.NoError(t, err)
	assert.Equal(t, sp.Name, sb.Name())

	got := run(t, sb, runs.Command{
		Argv: []string{"sh", "-c", `echo "$FROM_SPEC $FROM_EXEC"; pwd; nproc; grep MemTotal /proc/meminfo; echo kept > /root/kept`},
		Env:  []string{"FROM_EXEC=exec"},
	})
	assert.Equal(t, runs.Event{Kind: runs.EventExited, ExitCode: 0}, got.last)
	lines := strings.Split(strings.TrimSpace(got.stdout), "\n")
	require.Len(t, lines, 4, got.stdout)
	assert.Equal(t, "spec exec", lines[0], "the spec's environment, and the command's on top")
	assert.Equal(t, "/tmp", lines[1], "the spec's working directory")
	assert.Equal(t, "2", lines[2], "1.5 CPUs boot 2 vCPUs")
	memTotal := strings.Fields(lines[3])
	require.Len(t, memTotal, 3)
	kib, err := strconv.Atoi(memTotal[1])
	require.NoError(t, err)
	assert.InDelta(t, 81-24, kib/1024, 4, "81 MiB less what the guest kernel keeps")

	assert.Equal(t, "/\n", run(t, sb, runs.Command{Argv: []string{"pwd"}, Workdir: "/"}).stdout)

	info, found := find(t, s, sp.Name)
	require.True(t, found)
	assert.True(t, info.Running)
	for key, value := range sp.Labels {
		assert.Equal(t, value, info.Labels[key], key)
	}

	metrics, err := s.Metrics(ctx)
	require.NoError(t, err)
	require.Contains(t, metrics, sp.Name)
	assert.Equal(t, uint64(81<<20), metrics[sp.Name].MemoryLimit)
	assert.Positive(t, metrics[sp.Name].MemoryUsage)

	require.NoError(t, sb.Close())
	require.NoError(t, s.Stop(ctx, sp.Name, 10*time.Second))
	info, found = find(t, s, sp.Name)
	require.True(t, found)
	assert.False(t, info.Running)
	assert.NoError(t, s.Stop(ctx, sp.Name, 10*time.Second), "stopping a stopped sandbox")

	_, err = s.Connect(ctx, sp.Name)
	assert.Error(t, err, "nothing to connect to while it is stopped")

	sb, err = s.Start(ctx, sp.Name)
	require.NoError(t, err)
	got = sh(t, sb, "cat /root/kept; echo $FROM_SPEC")
	assert.Equal(t, "kept\nspec\n", got.stdout, "the root and the environment outlive a stop")
	require.NoError(t, sb.Close())

	require.NoError(t, s.Remove(ctx, sp.Name), "removing a running sandbox")
	_, found = find(t, s, sp.Name)
	assert.False(t, found)
	assert.NoError(t, s.Remove(ctx, sp.Name), "removing what is gone")
	assert.NoError(t, s.Stop(ctx, sp.Name, time.Second), "stopping what is gone")

	_, err = s.Start(ctx, sp.Name)
	assert.Error(t, err)
	_, err = s.Connect(ctx, sp.Name)
	assert.Error(t, err)
}

func TestStopKillsWhatDoesNotStopInTime(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 2*time.Minute)

	sp := spec(t, s)
	sb, err := s.Create(ctx, sp)
	require.NoError(t, err)
	defer sb.Close()

	begun := time.Now()
	require.NoError(t, s.Stop(ctx, sp.Name, 0), "no time at all is a kill")
	assert.Less(t, time.Since(begun), 15*time.Second)

	info, found := find(t, s, sp.Name)
	require.True(t, found)
	assert.False(t, info.Running)
}

func TestCreateThatFailsLeavesNothing(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	sp := spec(t, s)
	sp.Memory = 32 << 20 // too little for the guest to boot in

	_, err := s.Create(ctx, sp)
	require.Error(t, err)
	_, found := find(t, s, sp.Name)
	assert.False(t, found, "the failed boot was removed")

	sp.Memory = 128 << 20
	sb, err := s.Create(ctx, sp)
	require.NoError(t, err, "so its name can be used again")
	defer sb.Close()

	_, err = s.Create(ctx, sp)
	require.Error(t, err, "a name in use")
	info, found := find(t, s, sp.Name)
	require.True(t, found)
	assert.True(t, info.Running, "and the sandbox using it is left alone")
}

func TestExitsAndFailures(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	sb, err := s.Create(ctx, spec(t, s))
	require.NoError(t, err)
	defer sb.Close()

	require.Equal(t, 0, sh(t, sb, "printf '\\001garbage' > /tmp/garbage; chmod 755 /tmp/garbage").last.ExitCode)

	t.Run("an exit status is handed on", func(t *testing.T) {
		got := sh(t, sb, "echo out; echo err >&2; exit 7")
		assert.Equal(t, runs.Event{Kind: runs.EventExited, ExitCode: 7}, got.last)
		assert.Equal(t, "out\n", got.stdout)
		assert.Equal(t, "err\n", got.stderr)
		// stdout and stderr are two pipes, read in whatever order they fill
		assert.Equal(t, runs.EventStarted, got.kinds[0])
		assert.ElementsMatch(t, []runs.EventKind{runs.EventStarted, runs.EventStdout, runs.EventStderr, runs.EventExited}, got.kinds)
	})

	t.Run("a signal's exit is -1", func(t *testing.T) {
		assert.Equal(t, -1, sh(t, sb, "kill -SEGV $$").last.ExitCode)

		p, err := sb.Exec(ctx, runs.Command{Argv: []string{"sleep", "300"}})
		require.NoError(t, err)
		defer p.Close()
		require.Equal(t, runs.EventStarted, (<-p.Events()).Kind)
		require.NoError(t, p.Signal(ctx, syscall.SIGTERM))
		assert.Equal(t, runs.Event{Kind: runs.EventExited, ExitCode: -1}, collect(t, p, 30*time.Second).last)
	})

	for name, tt := range map[string]struct {
		argv  []string
		errno string
	}{
		"a program that is not there":              {argv: []string{"/no/such/program"}, errno: "ENOENT"},
		"a program that is not on the PATH":        {argv: []string{"no-such-program"}, errno: "ENOENT"},
		"a file that is not executable":            {argv: []string{"/etc/passwd"}, errno: "EACCES"},
		"a directory":                              {argv: []string{"/tmp"}, errno: "EACCES"},
		"a file that is no program the guest runs": {argv: []string{"/tmp/garbage"}, errno: "ENOEXEC"},
	} {
		t.Run(name+" fails to start", func(t *testing.T) {
			got := run(t, sb, runs.Command{Argv: tt.argv})
			assert.Equal(t, []runs.EventKind{runs.EventFailed}, got.kinds)
			assert.Equal(t, tt.errno, got.last.Errno)
			assert.NotEmpty(t, got.last.Message)
		})
	}
}

func TestTerminal(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	sb, err := s.Create(ctx, spec(t, s))
	require.NoError(t, err)
	defer sb.Close()

	t.Run("it starts at the size asked for and resizes", func(t *testing.T) {
		p, err := sb.Exec(ctx, runs.Command{
			Argv: []string{"sh", "-c", "read a; stty size; read b; stty size; echo TERM=$TERM"},
			TTY:  true, Stdin: true, Rows: 40, Cols: 100,
		})
		require.NoError(t, err)
		defer p.Close()

		// the terminal is sized by the time the start is handed on
		require.Equal(t, runs.EventStarted, (<-p.Events()).Kind)

		var out bytes.Buffer
		require.NotNil(t, p.Stdin())
		_, err = p.Stdin().Write([]byte("\n"))
		require.NoError(t, err)
		waitFor(t, p, &out, "40 100")

		require.NoError(t, p.Resize(ctx, 50, 132))
		_, err = p.Stdin().Write([]byte("\n"))
		require.NoError(t, err)
		waitFor(t, p, &out, "50 132")

		got := collect(t, p, 30*time.Second)
		assert.Equal(t, runs.Event{Kind: runs.EventExited, ExitCode: 0}, got.last)
		assert.Contains(t, out.String()+got.stdout, "TERM=xterm")
	})

	t.Run("without a size it is microsandbox's 24 by 80", func(t *testing.T) {
		got := run(t, sb, runs.Command{Argv: []string{"stty", "size"}, TTY: true})
		assert.Equal(t, "24 80\r\n", got.stdout)
	})

	t.Run("stdout and stderr are one stream", func(t *testing.T) {
		got := run(t, sb, runs.Command{Argv: []string{"sh", "-c", "echo out; echo err >&2"}, TTY: true})
		assert.Equal(t, "out\r\nerr\r\n", got.stdout)
		assert.Empty(t, got.stderr)
	})
}

func TestStdin(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	sb, err := s.Create(ctx, spec(t, s))
	require.NoError(t, err)
	defer sb.Close()

	t.Run("writes from several goroutines all arrive, while the events are read", func(t *testing.T) {
		p, err := sb.Exec(ctx, runs.Command{Argv: []string{"cat"}, Stdin: true})
		require.NoError(t, err)
		defer p.Close()

		const writers, lines = 4, 200
		var wg sync.WaitGroup
		for w := range writers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := range lines {
					_, err := fmt.Fprintf(p.Stdin(), "w%d-%03d\n", w, i)
					assert.NoError(t, err)
				}
			}()
		}

		go func() {
			wg.Wait()
			assert.NoError(t, p.Stdin().Close(), "end of input ends cat")
		}()

		got := collect(t, p, time.Minute)
		assert.Equal(t, runs.Event{Kind: runs.EventExited, ExitCode: 0}, got.last)

		next := make([]int, writers)
		scanner := bufio.NewScanner(strings.NewReader(got.stdout))
		for scanner.Scan() {
			var w, i int
			_, err := fmt.Sscanf(scanner.Text(), "w%d-%d", &w, &i)
			require.NoError(t, err, scanner.Text())
			require.Equal(t, next[w], i, "each writer's lines arrive whole and in order")
			next[w]++
		}
		assert.Equal(t, []int{lines, lines, lines, lines}, next)
	})

	t.Run("a command without stdin reads an empty one, as with docker", func(t *testing.T) {
		p, err := sb.Exec(ctx, runs.Command{Argv: []string{"cat"}})
		require.NoError(t, err)
		defer p.Close()

		assert.Nil(t, p.Stdin())
		assert.Equal(t, runs.Event{Kind: runs.EventExited, ExitCode: 0}, collect(t, p, 30*time.Second).last, "cat ends at once")
	})

	t.Run("a terminal without stdin is silent, as with docker", func(t *testing.T) {
		p, err := sb.Exec(ctx, runs.Command{Argv: []string{"sh", "-c", "read line; echo read-something"}, TTY: true})
		require.NoError(t, err)
		require.Equal(t, runs.EventStarted, (<-p.Events()).Kind)
		assert.Nil(t, p.Stdin())

		select {
		case event := <-p.Events():
			t.Fatalf("it should still be waiting for input, got %v %q", event.Kind, event.Data)
		case <-time.After(2 * time.Second):
		}

		require.NoError(t, p.Close())
	})
}

func TestSignals(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	sb, err := s.Create(ctx, spec(t, s))
	require.NoError(t, err)
	defer sb.Close()

	p, err := sb.Exec(ctx, runs.Command{Argv: []string{"sh", "-c", `
		trap 'echo got-INT' INT
		trap 'echo got-TERM; exit 3' TERM
		sleep 300 &
		echo ready
		while :; do sleep 0.1; done`}})
	require.NoError(t, err)
	defer p.Close()

	var out bytes.Buffer
	waitFor(t, p, &out, "ready")

	require.NoError(t, p.Signal(ctx, syscall.SIGINT))
	waitFor(t, p, &out, "got-INT")

	require.NoError(t, p.Signal(ctx, syscall.SIGTERM))
	got := collect(t, p, 30*time.Second)
	assert.Contains(t, out.String()+got.stdout, "got-TERM")
	assert.Equal(t, runs.Event{Kind: runs.EventExited, ExitCode: 3}, got.last)

	assert.Equal(t, "", sh(t, sb, "ps | grep 'sleep 30[0]' || true").stdout, "the signals reached the command's process group")
}

// beat starts a main process that writes the time to /tmp/beat each second.
func beat(t *testing.T, sb runs.Sandbox) runs.Process {
	t.Helper()

	p, err := sb.Exec(timeout(t, 30*time.Second), runs.Command{Argv: []string{"sh", "-c", "while :; do date +%s > /tmp/beat; sleep 1; done"}})
	require.NoError(t, err)
	require.Equal(t, runs.EventStarted, (<-p.Events()).Kind)

	return p
}

// beating is whether a sandbox's beat goes on over the next few seconds.
func beating(t *testing.T, sb runs.Sandbox) bool {
	t.Helper()

	first := sh(t, sb, "sleep 1; cat /tmp/beat").stdout
	time.Sleep(2500 * time.Millisecond)
	second := sh(t, sb, "cat /tmp/beat").stdout

	return first != second
}

func TestClose(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	sp := spec(t, s)
	sb, err := s.Create(ctx, sp)
	require.NoError(t, err)

	t.Run("closing a process ends it, and its exit still comes last", func(t *testing.T) {
		p := beat(t, sb)
		require.True(t, beating(t, sb))

		require.NoError(t, p.Close())
		var last runs.Event
		for event := range p.Events() {
			last = event
		}
		assert.Equal(t, runs.Event{Kind: runs.EventExited, ExitCode: -1}, last)
		assert.False(t, beating(t, sb))
	})

	t.Run("closing a sandbox leaves its VM and its commands running", func(t *testing.T) {
		p := beat(t, sb)
		require.NoError(t, sb.Close())

		other, err := s.Connect(ctx, sp.Name)
		require.NoError(t, err)
		defer other.Close()
		assert.True(t, beating(t, other))

		// and the command is still the process's to signal
		require.NoError(t, p.Signal(ctx, syscall.SIGKILL))
		assert.Equal(t, runs.Event{Kind: runs.EventExited, ExitCode: -1}, collect(t, p, 30*time.Second).last)
		require.NoError(t, p.Close())
		assert.False(t, beating(t, other))
	})
}

// machine is the msb machine process that runs a sandbox's VM.
func machine(t *testing.T, name string) int {
	t.Helper()

	pid := machinePID(name)
	require.NotZero(t, pid, "no msb machine runs %s", name)

	return pid
}

// zombies are this process's children that have ended and not been reaped.
func zombies(t *testing.T) []string {
	t.Helper()

	var found []string
	dirs, _ := filepath.Glob("/proc/[0-9]*")
	for _, dir := range dirs {
		stat, err := os.ReadFile(dir + "/stat")
		if err != nil {
			continue
		}
		// pid (comm) state ppid ...
		fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))
		if len(fields) > 1 && fields[0] == "Z" && fields[1] == strconv.Itoa(os.Getpid()) {
			found = append(found, string(stat[:bytes.LastIndexByte(stat, ')')+1]))
		}
	}

	return found
}

func TestWhenTheVMDies(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	sp := spec(t, s)
	sb, err := s.Create(ctx, sp)
	require.NoError(t, err)
	defer sb.Close()

	p := beat(t, sb)
	defer p.Close()

	require.NoError(t, syscall.Kill(machine(t, sp.Name), syscall.SIGKILL))

	got := collect(t, p, time.Minute)
	assert.Equal(t, runs.EventLost, got.last.Kind, "its stream ended without an exit")
	t.Logf("lost: %q", got.last.Message)

	require.Eventually(t, func() bool {
		info, found := find(t, s, sp.Name)
		return found && !info.Running
	}, 30*time.Second, 500*time.Millisecond, "the dead VM is not running")

	again, err := s.Start(ctx, sp.Name)
	require.NoError(t, err, "a crashed sandbox starts again")
	defer again.Close()
	assert.Equal(t, runs.Event{Kind: runs.EventExited, ExitCode: 0}, run(t, again, runs.Command{Argv: []string{"true"}}).last)
	require.NoError(t, s.Stop(ctx, sp.Name, 10*time.Second))

	assert.Empty(t, zombies(t), "VMs that ended are reaped")
}

// helper is the process that makes sandboxes, each with a file on its disk
// and a main process running, and exits without letting go of any. It prints
// the PID of each one's VM, and READY.
func helper(names string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	s := New(nil)
	for _, name := range strings.Split(names, ",") {
		sb, err := s.Create(ctx, runs.SandboxSpec{
			Name:    name,
			Image:   testImage(),
			Memory:  128 << 20,
			Labels:  map[string]string{"m2.test": "sdk", "m2.name": name},
			Network: "public",
		})
		if err != nil {
			fmt.Println("create:", err)
			return 1
		}

		// synced, since a VM that is killed loses what its guest had not
		// written out yet, as a machine does when its power is cut
		kept, err := sb.Exec(ctx, runs.Command{Argv: []string{"sh", "-c", "echo kept > /root/kept && sync"}})
		if err != nil {
			fmt.Println("exec:", err)
			return 1
		}
		for range kept.Events() {
		}

		p, err := sb.Exec(ctx, runs.Command{Argv: []string{"sh", "-c", "while :; do date +%s > /tmp/beat; sleep 1; done"}})
		if err != nil {
			fmt.Println("exec:", err)
			return 1
		}

		if event := <-p.Events(); event.Kind != runs.EventStarted {
			fmt.Println("not started:", event)
			return 1
		}

		fmt.Printf("SANDBOX %s %d\n", name, machinePID(name))
	}

	time.Sleep(2 * time.Second)
	fmt.Println("READY")

	return 0
}

// machinePID is the PID of the msb machine that runs a sandbox's VM, or 0.
func machinePID(name string) int {
	dirs, _ := filepath.Glob("/proc/[0-9]*")
	for _, dir := range dirs {
		cmdline, err := os.ReadFile(dir + "/cmdline")
		if err != nil {
			continue
		}
		argv := strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")
		if len(argv) < 2 || argv[1] != "machine" {
			continue
		}
		for i := range len(argv) - 1 {
			if argv[i] == "--name" && argv[i+1] == name {
				pid, _ := strconv.Atoi(filepath.Base(dir))
				return pid
			}
		}
	}

	return 0
}

func TestReadoptionAfterTheProcessRestarts(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	sp := spec(t, s) // only its name, and its removal

	maker := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
	maker.Env = append(os.Environ(), helperEnv+"="+sp.Name)
	out, err := maker.CombinedOutput()
	require.NoError(t, err, string(out))
	require.Contains(t, string(out), "READY")

	infos, err := s.List(ctx, map[string]string{"m2.test": "sdk", "m2.name": sp.Name})
	require.NoError(t, err)
	require.Len(t, infos, 1)
	assert.True(t, infos[0].Running, "the VM outlived the process that made it")
	assert.Equal(t, sp.Name, infos[0].Labels["m2.name"])

	sb, err := s.Connect(ctx, sp.Name)
	require.NoError(t, err)
	assert.False(t, beating(t, sb), "its main process did not: it ended with its maker")

	got := sh(t, sb, "echo adopted")
	assert.Equal(t, "adopted\n", got.stdout)
	require.NoError(t, sb.Close())

	require.NoError(t, s.Stop(ctx, sp.Name, 10*time.Second))
	sb, err = s.Start(ctx, sp.Name)
	require.NoError(t, err)
	defer sb.Close()
	assert.Equal(t, "again\n", sh(t, sb, "echo again").stdout)
}

func TestMetrics(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	sp := spec(t, s)
	sb, err := s.Create(ctx, sp)
	require.NoError(t, err)
	defer sb.Close()

	usage := func() uint64 {
		metrics, err := s.Metrics(ctx)
		require.NoError(t, err)
		require.Contains(t, metrics, sp.Name)
		assert.Equal(t, uint64(128<<20), metrics[sp.Name].MemoryLimit)
		return metrics[sp.Name].MemoryUsage
	}

	idle := usage()
	assert.Positive(t, idle)
	assert.Less(t, idle, uint64(128<<20))

	require.Equal(t, 0, sh(t, sb, "mkdir -p /mnt/fill && mount -t tmpfs -o size=96m tmpfs /mnt/fill && dd if=/dev/zero of=/mnt/fill/f bs=1M count=64 2>/dev/null").last.ExitCode)
	require.Eventually(t, func() bool { return usage() >= idle+56<<20 }, 15*time.Second, 500*time.Millisecond,
		"64 MiB more in use in the guest shows as usage")

	require.Equal(t, 0, sh(t, sb, "rm /mnt/fill/f").last.ExitCode)
	require.Eventually(t, func() bool { return usage() < idle+16<<20 }, 15*time.Second, 500*time.Millisecond,
		"and goes again when the guest frees it")
}

func TestDiskLimit(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	free := func(sb runs.Sandbox) uint64 {
		got := sh(t, sb, "df -k / | tail -1")
		fields := strings.Fields(got.stdout)
		require.Len(t, fields, 6, got.stdout)
		kib, err := strconv.ParseUint(fields[3], 10, 64)
		require.NoError(t, err)
		return kib << 10
	}

	t.Run("a small limit holds what it says and no more", func(t *testing.T) {
		sp := spec(t, s)
		sp.Disk = 64 << 20

		sb, err := s.Create(ctx, sp)
		require.NoError(t, err)
		defer sb.Close()

		assert.GreaterOrEqual(t, free(sb), uint64(64<<20))
		assert.Equal(t, 0, sh(t, sb, "dd if=/dev/zero of=/fill bs=1M count=64 2>&1").last.ExitCode, "64 MiB fit")

		got := sh(t, sb, "dd if=/dev/zero of=/fill2 bs=1M count=64 2>&1; sync")
		assert.Contains(t, got.stdout, "No space left on device", "64 MiB more do not")
	})

	t.Run("a large limit holds what it says", func(t *testing.T) {
		sp := spec(t, s)
		sp.Disk = 2 << 30

		sb, err := s.Create(ctx, sp)
		require.NoError(t, err)
		defer sb.Close()

		assert.GreaterOrEqual(t, free(sb), uint64(2<<30))
	})
}

func TestPortsFromAnotherContainer(t *testing.T) {
	bind, probe := os.Getenv("MICROSANDBOX_TEST_BIND"), os.Getenv("MICROSANDBOX_TEST_PROBE")
	if bind == "" || probe == "" {
		t.Skip("MICROSANDBOX_TEST_BIND and MICROSANDBOX_TEST_PROBE name no other container")
	}

	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	for i, network := range []string{"public", "isolated"} {
		t.Run(network, func(t *testing.T) {
			hostPort := uint16(20180 + i)

			sp := spec(t, s)
			sp.Network = network
			sp.Ports = []runs.PortBinding{{Bind: bind, HostPort: hostPort, GuestPort: 8080}}

			sb, err := s.Create(ctx, sp)
			require.NoError(t, err)
			defer sb.Close()

			server, err := sb.Exec(ctx, runs.Command{Argv: []string{"sh", "-c", "mkdir -p /www && echo hello-" + network + " > /www/index.html && exec httpd -f -p 8080 -h /www"}})
			require.NoError(t, err)
			defer server.Close()
			require.Equal(t, runs.EventStarted, (<-server.Events()).Kind)

			var body string
			require.Eventually(t, func() bool {
				response, err := http.Get(fmt.Sprintf("%s?%s:%d/", probe, bind, hostPort))
				if err != nil {
					return false
				}
				defer response.Body.Close()
				b, _ := io.ReadAll(response.Body)
				body = string(b)
				return strings.Contains(body, "hello-"+network)
			}, 30*time.Second, time.Second, "the other container fetched %q", body)
		})
	}
}

// Guests of some sizes do not boot, and memoryMiB raises those past it: every
// size around them has to boot, with as many vCPUs as tasks are given.
func TestMemorySizesThatBoot(t *testing.T) {
	s := vm(t)
	ctx := timeout(t, 15*time.Minute)

	var (
		mu     sync.Mutex
		failed []string
		wg     sync.WaitGroup
		slots  = make(chan struct{}, 4)
	)

	for _, cpus := range []float64{1, 4, 8} {
		for _, size := range []uint64{64, 88, 91, 92, 96, 100, 104, 108, 112, 116, 120, 124, 128, 136, 160, 256} {
			sp := spec(t, s)
			sp.CPUs, sp.Memory = cpus, size<<20

			wg.Add(1)
			go func() {
				defer wg.Done()
				slots <- struct{}{}
				defer func() { <-slots }()

				sb, err := s.Create(ctx, sp)
				if err != nil {
					mu.Lock()
					failed = append(failed, fmt.Sprintf("%v vCPUs, %d MiB: %v", cpus, size, err))
					mu.Unlock()
					return
				}

				assert.NoError(t, sb.Close())
				assert.NoError(t, s.Remove(ctx, sp.Name))
			}()
		}
	}

	wg.Wait()
	assert.Empty(t, failed)
}

// The container restarts while sandboxes run: every VM dies with it, and the
// sandboxes are left as microsandbox last recorded them, running. In the new
// container, a fresh process starts each again, with its disk as it was. The
// driver runs helper in the first container and this in the second, and names
// in MICROSANDBOX_TEST_RESTARTED each sandbox and the PID its VM had, as
// name:pid,name:pid.
//
// The new container's PIDs start over, and microsandbox 0.7.6 judges a VM by
// its PID alone when it starts a sandbox again (issue #1642): while the PID
// the old VM had is another process's or thread's, it refuses, and, for a
// thread, its stop and destroy fail too. Nothing in the SDK clears that, so
// for those this only records what happens, and that nothing it does signals
// the process that has the PID now.
func TestStartAfterTheContainerRestarted(t *testing.T) {
	restarted := os.Getenv("MICROSANDBOX_TEST_RESTARTED")
	if restarted == "" {
		t.Skip("MICROSANDBOX_TEST_RESTARTED names no sandbox from before a restart")
	}

	s := vm(t)
	ctx := timeout(t, 5*time.Minute)

	type before struct {
		name string
		pid  int
	}
	var sandboxes []before
	for _, entry := range strings.Split(restarted, ",") {
		name, pid, ok := strings.Cut(entry, ":")
		require.True(t, ok, entry)
		n, err := strconv.Atoi(pid)
		require.NoError(t, err)
		sandboxes = append(sandboxes, before{name: name, pid: n})
	}
	require.NotEmpty(t, sandboxes)

	// one of them has its old PID taken by another process of this
	// container, as any of the service's could take it
	occupant := occupy(t, sandboxes[0].pid)
	if occupant != nil {
		defer func() {
			_ = occupant.Process.Kill()
			_ = occupant.Wait()
		}()
	}

	started := 0
	for _, sandbox := range sandboxes {
		holder := holderOf(sandbox.pid)
		info, found := find(t, s, sandbox.name)
		require.True(t, found, sandbox.name)

		if holder == "" {
			assert.False(t, info.Running, "%s: no VM has its old PID, so it is not running", sandbox.name)

			sb, err := s.Start(ctx, sandbox.name)
			require.NoError(t, err, "start %s", sandbox.name)
			assert.Equal(t, "kept\n", sh(t, sb, "cat /root/kept").stdout, "%s's disk is as it was", sandbox.name)
			assert.NoError(t, sb.Close())
			assert.NoError(t, s.Remove(ctx, sandbox.name))
			started++

			continue
		}

		_, startErr := s.Start(ctx, sandbox.name)
		stopErr := s.Stop(ctx, sandbox.name, 5*time.Second)
		removeErr := s.Remove(ctx, sandbox.name)
		t.Logf("%s: its VM's old PID %d is %s now; running: %v; start: %v; stop: %v; remove: %v",
			sandbox.name, sandbox.pid, holder, info.Running, startErr, stopErr, removeErr)
	}

	if occupant != nil {
		assert.NoError(t, occupant.Process.Signal(syscall.Signal(0)), "the process that has an old PID was not signalled")
	}
	t.Logf("%d of %d sandboxes started again", started, len(sandboxes))
}

// holderOf is what has pid in this container now, or "" when nothing does.
func holderOf(pid int) string {
	if _, err := os.Stat(fmt.Sprintf("/proc/self/task/%d", pid)); err == nil {
		return "a thread of this process"
	}

	if syscall.Kill(pid, 0) != nil {
		return ""
	}

	cmdline, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	return fmt.Sprintf("process %q", strings.ReplaceAll(strings.TrimRight(string(cmdline), "\x00"), "\x00", " "))
}

// occupy starts processes until one has pid, and is that one, or nil when
// pid is taken already or the PIDs have passed it.
func occupy(t *testing.T, pid int) *exec.Cmd {
	t.Helper()

	if syscall.Kill(pid, 0) == nil {
		return nil
	}

	for range 100000 {
		cmd := exec.Command("sleep", "600")
		require.NoError(t, cmd.Start())
		if cmd.Process.Pid == pid {
			return cmd
		}

		_ = cmd.Process.Kill()
		_ = cmd.Wait()

		if cmd.Process.Pid > pid && cmd.Process.Pid < pid+1000 {
			return nil
		}
	}

	return nil
}
