//go:build linux

package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/khanzadimahdi/testproject/domain/workload/guest"
)

// The agent's handlers are served here on TCP, by a test that is nobody's
// init, against a machine that only remembers what it was asked. The task's
// root is the test's own ("/", so nothing is chrooted), and what the agent
// starts is kept in process groups — and, when the test runs as root with a
// cgroup2 it may write to, in cgroups too, each case run in both.

func TestMain(m *testing.M) {
	// what a task leaves behind comes back to the test, as it comes back to
	// init in a machine, and is collected the same way.
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		fmt.Fprintln(os.Stderr, "the test cannot collect what its processes leave behind:", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// fakeMachine remembers what the agent asked of the machine, and does none
// of it.
type fakeMachine struct {
	lock sync.Mutex

	clock      time.Time
	root       guest.Root
	interfaces []guest.Interface
	hostname   string
	finished   bool
	readOnly   bool
	asked      []string

	mountErr error

	poweredOff chan struct{}
}

var _ machine = (*fakeMachine)(nil)

func newFakeMachine() *fakeMachine {
	return &fakeMachine{poweredOff: make(chan struct{})}
}

func (m *fakeMachine) note(step string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.asked = append(m.asked, step)
}

func (m *fakeMachine) steps() []string {
	m.lock.Lock()
	defer m.lock.Unlock()

	return append([]string(nil), m.asked...)
}

// seen is what the machine was last told, read under its lock: the handler
// that told it ran in another goroutine.
type seen struct {
	clock      time.Time
	root       guest.Root
	interfaces []guest.Interface
	hostname   string
	finished   bool
	readOnly   bool
}

func (m *fakeMachine) seen() seen {
	m.lock.Lock()
	defer m.lock.Unlock()

	return seen{clock: m.clock, root: m.root, interfaces: m.interfaces, hostname: m.hostname, finished: m.finished, readOnly: m.readOnly}
}

// failMounts makes every root the machine is asked to mount fail with err.
func (m *fakeMachine) failMounts(err error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.mountErr = err
}

func (m *fakeMachine) setClock(now time.Time) error {
	m.note("clock")

	m.lock.Lock()
	defer m.lock.Unlock()

	m.clock = now

	return nil
}

func (m *fakeMachine) mountRoot(root guest.Root) error {
	m.note("root")

	m.lock.Lock()
	defer m.lock.Unlock()

	m.root = root

	return m.mountErr
}

func (m *fakeMachine) configureInterfaces(interfaces []guest.Interface) error {
	m.note("interfaces")

	m.lock.Lock()
	defer m.lock.Unlock()

	m.interfaces = interfaces

	return nil
}

func (m *fakeMachine) setHostname(hostname string) error {
	m.note("hostname")

	m.lock.Lock()
	defer m.lock.Unlock()

	m.hostname = hostname

	return nil
}

func (m *fakeMachine) finishRoot(readOnly bool) error {
	m.note("finish")

	m.lock.Lock()
	defer m.lock.Unlock()

	m.finished, m.readOnly = true, readOnly

	return nil
}

func (m *fakeMachine) powerOff() {
	m.note("power off")
	close(m.poweredOff)
}

// testAgent is an agent served on TCP.
type testAgent struct {
	*Agent

	machine *fakeMachine
	server  *httptest.Server
	files   string
}

func newTestAgent(t *testing.T, confine confinement) *testAgent {
	t.Helper()

	files := t.TempDir()
	m := newFakeMachine()

	a := newAgent(slog.New(slog.NewTextHandler(io.Discard, nil)), m, dirs{root: "/", files: files}, confine)
	server := httptest.NewServer(a.routes())

	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()

		// whatever the test left running goes with it.
		_ = confine.remove("", emptyTimeout)
	})

	return &testAgent{Agent: a, machine: m, server: server, files: files}
}

