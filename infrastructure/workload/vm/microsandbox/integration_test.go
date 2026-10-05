//go:build microsandbox && integration

package microsandbox

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	msb "github.com/superradcompany/microsandbox/sdk/go"

	"github.com/khanzadimahdi/testproject/domain"
	dockerdomain "github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/docker"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/task/vmruntime"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

func TestVersion(t *testing.T) {
	assert.Equal(t, msb.SDKVersion(), Version, "the engine says it is the release of the SDK it is built with")
}

func TestMachineLifecycle(t *testing.T) {
	e := engineFor(t)
	ctx := t.Context()

	spec := machine("it-machine", withPorts(8000))
	instance := created(t, e, spec)

	assert.Equal(t, vm.InstanceRunning, instance.State)
	assert.Equal(t, spec.Labels, instance.Labels)
	require.Len(t, instance.Endpoints, 1)
	assert.Equal(t, port.Port(8000), instance.Endpoints[0].Port)

	host, _, err := net.SplitHostPort(instance.Endpoints[0].Address)
	require.NoError(t, err)
	assert.Equal(t, itOptions(t).BindAddress, host, "a port is reached on the vmhost's own address")

	assert.Equal(t, ran{stdout: "out\n", stderr: "err\n", code: 3}, run(t, e, spec.ID, "echo out; echo err >&2; exit 3"))

	runOK(t, e, spec.ID, "echo persisted > /root/marker && sync")

	info, err := e.Info(ctx)
	require.NoError(t, err)
	assert.Equal(t, Name, info.Engine)
	assert.Equal(t, Version, info.Version)
	assert.NotZero(t, info.CPUs)
	assert.NotZero(t, info.Memory)
	assert.NotZero(t, info.Disk)
	assert.GreaterOrEqual(t, info.Allocated.Memory, spec.Resources.Memory)

	listed, err := e.List(ctx)
	require.NoError(t, err)
	assert.Contains(t, listed, inspected(t, e, spec.ID))

	_, err = e.Create(ctx, spec)
	assert.ErrorIs(t, err, domain.ErrAlreadyExists)

	t.Run("a stopped vm keeps its disk, and runs nothing", func(t *testing.T) {
		require.NoError(t, e.Stop(ctx, spec.ID))
		assert.Equal(t, vm.InstanceStopped, inspected(t, e, spec.ID).State)

		_, err := e.Exec(ctx, spec.ID, vm.ExecOptions{Command: []string{"true"}})
		assert.ErrorIs(t, err, vm.ErrNotRunning)

		_, err = e.Stats(ctx, spec.ID)
		assert.ErrorIs(t, err, vm.ErrNotRunning)

		require.NoError(t, e.Stop(ctx, spec.ID), "stopping one that is stopped is what was asked for")
	})

	t.Run("a started vm has its disk back", func(t *testing.T) {
		require.NoError(t, e.Start(ctx, spec.ID))

		started := inspected(t, e, spec.ID)
		assert.Equal(t, vm.InstanceRunning, started.State)
		assert.Equal(t, instance.Endpoints, started.Endpoints, "it keeps its ports")
		assert.Equal(t, "persisted", runOK(t, e, spec.ID, "cat /root/marker"))

		require.NoError(t, e.Start(ctx, spec.ID), "starting one that runs is what was asked for")
	})

	t.Run("a restarted vm boots again, with its disk and its ports", func(t *testing.T) {
		before := inspected(t, e, spec.ID)

		require.NoError(t, e.Restart(ctx, spec.ID))

		after := inspected(t, e, spec.ID)
		assert.Equal(t, vm.InstanceRunning, after.State)
		assert.True(t, after.StartedAt.After(before.StartedAt))
		assert.Equal(t, before.Endpoints, after.Endpoints)
		assert.Equal(t, "persisted", runOK(t, e, spec.ID, "cat /root/marker"))
	})

	t.Run("a deleted vm is gone, and deleting it again is what was asked for", func(t *testing.T) {
		require.NoError(t, e.Delete(ctx, spec.ID))

		_, err := e.Inspect(ctx, spec.ID)
		assert.ErrorIs(t, err, domain.ErrNotExists)

		assert.NoError(t, e.Delete(ctx, spec.ID))
		assert.ErrorIs(t, e.Start(ctx, spec.ID), domain.ErrNotExists)
		assert.ErrorIs(t, e.Stop(ctx, spec.ID), domain.ErrNotExists)

		_, err = msb.GetSandbox(ctx, spec.ID)
		assert.True(t, msb.IsKind(err, msb.ErrSandboxNotFound), "its sandbox is gone too: %v", err)
	})
}

