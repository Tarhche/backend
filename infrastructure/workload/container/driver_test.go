package container

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"sync"
	"testing"
	"time"

	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/system"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/driver"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

const testNode = "node-1"

// daemonInfo is what a daemon on an arm64 host with eight CPUs and 16 GiB says
// about itself.
func daemonInfo() system.Info {
	return system.Info{
		ServerVersion: "28.5.2",
		NCPU:          8,
		MemTotal:      16 << 30,
		Architecture:  "aarch64",
		Runtimes:      map[string]system.RuntimeWithStatus{"runc": {}},
	}
}

// spec is a container class on the engine at endpoint, with options.
func spec(class runtime.Class, endpoint string, options url.Values) driver.Spec {
	if options == nil {
		options = url.Values{}
	}

	return driver.Spec{Class: class, Kind: driver.KindContainer, Endpoint: endpoint, Options: options}
}

// built is a driver of the class on the engine, for testNode.
func built(t *testing.T, server string, class runtime.Class, options url.Values) *Driver {
	t.Helper()

	d, err := New(context.Background(), testNode, spec(class, "tcp://"+server, options), slog.New(slog.DiscardHandler))
	require.NoError(t, err)

	t.Cleanup(func() { _ = d.Close() })

	return d
}

// clock is a time a test moves on by hand.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.now = c.now.Add(d)
}

func TestNew(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)

	refused := map[string]driver.Spec{
		"a class of another kind": {Class: "firecracker", Kind: driver.KindMicroVM, Endpoint: "unix:///run/workload-vmhost/vmhost.sock", Options: url.Values{}},
		"an option a container does not read": spec(runtime.Sysbox, "tcp://docker:2375", url.Values{
			driver.OptionNodeMemory: {"8GiB"},
		}),
		"an option given twice": spec(runtime.Sysbox, "tcp://docker:2375", url.Values{
			driver.OptionOCIRuntime: {"runc", "runsc"},
		}),
		"a network prefix docker would refuse": spec("gvisor", "tcp://docker:2375", url.Values{
			driver.OptionNetworkPrefix: {"-gvisor"},
		}),
		"an endpoint the docker client cannot use": spec(runtime.Sysbox, "nonsense", nil),
	}

	for name, s := range refused {
		t.Run(name+" is refused", func(t *testing.T) {
			t.Parallel()

			d, err := New(context.Background(), testNode, s, logger)
			assert.Error(t, err)
			assert.Nil(t, d)
		})
	}

	t.Run("the factory says no the same way, with no driver at all", func(t *testing.T) {
		t.Parallel()

		d, err := NewFactory(logger)(context.Background(), testNode, refused["an endpoint the docker client cannot use"])
		assert.Error(t, err)

		// an interface holding a nil pointer would not be nil.
		assert.True(t, d == nil)
	})

	t.Run("no endpoint is the docker client's own default", func(t *testing.T) {
		t.Parallel()

		d, err := New(context.Background(), testNode, spec(runtime.Sysbox, "", nil), logger)
		require.NoError(t, err)
		require.NoError(t, d.Close())

		assert.Equal(t, runtime.Sysbox, d.Class())
		assert.Equal(t, driver.KindContainer, d.Kind())
	})

	t.Run("a daemon that does not answer yet is not refused, but offered unhealthy", func(t *testing.T) {
		t.Parallel()

		d, err := New(context.Background(), testNode, spec(runtime.Sysbox, "tcp://127.0.0.1:1", nil), logger)
		require.NoError(t, err)
		defer d.Close()

		offer := d.Offer(context.Background())

		assert.False(t, offer.Healthy)
		assert.Contains(t, offer.Reason, "docker cannot be reached")
	})
}