// testConfig is a machine on one network, its own address on loopback so that
// its ports can be dialled.
func testConfig() guest.Config {
	return guest.Config{
		Now:         time.Now(),
		Hostname:    "task-xkfqz",
		Root:        guest.Root{Image: guest.ImageDevice, Scratch: guest.ScratchDevice},
		Interfaces:  []guest.Interface{{MAC: "06:00:7f:00:00:01", Address: "127.0.0.1/8"}},
		Nameservers: []string{"1.1.1.1"},
	}
}

// configured is a test agent that has been told what it is.
func configured(t *testing.T, confine confinement) *testAgent {
	t.Helper()

	a := newTestAgent(t, confine)

	response := a.call(t, http.MethodPut, "/config", testConfig(), nil)
	require.Equal(t, http.StatusNoContent, response.StatusCode)

	return a
}

// call makes one request and decodes its answer into out, when there is one.
func (a *testAgent) call(t *testing.T, method string, path string, body any, out any) *http.Response {
	t.Helper()

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)

		payload = bytes.NewReader(encoded)
	}

	request, err := http.NewRequest(method, a.server.URL+path, payload)
	require.NoError(t, err)

	response, err := a.server.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()

	if out != nil && response.StatusCode < http.StatusBadRequest {
		require.NoError(t, json.NewDecoder(response.Body).Decode(out))
	} else {
		_, _ = io.Copy(io.Discard, response.Body)
	}

	return response
}