func TestMachineLogsAndStats(t *testing.T) {
	e := engineFor(t)
	ctx := t.Context()

	spec := machine("it-observe", func(spec *vm.Spec) { spec.Resources.CPUs = 2 })
	created(t, e, spec)

	runOK(t, e, spec.ID, "echo exec-line; echo console-line > /dev/console")

	var lines []vm.LogLine

	eventually(t, 10*time.Second, func() bool {
		var err error
		lines, err = e.Logs(ctx, spec.ID, vm.LogOptions{})
		require.NoError(t, err)

		return hasLine(lines, vm.LogSourceExec, "exec-line") && hasLine(lines, vm.LogSourceKernel, "console-line")
	}, "the exec's and the console's lines are in the log: %v", lines)

	assert.True(t, hasSource(lines, vm.LogSourceRuntime), "what microsandbox's runtime said is in it")
	assert.False(t, hasSource(lines, vm.LogSourceMain), "a vm has no main process")

	t.Run("since keeps the lines written at that moment, and none before", func(t *testing.T) {
		at := lineAt(t, lines, vm.LogSourceExec, "exec-line")

		since, err := e.Logs(ctx, spec.ID, vm.LogOptions{Since: at})
		require.NoError(t, err)
		assert.True(t, hasLine(since, vm.LogSourceExec, "exec-line"))

		for _, line := range since {
			assert.False(t, line.At.Before(at), "%v", line)
		}
	})

	t.Run("tail keeps the last lines", func(t *testing.T) {
		tail, err := e.Logs(ctx, spec.ID, vm.LogOptions{Tail: 2})
		require.NoError(t, err)
		require.Len(t, tail, 2)

		all, err := e.Logs(ctx, spec.ID, vm.LogOptions{})
		require.NoError(t, err)
		assert.Equal(t, all[len(all)-2:], tail)
	})

	t.Run("stats are of the whole vm, cpu across all its vcpus", func(t *testing.T) {
		// one of its two vCPUs kept busy is half of it, never all of it.
		runOK(t, e, spec.ID, "(timeout 20 sh -c 'while :; do :; done' </dev/null >/dev/null 2>&1 &); sleep 3")

		stats, err := e.Stats(ctx, spec.ID)
		require.NoError(t, err)

		t.Logf("stats: %+v", stats)

		assert.GreaterOrEqual(t, stats.CPUPercent, float64(0))
		assert.LessOrEqual(t, stats.CPUPercent, float64(80), "one busy vCPU of two is not the whole vm")
		assert.Equal(t, spec.Resources.Memory, stats.MemoryLimit)
		assert.NotZero(t, stats.MemoryUsed)
		assert.NotZero(t, stats.DiskTotal)
		assert.NotZero(t, stats.DiskUsed)
		assert.Less(t, stats.DiskUsed, stats.DiskTotal)
		assert.WithinDuration(t, time.Now(), stats.SampledAt, time.Minute)
	})
}

func hasLine(lines []vm.LogLine, source string, text string) bool {
	for _, line := range lines {
		if line.Source == source && strings.Contains(line.Line, text) {
			return true
		}
	}

	return false
}

func hasSource(lines []vm.LogLine, source string) bool {
	for _, line := range lines {
		if line.Source == source {
			return true
		}
	}

	return false
}

func lineAt(t *testing.T, lines []vm.LogLine, source string, text string) time.Time {
	t.Helper()

	for _, line := range lines {
		if line.Source == source && strings.Contains(line.Line, text) {
			return line.At
		}
	}

	t.Fatalf("no %s line %q", source, text)

	return time.Time{}
}

func TestDisks(t *testing.T) {
	e := engineFor(t)
	ctx := t.Context()

	t.Run("a disk that is not persistent is pristine on every start", func(t *testing.T) {
		spec := machine("it-pristine", withPorts(8000), func(spec *vm.Spec) { spec.PersistentDisk = false })
		instance := created(t, e, spec)

		runOK(t, e, spec.ID, "echo dirty > /root/dirty && sync")

		require.NoError(t, e.Stop(ctx, spec.ID))
		require.NoError(t, e.Start(ctx, spec.ID))

		assert.Equal(t, 1, run(t, e, spec.ID, "test -e /root/dirty").code, "what it wrote went with the stop")
		assert.Equal(t, instance.Endpoints, inspected(t, e, spec.ID).Endpoints, "it keeps its ports")

		runOK(t, e, spec.ID, "echo dirty > /root/dirty && sync")
		require.NoError(t, e.Restart(ctx, spec.ID))

		assert.Equal(t, 1, run(t, e, spec.ID, "test -e /root/dirty").code, "and with a restart")
	})

	t.Run("a persistent disk keeps what was written to it", func(t *testing.T) {
		spec := machine("it-persistent")
		created(t, e, spec)

		runOK(t, e, spec.ID, "echo kept > /root/kept && sync")

		require.NoError(t, e.Stop(ctx, spec.ID))
		require.NoError(t, e.Start(ctx, spec.ID))

		assert.Equal(t, "kept", runOK(t, e, spec.ID, "cat /root/kept"))

		// what the filesystem keeps of itself is not counted in its size.
		size, err := strconv.ParseUint(runOK(t, e, spec.ID, "df -P -k / | tail -1 | awk '{print $2}'"), 10, 64)
		require.NoError(t, err)
		assert.InDelta(t, float64(spec.Resources.Disk), float64(size<<10), float64(spec.Resources.Disk)/10, "the disk is the size it was given")
	})
}