func TestDriver_Offer(t *testing.T) {
	t.Parallel()

	t.Run("offers the class as the daemon runs it", func(t *testing.T) {
		t.Parallel()

		e := &engine{info: daemonInfo()}
		server := e.serve(t)

		offer := built(t, server.Listener.Addr().String(), runtime.Sysbox, nil).Offer(context.Background())

		assert.Equal(t, runtime.Offer{
			Class:   runtime.Sysbox,
			Driver:  "container",
			Version: "28.5.2",
			Healthy: true,
			Capabilities: runtime.Capabilities{
				Isolation:       runtime.IsolationContainer,
				NetworkPolicies: []network.Policy{network.PolicyNone, network.PolicyIsolated, network.PolicyPublic},
				StackNetworks:   true,
				ReadOnlyRoot:    true,
				DiskLimit:       false,
				TTY:             true,
				RestartPolicies: []string{"no", "always", "on-failure", "unless-stopped"},
				MinMemory:       task.MinMemory,
				Architectures:   []string{"arm64"},
			},
			Capacity: runtime.Capacity{
				CPU:      8,
				Memory:   16 << 30,
				Reserved: false,
			},
		}, offer)
	})

	t.Run("asks the daemon once in a while rather than on every heartbeat", func(t *testing.T) {
		t.Parallel()

		e := &engine{info: daemonInfo()}
		server := e.serve(t)

		d := built(t, server.Listener.Addr().String(), runtime.Sysbox, nil)
		now := &clock{now: time.Now()}
		d.about.now = now.Now

		d.Offer(context.Background())
		d.Offer(context.Background())
		assert.Equal(t, 1, e.infoAsked())

		now.advance(aboutTTL)

		d.Offer(context.Background())
		assert.Equal(t, 2, e.infoAsked())
	})

	t.Run("a daemon gone away offers the class unhealthy, with what it could do", func(t *testing.T) {
		t.Parallel()

		e := &engine{info: daemonInfo()}
		server := e.serve(t)

		d := built(t, server.Listener.Addr().String(), runtime.Sysbox, nil)
		now := &clock{now: time.Now()}
		d.about.now = now.Now

		require.True(t, d.Offer(context.Background()).Healthy)

		server.Close()
		now.advance(aboutTTL)

		offer := d.Offer(context.Background())

		assert.False(t, offer.Healthy)
		assert.Contains(t, offer.Reason, "docker cannot be reached")

		// what it is running is still the class's, so what the class could do
		// is still said.
		assert.Equal(t, []string{"arm64"}, offer.Capabilities.Architectures)
		assert.Equal(t, float64(8), offer.Capacity.CPU)
	})

	t.Run("a daemon without the runtime a class asks for cannot run it", func(t *testing.T) {
		t.Parallel()

		e := &engine{info: daemonInfo()}
		server := e.serve(t)

		offer := built(t, server.Listener.Addr().String(), "sysbox", url.Values{
			driver.OptionOCIRuntime: {"sysbox-runc"},
		}).Offer(context.Background())

		assert.False(t, offer.Healthy)
		assert.Contains(t, offer.Reason, `"sysbox-runc"`)
	})

	t.Run("one that has it can", func(t *testing.T) {
		t.Parallel()

		info := daemonInfo()
		info.Runtimes["sysbox-runc"] = system.RuntimeWithStatus{}

		e := &engine{info: info}
		server := e.serve(t)

		offer := built(t, server.Listener.Addr().String(), "sysbox", url.Values{
			driver.OptionOCIRuntime: {"sysbox-runc"},
		}).Offer(context.Background())

		assert.True(t, offer.Healthy)
		assert.Empty(t, offer.Reason)
	})

	t.Run("an architecture is named the way go names it", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []string{"amd64"}, architectures("x86_64"))
		assert.Equal(t, []string{"arm64"}, architectures("aarch64"))
		assert.Equal(t, []string{"riscv64"}, architectures("riscv64"))
		assert.Nil(t, architectures(""))
	})
}

// ids is the IDs of runs, in order.
func ids(runs []task.Execution) []string {
	result := make([]string, len(runs))
	for i := range runs {
		result[i] = runs[i].ID
	}

	return result
}