// upgrade asks for a connection to become protocol, as vmhost does, and hands
// it back with what was answered. What the agent sends after its answer is
// left in the reader.
func (a *testAgent) upgrade(t *testing.T, path string, protocol string, body any) (net.Conn, *bufio.Reader, *http.Response) {
	t.Helper()

	conn, err := net.Dial("tcp", a.server.Listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	var payload []byte
	if body != nil {
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}

	request, err := http.NewRequest(http.MethodPost, "http://agent"+path, bytes.NewReader(payload))
	require.NoError(t, err)

	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", protocol)
	request.Header.Set("Content-Type", "application/json")

	require.NoError(t, conn.SetDeadline(time.Now().Add(10*time.Second)))
	require.NoError(t, request.Write(conn))

	reader := bufio.NewReader(conn)

	response, err := http.ReadResponse(reader, request)
	require.NoError(t, err)

	if response.StatusCode != http.StatusSwitchingProtocols {
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
	}

	return conn, reader, response
}

// start runs a task and says how it started.
func (a *testAgent) start(t *testing.T, process guest.Process) guest.Status {
	t.Helper()

	var status guest.Status

	response := a.call(t, http.MethodPost, "/process", process, &status)
	require.Equal(t, http.StatusOK, response.StatusCode)

	return status
}

// waitFor waits for a run of the task to end, and says how it ended.
func (a *testAgent) waitFor(t *testing.T, generation uint64) guest.Status {
	t.Helper()

	var status guest.Status

	response := a.call(t, http.MethodGet, "/process/wait?generation="+strconv.FormatUint(generation, 10), nil, &status)
	require.Equal(t, http.StatusOK, response.StatusCode)

	return status
}

// logs is every line the task wrote after the one numbered after.
func (a *testAgent) logs(t *testing.T, after uint64) []guest.LogLine {
	t.Helper()

	response, err := a.server.Client().Get(a.server.URL + "/logs?after=" + strconv.FormatUint(after, 10))
	require.NoError(t, err)
	defer response.Body.Close()

	require.Equal(t, http.StatusOK, response.StatusCode)

	var lines []guest.LogLine

	decoder := json.NewDecoder(response.Body)
	for {
		var line guest.LogLine
		if err := decoder.Decode(&line); err != nil {
			require.ErrorIs(t, err, io.EOF)

			return lines
		}

		lines = append(lines, line)
	}
}

// confinements is every kind of group the agent can keep what it starts in
// that this test can make: process groups always, and cgroups when it runs as
// root with a cgroup2 it may write to.
func confinements(t *testing.T) map[string]func(t *testing.T) confinement {
	kinds := map[string]func(t *testing.T) confinement{
		"process groups": func(t *testing.T) confinement { return newProcessGroups() },
	}

	if reason := cgroupsUnavailable(); len(reason) > 0 {
		t.Logf("only process groups are tried: %s", reason)

		return kinds
	}

	kinds["cgroups"] = func(t *testing.T) confinement {
		root := filepath.Join(cgroupRoot, "workload-guest-test-"+strconv.Itoa(os.Getpid())+"-"+strconv.FormatInt(time.Now().UnixNano(), 36))
		require.NoError(t, os.Mkdir(root, 0o755))

		// a test cgroup counted with whatever its parent hands on; none at
		// all still keeps, signals and ends what is in it.
		_ = enableControllers(root)

		t.Cleanup(func() { _ = removeCgroup(root, emptyTimeout) })

		return cgroups{root: root}
	}

	return kinds
}

// cgroupsUnavailable says why the test cannot keep processes in cgroups, or
// nothing when it can.
func cgroupsUnavailable() string {
	if os.Geteuid() != 0 {
		return "the test does not run as root"
	}

	var stat unix.Statfs_t
	if err := unix.Statfs(cgroupRoot, &stat); err != nil || stat.Type != unix.CGROUP2_SUPER_MAGIC {
		return cgroupRoot + " is not a cgroup2"
	}

	probe := filepath.Join(cgroupRoot, "workload-guest-probe-"+strconv.Itoa(os.Getpid()))
	if err := os.Mkdir(probe, 0o755); err != nil {
		return "no cgroup can be made: " + err.Error()
	}

	_ = os.Remove(probe)

	return ""
}

// forEachConfinement runs a test once for every kind of group the agent can
// keep what it starts in here.
func forEachConfinement(t *testing.T, test func(t *testing.T, confine func(t *testing.T) confinement)) {
	for name, confine := range confinements(t) {
		t.Run(name, func(t *testing.T) {
			test(t, confine)
		})
	}
}

// eventually waits for condition to hold, for a few seconds at most.
func eventually(t *testing.T, condition func() bool, message string) {
	t.Helper()

	assert.Eventually(t, condition, 5*time.Second, 10*time.Millisecond, message)
}

// gone reports whether there is no process pid any more.
func gone(pid int) bool {
	return errors.Is(unix.Kill(pid, 0), unix.ESRCH)
}

func TestHealth(t *testing.T) {
	a := newTestAgent(t, newProcessGroups())

	response := a.call(t, http.MethodGet, "/health", nil, nil)

	assert.Equal(t, http.StatusNoContent, response.StatusCode)
	assert.Equal(t, guest.ProtocolVersion, response.Header.Get(guest.VersionHeader))
}

func TestVersionOnEveryAnswer(t *testing.T) {
	a := newTestAgent(t, newProcessGroups())

	for name, request := range map[string]struct {
		method string
		path   string
		body   any
		status int
	}{
		"a route there is not":           {method: http.MethodGet, path: "/nothing", status: http.StatusNotFound},
		"a method a route does not take": {method: http.MethodDelete, path: "/health", status: http.StatusMethodNotAllowed},
		"a request that is not JSON":     {method: http.MethodPut, path: "/config", body: "not a config", status: http.StatusBadRequest},
		"what needs a machine told":      {method: http.MethodPut, path: "/hosts", body: []guest.Host{}, status: http.StatusConflict},
		"a status":                       {method: http.MethodGet, path: "/process", status: http.StatusOK},
		"stats":                          {method: http.MethodGet, path: "/stats", status: http.StatusOK},
	} {
		t.Run(name, func(t *testing.T) {
			response := a.call(t, request.method, request.path, request.body, nil)

			assert.Equal(t, request.status, response.StatusCode)
			assert.Equal(t, guest.ProtocolVersion, response.Header.Get(guest.VersionHeader))
		})
	}
}

func TestConfigure(t *testing.T) {
	t.Run("the machine is made what it is told, in order, and the task's files are written", func(t *testing.T) {
		a := newTestAgent(t, newProcessGroups())

		config := testConfig()
		config.Hosts = []guest.Host{{Address: "10.0.0.3", Names: []string{"db"}}}

		response := a.call(t, http.MethodPut, "/config", config, nil)
		require.Equal(t, http.StatusNoContent, response.StatusCode)

		assert.Equal(t, []string{"root", "interfaces", "hostname", "finish"}, a.machine.steps(), "a clock that agrees with the host's is left alone")

		machine := a.machine.seen()
		assert.Equal(t, config.Root, machine.root)
		assert.Equal(t, config.Interfaces, machine.interfaces)
		assert.Equal(t, "task-xkfqz", machine.hostname)
		assert.True(t, machine.finished)
		assert.False(t, machine.readOnly)

		hosts, err := os.ReadFile(filepath.Join(a.files, "hosts"))
		require.NoError(t, err)
		assert.Equal(t, string(hostsFile(config.Hostname, config.Interfaces, config.Hosts)), string(hosts))

		resolv, err := os.ReadFile(filepath.Join(a.files, "resolv.conf"))
		require.NoError(t, err)
		assert.Equal(t, "nameserver 1.1.1.1\n", string(resolv))

		hostname, err := os.ReadFile(filepath.Join(a.files, "hostname"))
		require.NoError(t, err)
		assert.Equal(t, "task-xkfqz\n", string(hostname))
	})

	t.Run("a clock the host disagrees with is set from the host's", func(t *testing.T) {
		a := newTestAgent(t, newProcessGroups())

		config := testConfig()
		config.Now = time.Now().Add(time.Hour)

		response := a.call(t, http.MethodPut, "/config", config, nil)
		require.Equal(t, http.StatusNoContent, response.StatusCode)

		assert.Equal(t, "clock", a.machine.steps()[0])
		assert.WithinDuration(t, config.Now, a.machine.seen().clock, time.Second)
	})

	t.Run("a read-only root is made read only once it is finished", func(t *testing.T) {
		a := newTestAgent(t, newProcessGroups())

		config := testConfig()
		config.Root.Scratch = ""

		response := a.call(t, http.MethodPut, "/config", config, nil)
		require.Equal(t, http.StatusNoContent, response.StatusCode)

		assert.True(t, a.machine.seen().readOnly)
	})

	t.Run("a machine is told once: the same again is taken, anything else refused", func(t *testing.T) {
		a := newTestAgent(t, newProcessGroups())

		config := testConfig()
		require.Equal(t, http.StatusNoContent, a.call(t, http.MethodPut, "/config", config, nil).StatusCode)

		config.Now = time.Now().Add(time.Minute)
		assert.Equal(t, http.StatusNoContent, a.call(t, http.MethodPut, "/config", config, nil).StatusCode, "the time it was told at is no part of what it is")

		config.Hostname = "another"
		assert.Equal(t, http.StatusConflict, a.call(t, http.MethodPut, "/config", config, nil).StatusCode)

		assert.Equal(t, []string{"root", "interfaces", "hostname", "finish"}, a.machine.steps(), "the machine was made once")
	})

	t.Run("a machine with no image to make its root from is refused", func(t *testing.T) {
		a := newTestAgent(t, newProcessGroups())

		config := testConfig()
		config.Root = guest.Root{}

		assert.Equal(t, http.StatusBadRequest, a.call(t, http.MethodPut, "/config", config, nil).StatusCode)
		assert.Empty(t, a.machine.steps())
	})

	t.Run("a machine that could not be made what it was told is not told again", func(t *testing.T) {
		a := newTestAgent(t, newProcessGroups())
		a.machine.failMounts(errors.New("no such disk"))

		assert.Equal(t, http.StatusInternalServerError, a.call(t, http.MethodPut, "/config", testConfig(), nil).StatusCode)
		assert.Equal(t, http.StatusInternalServerError, a.call(t, http.MethodPut, "/config", testConfig(), nil).StatusCode)

		assert.Equal(t, []string{"root"}, a.machine.steps(), "nothing was made again over half of what was")

		response := a.call(t, http.MethodPost, "/process", guest.Process{Args: []string{"/bin/true"}}, nil)
		assert.Equal(t, http.StatusConflict, response.StatusCode, "a machine that is nothing runs nothing")
	})
}

func TestSetHosts(t *testing.T) {
	t.Run("a machine not yet told what it is has no neighbours to be told of", func(t *testing.T) {
		a := newTestAgent(t, newProcessGroups())

		assert.Equal(t, http.StatusConflict, a.call(t, http.MethodPut, "/hosts", []guest.Host{}, nil).StatusCode)
	})

	t.Run("the hosts file is rewritten where it is, so what the task sees changes", func(t *testing.T) {
		a := configured(t, newProcessGroups())

		before, err := os.Stat(filepath.Join(a.files, "hosts"))
		require.NoError(t, err)

		hosts := []guest.Host{{Address: "127.0.0.2", Names: []string{"db", "db-xkfqz"}}}
		require.Equal(t, http.StatusNoContent, a.call(t, http.MethodPut, "/hosts", hosts, nil).StatusCode)

		after, err := os.Stat(filepath.Join(a.files, "hosts"))
		require.NoError(t, err)
		assert.True(t, os.SameFile(before, after))

		content, err := os.ReadFile(filepath.Join(a.files, "hosts"))
		require.NoError(t, err)
		assert.Equal(t, "127.0.0.1\tlocalhost\n"+
			"::1\tlocalhost ip6-localhost ip6-loopback\n"+
			"127.0.0.1\ttask-xkfqz\n"+
			"127.0.0.2\tdb db-xkfqz\n", string(content))

		require.Equal(t, http.StatusNoContent, a.call(t, http.MethodPut, "/hosts", []guest.Host{}, nil).StatusCode)

		content, err = os.ReadFile(filepath.Join(a.files, "hosts"))
		require.NoError(t, err)
		assert.NotContains(t, string(content), "db", "a neighbour that left is forgotten")
	})
}

func TestPowerOff(t *testing.T) {
	forEachConfinement(t, func(t *testing.T, confine func(t *testing.T) confinement) {
		a := configured(t, confine(t))

		var pid int
		a.start(t, guest.Process{Args: []string{"/bin/sh", "-c", "sleep 60"}})

		a.lock.Lock()
		pid = a.pid
		a.lock.Unlock()

		response := a.call(t, http.MethodPost, "/poweroff", nil, nil)
		assert.Equal(t, http.StatusAccepted, response.StatusCode)

		select {
		case <-a.machine.poweredOff:
		case <-time.After(10 * time.Second):
			t.Fatal("the machine was not turned off")
		}

		eventually(t, func() bool { return gone(pid) }, "the task was ended before the machine was turned off")

		// asked again, it is turned off once.
		a.call(t, http.MethodPost, "/poweroff", nil, nil)
		time.Sleep(2 * answerTime)
		assert.Equal(t, 1, countOf(a.machine.steps(), "power off"))
	})
}

func TestServe(t *testing.T) {
	t.Run("the agent answers on any listener, until it is told to stop", func(t *testing.T) {
		a := newAgent(slog.New(slog.NewTextHandler(io.Discard, nil)), newFakeMachine(), dirs{root: "/", files: t.TempDir()}, newProcessGroups())

		listener, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())

		served := make(chan error, 1)
		go func() { served <- a.serve(ctx, listener) }()

		response, err := http.Get("http://" + listener.Addr().String() + "/health")
		require.NoError(t, err)
		response.Body.Close()

		assert.Equal(t, http.StatusNoContent, response.StatusCode)
		assert.Equal(t, guest.ProtocolVersion, response.Header.Get(guest.VersionHeader))

		cancel()

		select {
		case err := <-served:
			assert.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("the agent did not stop")
		}
	})
}

func countOf(values []string, value string) int {
	count := 0
	for _, v := range values {
		if v == value {
			count++
		}
	}

	return count
}