func TestPorts(t *testing.T) {
	e := engineFor(t)
	bind := itOptions(t).BindAddress

	spec := machine("it-ports", withPorts(8000))
	instance := created(t, e, spec)
	serve(t, e, spec.ID, 8000, "hello-from-it-ports")

	address := endpoint(t, instance, 8000)

	t.Run("the orchestrator reaches a published port", func(t *testing.T) {
		var body string

		eventually(t, 20*time.Second, func() bool {
			var err error
			body, err = fetch(address, bind)

			return err == nil
		}, "GET %s from %s", address, bind)

		assert.Equal(t, "hello-from-it-ports", body)
	})

	t.Run("nobody else does", func(t *testing.T) {
		// anybody else reaches what listens there and is not a vm's.
		listener, err := net.Listen("tcp", net.JoinHostPort(bind, "0"))
		require.NoError(t, err)

		control := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, "not-a-vm\n")
		})}
		defer control.Close()

		go func() { _ = control.Serve(listener) }()

		body, err := fetch(listener.Addr().String(), elsewhere)
		require.NoError(t, err)
		require.Equal(t, "not-a-vm", body)

		body, err = fetch(address, elsewhere)
		assert.Error(t, err, "GET %s from %s answered %q", address, elsewhere, body)
	})

	t.Run("a vm whose ingress is denied publishes nothing", func(t *testing.T) {
		denied := created(t, e, machine("it-ports-denied", withPorts(8000), withNetwork(vm.AccessDeny, vm.AccessAllow)))
		assert.Empty(t, denied.Endpoints)
	})
}

func TestNetworkModes(t *testing.T) {
	e := engineFor(t)
	bind := itOptions(t).BindAddress

	for _, mode := range []struct {
		ingress vm.Access
		egress  vm.Access
	}{
		{vm.AccessAllow, vm.AccessAllow},
		{vm.AccessAllow, vm.AccessDeny},
		{vm.AccessDeny, vm.AccessAllow},
		{vm.AccessDeny, vm.AccessDeny},
	} {
		t.Run(fmt.Sprintf("ingress %s, egress %s", mode.ingress, mode.egress), func(t *testing.T) {
			spec := machine(fmt.Sprintf("it-net-%s-%s", mode.ingress, mode.egress), withPorts(8000), withNetwork(mode.ingress, mode.egress))
			instance := created(t, e, spec)

			out := run(t, e, spec.ID, "nslookup example.com >/dev/null 2>&1 && wget -q -T 10 -O /dev/null https://example.com/")
			if mode.egress == vm.AccessAllow {
				assert.Equal(t, 0, out.code, "the public internet is reached: %+v", out)
			} else {
				assert.NotEqual(t, 0, out.code, "nothing is reached: %+v", out)
			}

			if mode.ingress == vm.AccessDeny {
				assert.Empty(t, instance.Endpoints)

				return
			}

			serve(t, e, spec.ID, 8000, "in")
			address := endpoint(t, instance, 8000)

			eventually(t, 20*time.Second, func() bool {
				body, err := fetch(address, bind)

				return err == nil && body == "in"
			}, "GET %s", address)
		})
	}
}

func TestIsolation(t *testing.T) {
	e := engineFor(t)

	target := created(t, e, machine("it-isolated-target", withPorts(8000)))
	serve(t, e, target.ID, 8000, "secret-of-the-target")

	_, published, err := net.SplitHostPort(endpoint(t, target, 8000))
	require.NoError(t, err)

	reaching := created(t, e, machine("it-isolated-reaching", withPorts(8000)))

	for _, address := range []string{
		net.JoinHostPort(itOptions(t).BindAddress, published),
		net.JoinHostPort("host.microsandbox.internal", published),
		endpoint(t, reaching, 8000),
	} {
		out := run(t, e, reaching.ID, "wget -q -T 5 -O- http://"+address+"/")
		assert.NotEqual(t, 0, out.code, "a vm reached %s: %+v", address, out)
		assert.NotContains(t, out.stdout, "secret-of-the-target")
	}
}

func TestPTY(t *testing.T) {
	e := engineFor(t)
	ctx := t.Context()

	spec := machine("it-pty")
	created(t, e, spec)

	session, err := e.Exec(ctx, spec.ID, vm.ExecOptions{Command: []string{"/bin/sh"}, TTY: true, Rows: 40, Cols: 120})
	require.NoError(t, err)
	defer session.Close()

	var lock sync.Mutex
	var output bytes.Buffer

	go func() {
		buffer := make([]byte, 4096)

		for {
			n, err := session.Stdout().Read(buffer)

			lock.Lock()
			output.Write(buffer[:n])
			lock.Unlock()

			if err != nil {
				return
			}
		}
	}()

	said := func(text string) func() bool {
		return func() bool {
			lock.Lock()
			defer lock.Unlock()

			return strings.Contains(output.String(), text)
		}
	}

	_, err = io.WriteString(session.Stdin(), "stty size\n")
	require.NoError(t, err)
	eventually(t, 10*time.Second, said("40 120"), "the terminal starts at the size asked for: %q", &output)

	require.NoError(t, session.Resize(ctx, 50, 160))
	_, err = io.WriteString(session.Stdin(), "stty size; echo TERM=$TERM\n")
	require.NoError(t, err)
	eventually(t, 10*time.Second, said("50 160"), "the terminal is resized: %q", &output)
	eventually(t, 10*time.Second, said("TERM=xterm-256color"), "the terminal says what it is: %q", &output)

	stderr, err := io.ReadAll(session.Stderr())
	require.NoError(t, err)
	assert.Empty(t, stderr, "a terminal's errors are on its output")

	_, err = io.WriteString(session.Stdin(), "exit 7\n")
	require.NoError(t, err)

	code, err := session.Wait(ctx)
	require.NoError(t, err)
	assert.Equal(t, 7, code)

	t.Run("closing a session ends its command", func(t *testing.T) {
		session, err := e.Exec(ctx, spec.ID, vm.ExecOptions{Command: []string{"sleep", "600"}})
		require.NoError(t, err)

		require.NoError(t, session.Close())

		waiting, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		code, err := session.Wait(waiting)
		require.NoError(t, err)
		assert.Equal(t, -1, code, "it was killed")
		assert.Equal(t, "", runOK(t, e, spec.ID, "pgrep -x sleep || true"))
	})
}