func TestDriver_scope(t *testing.T) {
	t.Parallel()

	e := &engine{info: daemonInfo()}

	// a container from before there were classes says nothing about its
	// class, and was run as sysbox.
	e.hold(&engineContainer{id: "legacy", name: "api-abcde", labels: map[string]string{
		NodeNameLabel: testNode, taskUUIDLabel: "task-1", taskSlugLabel: "api-abcde", taskAttemptLabel: "0",
	}})
	e.hold(&engineContainer{id: "sysbox", name: "api-abcde-1", labels: map[string]string{
		NodeNameLabel: testNode, taskUUIDLabel: "task-1", taskSlugLabel: "api-abcde", taskAttemptLabel: "1", taskRuntimeLabel: "sysbox",
	}})

	// another orchestrator on the same daemon, holding an attempt at the same
	// task.
	e.hold(&engineContainer{id: "elsewhere", name: "api-abcde-2", labels: map[string]string{
		NodeNameLabel: "node-2", taskUUIDLabel: "task-1", taskSlugLabel: "api-abcde", taskRuntimeLabel: "sysbox",
	}})

	// another class on the same daemon and node.
	e.hold(&engineContainer{id: "gvisor", name: "web-fghij", labels: map[string]string{
		NodeNameLabel: testNode, taskUUIDLabel: "task-2", taskSlugLabel: "web-fghij", taskRuntimeLabel: "gvisor",
	}})

	server := e.serve(t)

	sysbox := built(t, server.Listener.Addr().String(), runtime.Sysbox, nil).Tasks()
	gvisor := built(t, server.Listener.Addr().String(), "gvisor", url.Values{driver.OptionNetworkPrefix: {"gvisor-"}}).Tasks()

	ctx := context.Background()

	t.Run("a node holds its own runs of its own class, and those from before there were classes", func(t *testing.T) {
		t.Parallel()

		held, err := sysbox.OnNode(ctx, testNode)
		require.NoError(t, err)
		assert.Equal(t, []string{"legacy", "sysbox"}, ids(held))

		for _, run := range held {
			assert.Equal(t, runtime.Sysbox, run.Runtime)
		}

		held, err = gvisor.OnNode(ctx, testNode)
		require.NoError(t, err)
		assert.Equal(t, []string{"gvisor"}, ids(held))
		assert.Equal(t, runtime.Class("gvisor"), held[0].Runtime)
	})

	t.Run("a task's runs are this node's alone, though every node hears of it", func(t *testing.T) {
		t.Parallel()

		runs, err := sysbox.Of(ctx, "task-1")
		require.NoError(t, err)
		assert.Equal(t, []string{"legacy", "sysbox"}, ids(runs))

		runs, err = sysbox.BySlug(ctx, "api-abcde")
		require.NoError(t, err)
		assert.Equal(t, []string{"legacy", "sysbox"}, ids(runs))
	})

	t.Run("another class's runs are not this class's", func(t *testing.T) {
		t.Parallel()

		runs, err := sysbox.Of(ctx, "task-2")
		require.NoError(t, err)
		assert.Empty(t, runs)

		runs, err = gvisor.Of(ctx, "task-1")
		require.NoError(t, err)
		assert.Empty(t, runs)

		runs, err = gvisor.BySlug(ctx, "web-fghij")
		require.NoError(t, err)
		assert.Equal(t, []string{"gvisor"}, ids(runs))
	})
}

func TestDriver_create(t *testing.T) {
	t.Parallel()

	t.Run("a class's own runtime, networks and labels", func(t *testing.T) {
		t.Parallel()

		e := &engine{info: daemonInfo()}
		server := e.serve(t)

		tasks := built(t, server.Listener.Addr().String(), "gvisor", url.Values{
			driver.OptionOCIRuntime:    {"runsc"},
			driver.OptionNetworkPrefix: {"gvisor-"},
		}).Tasks()

		id, err := tasks.Create(context.Background(), &task.Execution{
			Name:     "web-fghij",
			Image:    "busybox",
			TaskUUID: "task-2",
			Slug:     "web-fghij",
			Kind:     task.KindService,

			// whatever it says, the run is the node's whose driver made it.
			NodeName: "somebody-else",

			Networks: network.Attachments(network.PolicyPublic, "", ""),
		})
		require.NoError(t, err)

		created := e.lastCreated()

		assert.Equal(t, testNode, created.Labels[NodeNameLabel])
		assert.Equal(t, "gvisor", created.Labels[taskRuntimeLabel])
		assert.Equal(t, "runsc", created.HostConfig.Runtime)

		// its own isolated network, and docker's bridge to route out, which no
		// class has a copy of.
		assert.Equal(t, containerTypes.NetworkMode("gvisor-workload-isolated"), created.HostConfig.NetworkMode)
		assert.Equal(t, []string{"bridge"}, e.networksConnected())

		inspected, err := tasks.Inspect(context.Background(), id)
		require.NoError(t, err)

		assert.Equal(t, []network.Attachment{{Name: network.IsolatedNetworkName}}, inspected.Networks)
		assert.Equal(t, runtime.Class("gvisor"), inspected.Runtime)
	})

	t.Run("sysbox with nothing said is created as it always was", func(t *testing.T) {
		t.Parallel()

		e := &engine{info: daemonInfo()}
		server := e.serve(t)

		tasks := built(t, server.Listener.Addr().String(), runtime.Sysbox, nil).Tasks()

		_, err := tasks.Create(context.Background(), &task.Execution{
			Name:     "api-abcde",
			Image:    "busybox",
			TaskUUID: "task-1",
			NodeName: testNode,
			Networks: network.Attachments(network.PolicyIsolated, "shop", "api"),
		})
		require.NoError(t, err)

		created := e.lastCreated()

		assert.Empty(t, created.HostConfig.Runtime)
		assert.Equal(t, "sysbox", created.Labels[taskRuntimeLabel])
		assert.Equal(t, containerTypes.NetworkMode("workload-stack-shop"), created.HostConfig.NetworkMode)
		assert.Equal(t, []string{"api"}, created.NetworkingConfig.EndpointsConfig["workload-stack-shop"].Aliases)
	})
}

func TestDriver_endpoints(t *testing.T) {
	t.Parallel()

	e := &engine{info: daemonInfo()}
	e.hold(&engineContainer{
		id:     "published",
		name:   "web-fghij",
		labels: map[string]string{NodeNameLabel: testNode, taskUUIDLabel: "task-1"},
		ports: nat.PortMap{
			"80/tcp":   {{HostIP: "0.0.0.0", HostPort: "32768"}},
			"8080/tcp": nil,
		},
	})
	server := e.serve(t)

	tasks := built(t, server.Listener.Addr().String(), runtime.Sysbox, nil).Tasks()

	t.Run("a listing says which ports came up", func(t *testing.T) {
		t.Parallel()

		held, err := tasks.OnNode(context.Background(), testNode)
		require.NoError(t, err)
		require.Len(t, held, 1)

		assert.Equal(t, []port.Port{80}, held[0].Endpoints)
		assert.Equal(t, port.PortMap{80: {{HostIP: "0.0.0.0", HostPort: 32768}}}, held[0].PortBindings)
	})

	t.Run("so does an inspection", func(t *testing.T) {
		t.Parallel()

		inspected, err := tasks.Inspect(context.Background(), "published")
		require.NoError(t, err)

		assert.Equal(t, []port.Port{80}, inspected.Endpoints)
	})
}

func TestDockerManager_DialContext(t *testing.T) {
	t.Parallel()

	// what stands where docker published the container's port.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}

			_, _ = io.WriteString(conn, "hello from the task")
			_ = conn.Close()
		}
	}()

	published := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)

	e := &engine{info: daemonInfo()}
	e.hold(&engineContainer{
		id:     "web",
		name:   "web-fghij",
		labels: map[string]string{NodeNameLabel: testNode},
		ports:  nat.PortMap{"80/tcp": {{HostIP: "0.0.0.0", HostPort: published}}, "8080/tcp": nil},
	})
	server := e.serve(t)

	tasks := built(t, server.Listener.Addr().String(), runtime.Sysbox, url.Values{
		driver.OptionAdvertiseHost: {"127.0.0.1"},
	}).Tasks()

	dialer, ok := tasks.(task.Dialer)
	require.True(t, ok, "a container's ports can be dialled")

	t.Run("reaches a port where docker published it", func(t *testing.T) {
		t.Parallel()

		conn, err := dialer.DialContext(context.Background(), "web", 80)
		require.NoError(t, err)
		defer conn.Close()

		said, err := io.ReadAll(conn)
		require.NoError(t, err)
		assert.Equal(t, "hello from the task", string(said))
	})

	t.Run("a port that was not published cannot be reached", func(t *testing.T) {
		t.Parallel()

		_, err := dialer.DialContext(context.Background(), "web", 8080)
		assert.Error(t, err)

		_, err = dialer.DialContext(context.Background(), "web", 9090)
		assert.Error(t, err)
	})

	t.Run("a container that is gone is not there", func(t *testing.T) {
		t.Parallel()

		_, err := dialer.DialContext(context.Background(), "missing", 80)
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestLabels(t *testing.T) {
	t.Parallel()

	t.Run("a run says what it is running, and as which class", func(t *testing.T) {
		t.Parallel()

		execution := task.Execution{
			TaskUUID:    "task-1",
			TaskName:    "api",
			Slug:        "api-abcde",
			Kind:        task.KindService,
			NodeName:    testNode,
			OwnerUUID:   "owner-1",
			StackUUID:   "stack-1",
			Attempt:     2,
			Interactive: true,
			TTL:         90 * time.Second,
		}

		var read task.Execution
		identify(&read, labelsOf(&execution, "gvisor"))

		execution.Runtime = "gvisor"
		assert.Equal(t, execution, read)
	})

	t.Run("a run from before there were classes is sysbox's", func(t *testing.T) {
		t.Parallel()

		var read task.Execution
		identify(&read, map[string]string{taskUUIDLabel: "task-1"})

		assert.Equal(t, runtime.Sysbox, read.Runtime)
		assert.Equal(t, task.DefaultKind, read.Kind)
	})
}