func TestDockerVM(t *testing.T) {
	e := engineFor(t)
	ctx := t.Context()
	bind := itOptions(t).BindAddress
	daemons := docker.NewDaemons(e, 3*time.Minute, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	spec := dockerVM("it-docker", withPorts(8080))
	instance := created(t, e, spec)
	daemon := daemons.Daemon(spec.ID)

	pinged := time.Now()
	require.NoError(t, daemon.Ping(ctx))
	t.Logf("dockerd answered %s after the create", time.Since(pinged).Round(time.Millisecond))

	assert.Equal(t, "vminit", runOK(t, e, spec.ID, "cat /proc/1/comm"), "dockerd is looked after by vminit as the guest's init")

	page := func(address string) func() bool {
		return func() bool {
			body, err := fetch(address, bind)

			return err == nil && body == "hello-from-nginx"
		}
	}

	t.Run("dockerd is reached over dial-stdio", func(t *testing.T) {
		started := time.Now()

		web, err := daemon.CreateContainer(ctx, dockerdomain.ContainerSpec{
			Name:          "web",
			Image:         "nginx:alpine",
			Ports:         []dockerdomain.PortBinding{{ContainerPort: 80, HostPort: 8080}},
			RestartPolicy: "unless-stopped",
		})
		require.NoError(t, err)
		assert.Equal(t, "running", web.State)
		t.Logf("pulled and started nginx in %s", time.Since(started).Round(time.Millisecond))

		runOK(t, e, spec.ID, "docker exec web sh -c 'echo hello-from-nginx > /usr/share/nginx/html/index.html'")

		eventually(t, 30*time.Second, page(endpoint(t, instance, 8080)), "the container is reached through the vm's port")

		containers, err := daemon.Containers(ctx, dockerdomain.ContainerFilter{All: true})
		require.NoError(t, err)
		require.Len(t, containers, 1)
		assert.Equal(t, "web", containers[0].Name)
	})

	t.Run("a compose stack comes up, stops, starts, restarts and goes", func(t *testing.T) {
		compose := daemons.Compose(spec.ID)

		const yaml = `services:
  app:
    image: busybox:latest
    command: ["sh", "-c", "echo up; sleep 3600"]
    restart: unless-stopped
`

		stack := func() []dockerdomain.Container {
			containers, err := daemon.Containers(ctx, dockerdomain.ContainerFilter{All: true, Stack: "demo"})
			require.NoError(t, err)

			return containers
		}

		_, err := compose.Up(ctx, "demo", yaml)
		require.NoError(t, err)
		require.Len(t, stack(), 1)
		assert.Equal(t, "running", stack()[0].State)

		_, err = compose.Stop(ctx, "demo", yaml)
		require.NoError(t, err)
		assert.Equal(t, "exited", stack()[0].State)

		_, err = compose.Start(ctx, "demo", yaml)
		require.NoError(t, err)
		assert.Equal(t, "running", stack()[0].State)

		_, err = compose.Restart(ctx, "demo", yaml)
		require.NoError(t, err)
		assert.Equal(t, "running", stack()[0].State)

		_, err = compose.Down(ctx, "demo", yaml, true)
		require.NoError(t, err)
		assert.Empty(t, stack())

		_, err = compose.Up(ctx, "demo", "services: [not valid")
		assert.Error(t, err, "compose's own refusal is an error")
	})

	t.Run("a stop stops dockerd gracefully, and a start brings its containers back", func(t *testing.T) {
		require.NoError(t, e.Stop(ctx, spec.ID))
		daemons.Forget(spec.ID)
		assert.Equal(t, vm.InstanceStopped, inspected(t, e, spec.ID).State)

		require.NoError(t, e.Start(ctx, spec.ID))
		require.NoError(t, daemon.Ping(ctx))

		eventually(t, 60*time.Second, page(endpoint(t, instance, 8080)), "the unless-stopped container is back")

		log := runOK(t, e, spec.ID, "cat /var/log/vminit.log")
		assert.Contains(t, log, "stopping dockerd", "vminit was told to stop")
		assert.Contains(t, log, "dockerd stopped after", "and stopped dockerd itself")
	})

	runOK(t, e, spec.ID, "echo docker-data > /root/data.txt && sync")

	archive, written := archived(t, e, spec.ID)
	assert.Equal(t, vm.KindDocker, written.Kind)
	assert.Equal(t, spec.Resources.Disk, written.Disk)

	t.Run("a snapshot restores as a new vm, with dockerd under its supervisor", func(t *testing.T) {
		copied := dockerVM("it-docker-copy", withPorts(8080))
		instance := restored(t, e, copied, archive)

		assert.Equal(t, vm.InstanceRunning, instance.State)
		assert.Equal(t, copied.Labels, instance.Labels, "its labels are the spec's, which a restore drops")
		require.NoError(t, daemons.Daemon(copied.ID).Ping(ctx))

		assert.Equal(t, "docker-data", runOK(t, e, copied.ID, "cat /root/data.txt"))
		assert.NotEqual(t, "vminit", runOK(t, e, copied.ID, "cat /proc/1/comm"), "a restore drops the init")
		assert.NotEmpty(t, runOK(t, e, copied.ID, "cat /run/vminit.pid"), "so the supervisor looks after dockerd")

		eventually(t, 60*time.Second, page(endpoint(t, instance, 8080)), "its unless-stopped container is back")

		t.Run("which stops dockerd gracefully ahead of a stop", func(t *testing.T) {
			require.NoError(t, e.Stop(ctx, copied.ID))
			daemons.Forget(copied.ID)
			require.NoError(t, e.Start(ctx, copied.ID))
			require.NoError(t, daemons.Daemon(copied.ID).Ping(ctx))

			assert.Contains(t, runOK(t, e, copied.ID, "cat /var/log/vminit.log"), "signal TERM: stopping dockerd")
		})
	})

	t.Run("a snapshot restores onto the vm it was taken of, which keeps its ports", func(t *testing.T) {
		runOK(t, e, spec.ID, "echo changed > /root/data.txt")

		before := inspected(t, e, spec.ID)
		after := restoredOnto(t, e, spec, archive)
		daemons.Forget(spec.ID)

		assert.Equal(t, before.Endpoints, after.Endpoints)
		require.NoError(t, daemon.Ping(ctx))
		assert.Equal(t, "docker-data", runOK(t, e, spec.ID, "cat /root/data.txt"))
	})

	t.Run("a reconfigured docker vm keeps its disk, its ports and its containers", func(t *testing.T) {
		before := inspected(t, e, spec.ID)

		reconfigured := spec
		reconfigured.Ports = []port.Port{8080, 9090}

		after, err := e.Reconfigure(ctx, reconfigured)
		require.NoError(t, err)
		daemons.Forget(spec.ID)

		assert.Equal(t, vm.InstanceRunning, after.State)
		assert.Equal(t, endpoint(t, before, 8080), endpoint(t, after, 8080))
		assert.NotEmpty(t, endpoint(t, after, 9090))

		require.NoError(t, daemon.Ping(ctx))
		assert.Equal(t, "docker-data", runOK(t, e, spec.ID, "cat /root/data.txt"))
		eventually(t, 60*time.Second, page(endpoint(t, after, 8080)), "its unless-stopped container is back")
	})
}

func TestSnapshotAndRestore(t *testing.T) {
	e := engineFor(t)
	ctx := t.Context()

	spec := machine("it-snap", withPorts(8000))
	original := created(t, e, spec)

	runOK(t, e, spec.ID, "echo v1 > /root/v && sync")

	archive, written := archived(t, e, spec.ID)
	assert.Equal(t, vm.KindMachine, written.Kind)
	assert.Equal(t, machineImage(), written.Image)
	assert.Equal(t, spec.Resources.Disk, written.Disk)

	assert.Equal(t, "v1", runOK(t, e, spec.ID, "cat /root/v"), "a live snapshot leaves the vm running")
	runOK(t, e, spec.ID, "echo v2 > /root/v && sync")

	t.Run("as a new vm, with the resources, ports and labels it is restored with", func(t *testing.T) {
		copied := machine("it-snap-copy", withPorts(8000, 9000), func(spec *vm.Spec) {
			spec.Resources.CPUs = 2
			spec.Resources.Memory = 512 << 20
		})

		instance := restored(t, e, copied, archive)
		assert.Equal(t, vm.InstanceRunning, instance.State)
		assert.Equal(t, copied.Labels, instance.Labels)
		assert.Len(t, instance.Endpoints, 2)

		assert.Equal(t, "v1", runOK(t, e, copied.ID, "cat /root/v"))
		assert.Equal(t, "2", runOK(t, e, copied.ID, "nproc"))

		t.Run("which snapshots and restores again", func(t *testing.T) {
			again, _ := archived(t, e, copied.ID)

			twice := restored(t, e, machine("it-snap-twice"), again)
			assert.Equal(t, vm.InstanceRunning, twice.State)
			assert.Equal(t, "v1", runOK(t, e, "it-snap-twice", "cat /root/v"))
		})
	})

	t.Run("onto the vm it was taken of, which keeps its ports", func(t *testing.T) {
		after := restoredOnto(t, e, spec, archive)

		assert.Equal(t, original.Endpoints, after.Endpoints)
		assert.Equal(t, "v1", runOK(t, e, spec.ID, "cat /root/v"))
	})

	t.Run("onto a vm that is stopped, which boots", func(t *testing.T) {
		require.NoError(t, e.Stop(ctx, spec.ID))

		after := restoredOnto(t, e, spec, archive)

		assert.Equal(t, vm.InstanceRunning, after.State)
		assert.Equal(t, "v1", runOK(t, e, spec.ID, "cat /root/v"))
	})

	t.Run("an archive another engine wrote is refused, and the vm left as it was", func(t *testing.T) {
		other := memory.New()

		_, err := other.Create(ctx, vm.Spec{ID: spec.ID, Kind: vm.KindMachine, Image: machineImage()})
		require.NoError(t, err)

		var foreign bytes.Buffer
		_, err = other.Snapshot(ctx, spec.ID, &foreign)
		require.NoError(t, err)

		_, err = e.Restore(ctx, spec, &foreign)
		assert.ErrorIs(t, err, vm.ErrEngineMismatch)

		assert.Equal(t, vm.InstanceRunning, inspected(t, e, spec.ID).State)
		assert.Equal(t, "v1", runOK(t, e, spec.ID, "cat /root/v"))
	})

	t.Run("nothing is archived of an instance that is not there", func(t *testing.T) {
		_, err := e.Snapshot(ctx, "it-nothing", io.Discard)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestReconfigure(t *testing.T) {
	e := engineFor(t)
	ctx := t.Context()
	bind := itOptions(t).BindAddress

	spec := machine("it-reconfigure", withPorts(8000))
	before := created(t, e, spec)

	runOK(t, e, spec.ID, "echo kept > /root/kept && sync")

	reconfigured := spec
	reconfigured.Ports = []port.Port{8000, 9000}
	reconfigured.Network.Egress = vm.AccessDeny

	started := time.Now()

	after, err := e.Reconfigure(ctx, reconfigured)
	require.NoError(t, err)
	t.Logf("reconfigured in %s", time.Since(started).Round(time.Millisecond))

	assert.Equal(t, vm.InstanceRunning, after.State)
	assert.Equal(t, endpoint(t, before, 8000), endpoint(t, after, 8000), "a port it keeps keeps its host port")
	assert.NotEmpty(t, endpoint(t, after, 9000))
	assert.Equal(t, "kept", runOK(t, e, spec.ID, "cat /root/kept"))
	assert.NotEqual(t, 0, run(t, e, spec.ID, "nslookup example.com >/dev/null 2>&1").code, "its egress is denied now")

	serve(t, e, spec.ID, 9000, "on-the-new-port")
	eventually(t, 20*time.Second, func() bool {
		body, err := fetch(endpoint(t, after, 9000), bind)

		return err == nil && body == "on-the-new-port"
	}, "the new port is published")

	t.Run("a spec that changes nothing does not restart it", func(t *testing.T) {
		same, err := e.Reconfigure(ctx, reconfigured)
		require.NoError(t, err)
		assert.Equal(t, inspected(t, e, spec.ID).StartedAt, same.StartedAt)
	})

	t.Run("a stopped vm stays stopped, and boots with what it was given", func(t *testing.T) {
		require.NoError(t, e.Stop(ctx, spec.ID))

		bigger := reconfigured
		bigger.Resources.CPUs = 2

		after, err := e.Reconfigure(ctx, bigger)
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceStopped, after.State)

		require.NoError(t, e.Start(ctx, spec.ID))
		assert.Equal(t, "2", runOK(t, e, spec.ID, "nproc"))
		assert.Equal(t, "kept", runOK(t, e, spec.ID, "cat /root/kept"))
	})

	t.Run("one that is pristine on every start boots again with it", func(t *testing.T) {
		pristine := machine("it-reconfigure-pristine", withPorts(8000), func(spec *vm.Spec) { spec.PersistentDisk = false })
		created(t, e, pristine)

		pristine.Ports = []port.Port{8000, 8001}

		after, err := e.Reconfigure(ctx, pristine)
		require.NoError(t, err)
		assert.Equal(t, vm.InstanceRunning, after.State)
		assert.Len(t, after.Endpoints, 2)
	})
}

func TestCapacity(t *testing.T) {
	e := engineFor(t)
	ctx := t.Context()

	info, err := e.Info(ctx)
	require.NoError(t, err)

	spec := machine("it-too-big", func(spec *vm.Spec) { spec.Resources.Memory = info.Memory + 1<<20 })

	_, err = e.Create(ctx, spec)
	assert.ErrorIs(t, err, vm.ErrNoCapacity)

	_, err = e.Inspect(ctx, spec.ID)
	assert.ErrorIs(t, err, domain.ErrNotExists, "nothing is left of it")
}

func TestCodeRunner(t *testing.T) {
	e := engineFor(t)
	ctx := t.Context()
	runtime := vmruntime.New(e, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	execution := func(taskUUID string, image string, entrypoint []string, command []string) *task.Execution {
		return &task.Execution{
			TaskUUID:       taskUUID,
			TaskName:       taskUUID,
			Slug:           taskUUID,
			OwnerUUID:      itOwner,
			Attempt:        1,
			Image:          image,
			ResourceLimits: task.ResourceLimits{Cpu: 1, Memory: 256 << 20, Disk: 100 << 20},
			NetworkPolicy:  network.PolicyNone,
			Entrypoint:     entrypoint,
			Command:        command,
		}
	}

	ran := func(t *testing.T, execution *task.Execution) (string, task.Execution, string) {
		t.Helper()

		started := time.Now()

		id, err := runtime.Create(ctx, execution)
		require.NoError(t, err)
		t.Cleanup(func() { removed(t, e, id) })

		eventually(t, 5*time.Minute, func() bool {
			return inspected(t, e, id).State == vm.InstanceExited
		}, "%s exits", id)

		t.Logf("%s: ran in %s", execution.TaskUUID, time.Since(started).Round(time.Millisecond))

		run, err := runtime.Inspect(ctx, id)
		require.NoError(t, err)

		var logs bytes.Buffer
		require.NoError(t, runtime.Logs(ctx, id, &logs))

		return id, run, logs.String()
	}

	t.Run("a snippet's exit code and output", func(t *testing.T) {
		id, run, logs := ran(t, execution("it-task-python", "python:3.12-alpine", nil, []string{"python", "-c", "print(1); import sys; sys.exit(3)"}))

		assert.Equal(t, task.StatusExited, run.Status)
		assert.Equal(t, 3, run.ExitCode)
		assert.Equal(t, "1\n", logs)

		_, err := msb.GetSandbox(ctx, id)
		require.NoError(t, err)

		t.Run("which runs again from scratch when it is restarted", func(t *testing.T) {
			require.NoError(t, runtime.Restart(ctx, id))

			eventually(t, 2*time.Minute, func() bool {
				return inspected(t, e, id).State == vm.InstanceExited
			}, "%s exits again", id)

			again, err := runtime.Inspect(ctx, id)
			require.NoError(t, err)
			assert.Equal(t, 3, again.ExitCode)
		})
	})

	t.Run("an entrypoint replaces the image's, in the environment and directory it is given", func(t *testing.T) {
		replaced := execution("it-task-entrypoint", machineImage(), []string{"sh", "-c"}, []string{"echo from-the-entrypoint $GREETING in $(pwd); exit 5"})
		replaced.Environment = []string{"GREETING=hello"}
		replaced.WorkingDirectory = "/tmp"

		_, run, logs := ran(t, replaced)

		assert.Equal(t, 5, run.ExitCode)
		assert.Equal(t, "from-the-entrypoint hello in /tmp\n", logs)
	})

	t.Run("a command that is not there exits as a shell says", func(t *testing.T) {
		id, run, _ := ran(t, execution("it-task-missing", machineImage(), nil, []string{"no-such-command"}))

		assert.Equal(t, 127, run.ExitCode)

		lines, err := e.Logs(ctx, id, vm.LogOptions{})
		require.NoError(t, err)
		assert.True(t, hasLine(lines, vm.LogSourceMain, "no-such-command"), "its log says why: %v", lines)
	})

	t.Run("an exec's output is not the main process's", func(t *testing.T) {
		id, err := runtime.Create(ctx, execution("it-task-live", machineImage(), nil, []string{"sh", "-c", "echo main-started; sleep 600"}))
		require.NoError(t, err)
		t.Cleanup(func() { removed(t, e, id) })

		assert.Equal(t, vm.InstanceRunning, inspected(t, e, id).State)
		runOK(t, e, id, "echo from-an-exec")

		var lines []vm.LogLine
		eventually(t, 10*time.Second, func() bool {
			lines, err = e.Logs(ctx, id, vm.LogOptions{})
			require.NoError(t, err)

			return hasLine(lines, vm.LogSourceMain, "main-started") && hasLine(lines, vm.LogSourceExec, "from-an-exec")
		}, "both are in the log: %v", lines)

		assert.False(t, hasLine(lines, vm.LogSourceMain, "from-an-exec"))

		require.NoError(t, runtime.Stop(ctx, id))
		assert.Equal(t, vm.InstanceStopped, inspected(t, e, id).State, "stopped, rather than exited")
	})
}

// TestReadoptionChild is the vmhost that goes away in TestReadoption: it makes
// what the next one takes back, and exits without stopping any of it.
func TestReadoptionChild(t *testing.T) {
	if len(os.Getenv("VMHOST_IT_CHILD")) == 0 {
		t.Skip("TestReadoption runs it")
	}

	ctx := t.Context()

	e, err := New(ctx, itOptions(t))
	require.NoError(t, err)

	machineVM := created(t, e, machine("it-adopt-machine", withPorts(8000)))
	runOK(t, e, machineVM.ID, "echo adopted > /root/adopted && sync")
	serve(t, e, machineVM.ID, 8000, "served-before-the-vmhost-went")

	taskVM := created(t, e, machine("it-adopt-task", func(spec *vm.Spec) {
		spec.PersistentDisk = false
		spec.Command = []string{"sleep", "600"}
		spec.Labels = map[string]string{vm.LabelPurpose: vm.PurposeTask, vm.LabelTask: "it-adopt-task"}
	}))
	require.Equal(t, vm.InstanceRunning, taskVM.State)

	dockerSpec := dockerVM("it-adopt-docker")
	created(t, e, dockerSpec)
	archive, _ := archived(t, e, dockerSpec.ID)
	restoredOnto(t, e, dockerSpec, archive)

	daemons := docker.NewDaemons(e, 3*time.Minute, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	require.NoError(t, daemons.Daemon(dockerSpec.ID).Ping(ctx))

	// the supervisor and dockerd go behind the engine's back.
	runOK(t, e, dockerSpec.ID, "kill $(cat /run/vminit.pid); while docker info >/dev/null 2>&1; do sleep 0.2; done")

	fmt.Printf("CHILD-ENDPOINT=%s\n", endpoint(t, machineVM, 8000))
	fmt.Println("CHILD-DONE")

	// gone, as a vmhost that is killed goes: nothing is stopped, and nothing
	// cleaned up.
	os.Exit(0)
}

func TestReadoption(t *testing.T) {
	engineFor(t)
	ctx := t.Context()

	child := exec.Command(os.Args[0], "-test.run=^TestReadoptionChild$", "-test.v", "-test.timeout=20m")
	child.Env = append(os.Environ(), "VMHOST_IT_CHILD=1")

	output, err := child.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.Contains(t, string(output), "CHILD-DONE", "%s", output)

	var address string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		if value, ok := strings.CutPrefix(scanner.Text(), "CHILD-ENDPOINT="); ok {
			address = value
		}
	}

	started := time.Now()

	adopted, err := New(ctx, itOptions(t))
	require.NoError(t, err)
	t.Logf("the next vmhost's engine was ready in %s", time.Since(started).Round(time.Millisecond))

	for _, id := range []string{"it-adopt-machine", "it-adopt-task", "it-adopt-docker"} {
		t.Cleanup(func() { removed(t, adopted, id) })
	}

	t.Run("a running vm is taken back as it was", func(t *testing.T) {
		instance := inspected(t, adopted, "it-adopt-machine")
		assert.Equal(t, vm.InstanceRunning, instance.State)
		assert.Equal(t, itOwner, instance.Labels[vm.LabelOwner])
		assert.Equal(t, address, endpoint(t, instance, 8000), "with the ports it had")

		assert.Equal(t, "adopted", runOK(t, adopted, "it-adopt-machine", "cat /root/adopted"))

		body, err := fetch(address, itOptions(t).BindAddress)
		require.NoError(t, err)
		assert.Equal(t, "served-before-the-vmhost-went", body)
	})

	t.Run("a main process lost with the vmhost has ended, and its vm stopped", func(t *testing.T) {
		instance := inspected(t, adopted, "it-adopt-task")
		assert.Equal(t, vm.InstanceExited, instance.State)
		assert.Equal(t, -1, instance.ExitCode)
		assert.NotEmpty(t, instance.Reason)

		h, err := msb.GetSandbox(ctx, "it-adopt-task")
		require.NoError(t, err)
		assert.False(t, up(h.Status()), "its sandbox is %s", h.Status())
	})

	t.Run("a running docker vm gets its dockerd back", func(t *testing.T) {
		daemons := docker.NewDaemons(adopted, 3*time.Minute, slog.New(slog.NewTextHandler(os.Stderr, nil)))

		require.NoError(t, daemons.Daemon("it-adopt-docker").Ping(ctx))
	})
}

// TestNothingLeftBehind checks that the tests before it, which snapshotted,
// restored and reconfigured VMs, left no snapshot installed and nothing in the
// engine's temporary directory: what an operation installs or writes for
// itself is gone once it is over.
func TestNothingLeftBehind(t *testing.T) {
	engineFor(t)

	snapshots, err := msb.Snapshot.List(t.Context())
	require.NoError(t, err)

	for _, snapshot := range snapshots {
		name := snapshot.Name()
		assert.False(t, name != nil && strings.HasPrefix(*name, snapshotPrefix), "a snapshot is left at %s", snapshot.Path())
	}

	left, err := os.ReadDir(filepath.Join(itOptions(t).Home, "vmhost", "tmp"))
	require.NoError(t, err)
	assert.Empty(t, left)
}

// TestShutdown stops everything, so it is the last test there is.
func TestShutdown(t *testing.T) {
	e := engineFor(t)
	ctx := t.Context()

	machineSpec := machine("it-shutdown-machine")
	dockerSpec := dockerVM("it-shutdown-docker")

	created(t, e, machineSpec)
	created(t, e, dockerSpec)

	daemons := docker.NewDaemons(e, 3*time.Minute, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	require.NoError(t, daemons.Daemon(dockerSpec.ID).Ping(ctx))

	// written, and not flushed: only a VM that stops gracefully keeps it.
	runOK(t, e, machineSpec.ID, "echo unflushed > /root/unflushed")

	stopping, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	started := time.Now()
	require.NoError(t, e.Shutdown(stopping))
	t.Logf("every vm stopped in %s", time.Since(started).Round(time.Millisecond))

	for _, id := range []string{machineSpec.ID, dockerSpec.ID} {
		h, err := msb.GetSandbox(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, msb.SandboxStatusStopped, h.Status(), "%s", id)
	}

	_, err := e.Create(ctx, machine("it-after-shutdown"))
	assert.Error(t, err, "nothing boots once the engine is shutting down")

	next, err := New(ctx, itOptions(t))
	require.NoError(t, err)

	for _, id := range []string{machineSpec.ID, dockerSpec.ID} {
		t.Cleanup(func() { removed(t, next, id) })
	}

	require.NoError(t, next.Start(ctx, machineSpec.ID))
	assert.Equal(t, "unflushed", runOK(t, next, machineSpec.ID, "cat /root/unflushed"))

	require.NoError(t, next.Start(ctx, dockerSpec.ID))
	assert.Contains(t, runOK(t, next, dockerSpec.ID, "cat /var/log/vminit.log"), "stopping dockerd", "dockerd was stopped by vminit, not killed")
}
