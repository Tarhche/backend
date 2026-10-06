package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/docker/docker/api"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	memory "github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

// versioned takes the API version off a path, which the client puts on every
// request once it has negotiated one.
var versioned = regexp.MustCompile(`^/v[0-9.]+`)

// fakeDocker is as much of dockerd's API as these tests ask of it. Like
// dockerd, it keeps the containers it created and the images it holds, so
// what a test reads back went through the same client, and the same json, as
// what it sent.
type fakeDocker struct {
	mu sync.Mutex

	images     map[string]bool
	containers map[string]*container.CreateRequest
	names      map[string]string
	running    map[string]bool

	pulled      []string
	pullError   string
	pullRefusal string
	listFilter  string
	requests    atomic.Int32
}

func newFakeDocker() *fakeDocker {
	return &fakeDocker{
		images:     map[string]bool{"nginx:alpine": true},
		containers: make(map[string]*container.CreateRequest),
		names:      make(map[string]string),
		running:    make(map[string]bool),
	}
}

func (f *fakeDocker) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests.Add(1)
	rw.Header().Set("Api-Version", api.DefaultVersion)

	path := versioned.ReplaceAllString(r.URL.Path, "")

	if match := regexp.MustCompile(`^/containers/([^/]+)/(json|start|logs|stats)$`).FindStringSubmatch(path); match != nil {
		f.container(rw, r, match[1], match[2])

		return
	}

	switch {
	case path == "/_ping":
		rw.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(rw, "OK")

	case path == "/containers/json":
		f.listFilter = r.URL.Query().Get("filters")

		writeJSON(rw, http.StatusOK, []container.Summary{
			{
				ID:      "c1",
				Names:   []string{"/web-1"},
				Image:   "nginx:alpine",
				ImageID: "sha256:nginx",
				State:   container.StateRunning,
				Status:  "Up 2 minutes",
				Command: "nginx -g 'daemon off;'",
				Created: 1791115749,
				Labels:  map[string]string{docker.LabelComposeProject: "shop-abcde", docker.LabelComposeService: "web"},
				Ports: []container.Port{
					{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
					{IP: "::", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
					{PrivatePort: 443, Type: "tcp"},
				},
				NetworkSettings: &container.NetworkSettingsSummary{Networks: map[string]*network.EndpointSettings{"shop_default": {}, "bridge": {}}},
			},
		})

	case path == "/images/json":
		writeJSON(rw, http.StatusOK, []image.Summary{
			{ID: "sha256:nginx", RepoTags: []string{"nginx:alpine"}, RepoDigests: []string{"nginx@sha256:0123"}, Size: 42 << 20, Created: 1791115749},
			{ID: "sha256:redis", RepoTags: []string{"<none>:<none>"}, RepoDigests: []string{"<none>@<none>"}, Size: 7 << 20, Created: 1791115749},
		})

	case path == "/networks" && r.Method == http.MethodGet:
		writeJSON(rw, http.StatusOK, []network.Summary{
			{ID: "n1", Name: "shop_default", Driver: "bridge", Scope: "local", Labels: map[string]string{docker.LabelComposeProject: "shop-abcde"}},
			{ID: "n2", Name: "spare", Driver: "bridge", Scope: "local", Internal: true},
		})

	case path == "/volumes" && r.Method == http.MethodGet:
		writeJSON(rw, http.StatusOK, volume.ListResponse{Volumes: []*volume.Volume{
			{Name: "data", Driver: "local", Mountpoint: "/var/lib/docker/volumes/data/_data", CreatedAt: "2026-10-04T12:00:00Z"},
		}})

	case path == "/containers/create":
		var request container.CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(rw, http.StatusBadRequest, map[string]string{"message": err.Error()})

			return
		}

		id := fmt.Sprintf("c%d", len(f.containers)+2)
		f.containers[id] = &request
		f.names[id] = r.URL.Query().Get("name")

		writeJSON(rw, http.StatusCreated, container.CreateResponse{ID: id})

	case regexp.MustCompile(`^/images/.+/json$`).MatchString(path):
		reference := path[len("/images/") : len(path)-len("/json")]
		if !f.images[reference] {
			writeJSON(rw, http.StatusNotFound, map[string]string{"message": "No such image: " + reference})

			return
		}

		writeJSON(rw, http.StatusOK, image.InspectResponse{ID: "sha256:" + reference, RepoTags: []string{reference}, Size: 42 << 20})

	case path == "/images/create":
		// the client asks for a reference in full; the image is known by what
		// it was asked for as.
		reference := r.URL.Query().Get("fromImage") + ":" + r.URL.Query().Get("tag")
		f.pulled = append(f.pulled, reference)
		reference = strings.TrimPrefix(reference, "docker.io/library/")

		// an image no registry has is refused before the pull starts.
		if len(f.pullRefusal) > 0 {
			writeJSON(rw, http.StatusNotFound, map[string]string{"message": f.pullRefusal})

			return
		}

		// a pull answers 200 whatever happens, and says how it went as it goes.
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(rw, `{"status":"Pulling from library/redis","id":"7"}`+"\n")
		_, _ = io.WriteString(rw, `{"status":"Downloading","progressDetail":{"current":1,"total":2},"id":"a"}`+"\n")

		if len(f.pullError) > 0 {
			_, _ = fmt.Fprintf(rw, `{"errorDetail":{"message":%q},"error":%q}`+"\n", f.pullError, f.pullError)

			return
		}

		f.images[reference] = true
		_, _ = io.WriteString(rw, `{"status":"Status: Downloaded newer image for `+reference+`"}`+"\n")

	default:
		writeJSON(rw, http.StatusNotFound, map[string]string{"message": "page not found: " + path})
	}
}

func (f *fakeDocker) container(rw http.ResponseWriter, r *http.Request, id string, what string) {
	created, ok := f.containers[id]
	if !ok {
		writeJSON(rw, http.StatusNotFound, map[string]string{"message": "No such container: " + id})

		return
	}

	switch what {
	case "start":
		f.running[id] = true
		rw.WriteHeader(http.StatusNoContent)

	case "json":
		status := container.StateCreated
		if f.running[id] {
			status = container.StateRunning
		}

		ports := nat.PortMap{}
		for guest, bindings := range created.HostConfig.PortBindings {
			for range bindings {
				ports[guest] = append(ports[guest], nat.PortBinding{HostIP: "0.0.0.0", HostPort: "32768"})
			}
		}

		writeJSON(rw, http.StatusOK, container.InspectResponse{
			ContainerJSONBase: &container.ContainerJSONBase{
				ID:         id,
				Name:       "/" + f.names[id],
				Created:    "2026-10-04T12:00:00.5Z",
				Path:       "redis-server",
				Args:       []string{"--appendonly", "yes"},
				State:      &container.State{Status: status},
				HostConfig: created.HostConfig,
			},
			Config:          created.Config,
			NetworkSettings: &container.NetworkSettings{NetworkSettingsBase: container.NetworkSettingsBase{Ports: ports}},
		})

	case "logs":
		rw.WriteHeader(http.StatusOK)

		stdout := stdcopy.NewStdWriter(rw, stdcopy.Stdout)
		stderr := stdcopy.NewStdWriter(rw, stdcopy.Stderr)
		_, _ = io.WriteString(stdout, "2026-10-04T12:00:00.000000001Z ready to accept connections\n")
		_, _ = io.WriteString(stderr, "2026-10-04T12:00:01Z warning: no config file\n")
		_, _ = io.WriteString(stdout, "2026-10-04T12:00:02Z a line with no end")

	case "stats":
		var sample container.StatsResponse
		sample.Read = time.Date(2026, 10, 4, 12, 0, 1, 0, time.UTC)
		sample.CPUStats.CPUUsage.TotalUsage = 3_000_000_000
		sample.PreCPUStats.CPUUsage.TotalUsage = 1_000_000_000
		sample.CPUStats.SystemUsage = 8_000_000_000
		sample.PreCPUStats.SystemUsage = 4_000_000_000
		sample.CPUStats.OnlineCPUs = 4
		sample.MemoryStats.Usage = 300 << 20
		sample.MemoryStats.Limit = 1 << 30
		sample.MemoryStats.Stats = map[string]uint64{"inactive_file": 100 << 20}
		sample.PidsStats.Current = 7
		sample.Networks = map[string]container.NetworkStats{"eth0": {RxBytes: 10, TxBytes: 20}, "eth1": {RxBytes: 1, TxBytes: 2}}

		writeJSON(rw, http.StatusOK, sample)
	}
}

func writeJSON(rw http.ResponseWriter, status int, body any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(body)
}

// dialStdioTo is `docker system dial-stdio` inside a VM whose dockerd is
// listening at address: what it reads it sends there, and what comes back it
// writes. The first refusals dials say dockerd is not up yet, as a VM still
// booting does.
func dialStdioTo(address string, refusals *atomic.Int32) memory.ExecFunc {
	return func(ctx context.Context, id string, options vm.ExecOptions, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
		// only vm-1 is a Docker VM: any other has no docker command at all.
		if id != "vm-1" || !slices.Equal(options.Command, dialStdio) {
			fmt.Fprintln(stderr, "sh: docker: not found")

			return 127
		}

		if refusals != nil && refusals.Add(-1) >= 0 {
			fmt.Fprintln(stderr, "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")

			return 1
		}

		connection, err := net.Dial("tcp", address)
		if err != nil {
			fmt.Fprintln(stderr, err)

			return 1
		}
		defer connection.Close()

		go func() {
			_, _ = io.Copy(connection, stdin)
			_ = connection.(*net.TCPConn).CloseWrite()
		}()

		answered := make(chan struct{})
		go func() {
			_, _ = io.Copy(stdout, connection)
			close(answered)
		}()

		select {
		case <-answered:
		case <-ctx.Done():
		}

		return 0
	}
}

// dockerVM is a Docker VM of the memory engine whose dockerd is fake, with the
// daemons of the node holding it.
func dockerVM(t *testing.T, fake *fakeDocker, refusals *atomic.Int32, readyTimeout time.Duration) (*Daemons, *memory.Engine) {
	t.Helper()

	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	e := memory.New(memory.WithExec(dialStdioTo(server.Listener.Addr().String(), refusals)))

	_, err := e.Create(t.Context(), vm.Spec{ID: "vm-1", Kind: vm.KindDocker, Image: "docker:29-dind"})
	require.NoError(t, err)

	daemons := NewDaemons(e, readyTimeout, slog.New(slog.DiscardHandler))
	t.Cleanup(func() { daemons.Forget("vm-1") })

	return daemons, e
}

func TestDaemon_Ping(t *testing.T) {
	t.Parallel()

	t.Run("a dockerd still coming up is waited for", func(t *testing.T) {
		t.Parallel()

		refusals := &atomic.Int32{}
		refusals.Store(2)

		daemons, _ := dockerVM(t, newFakeDocker(), refusals, 10*time.Second)

		require.NoError(t, daemons.Daemon("vm-1").Ping(t.Context()))
		assert.Less(t, refusals.Load(), int32(0), "it was asked again until it answered")
	})

	t.Run("a dockerd that never answers is unavailable once its time is up", func(t *testing.T) {
		t.Parallel()

		refusals := &atomic.Int32{}
		refusals.Store(1 << 30)

		daemons, _ := dockerVM(t, newFakeDocker(), refusals, 300*time.Millisecond)

		started := time.Now()
		err := daemons.Daemon("vm-1").Ping(t.Context())

		assert.ErrorIs(t, err, docker.ErrUnavailable)
		assert.Contains(t, err.Error(), "Cannot connect to the Docker daemon")
		assert.Less(t, time.Since(started), 5*time.Second)
	})

	testcases := []struct {
		name    string
		prepare func(t *testing.T, e *memory.Engine)
		vmUUID  string
		want    error
	}{
		{
			name: "a VM with no docker in it is no Docker VM, and is not waited for",
			prepare: func(t *testing.T, e *memory.Engine) {
				_, err := e.Create(t.Context(), vm.Spec{ID: "machine", Kind: vm.KindMachine, Image: "ubuntu:24.04"})
				require.NoError(t, err)
			},
			vmUUID: "machine",
			want:   vm.ErrNotDocker,
		},
		{
			name: "a VM that is stopped will not have a dockerd until it is started",
			prepare: func(t *testing.T, e *memory.Engine) {
				require.NoError(t, e.Stop(t.Context(), "vm-1"))
			},
			vmUUID: "vm-1",
			want:   vm.ErrNotRunning,
		},
		{
			name:   "a VM that is not here has no dockerd here",
			vmUUID: "vm-2",
			want:   domain.ErrNotExists,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			daemons, e := dockerVM(t, newFakeDocker(), nil, time.Minute)
			if tt.prepare != nil {
				tt.prepare(t, e)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			assert.ErrorIs(t, daemons.Daemon(tt.vmUUID).Ping(ctx), tt.want)
		})
	}
}

func TestDaemon_CreateContainer(t *testing.T) {
	t.Parallel()

	spec := docker.ContainerSpec{
		Name:          "cache",
		Image:         "redis:7",
		Command:       []string{"redis-server", "--appendonly", "yes"},
		Env:           []string{"A=1"},
		Ports:         []docker.PortBinding{{ContainerPort: 6379}},
		Mounts:        []docker.Mount{{Type: "volume", Source: "data", Target: "/data"}},
		Networks:      []string{"backend"},
		RestartPolicy: "unless-stopped",
		CPUs:          0.5,
		Memory:        256 << 20,
		Labels:        map[string]string{"app": "shop"},
	}

	t.Run("an image the VM does not hold is pulled, and the container is created and started", func(t *testing.T) {
		t.Parallel()

		fake := newFakeDocker()
		daemons, _ := dockerVM(t, fake, nil, time.Minute)

		created, err := daemons.Daemon("vm-1").CreateContainer(t.Context(), spec)
		require.NoError(t, err)

		assert.Equal(t, []string{"docker.io/library/redis:7"}, fake.pulled)

		assert.Equal(t, "c2", created.ID)
		assert.Equal(t, "cache", created.Name)
		assert.Equal(t, container.StateRunning, created.State)
		assert.Equal(t, "redis-server --appendonly yes", created.Command)
		assert.Equal(t, "unless-stopped", created.RestartPolicy)
		assert.Equal(t, []docker.PortBinding{{ContainerPort: 6379, HostPort: 32768, Protocol: "tcp"}}, created.Ports)
		assert.Equal(t, time.Date(2026, 10, 4, 12, 0, 0, 500_000_000, time.UTC), created.CreatedAt)

		given := fake.containers["c2"]
		assert.Equal(t, int64(500_000_000), given.HostConfig.NanoCPUs)
		assert.Equal(t, int64(256<<20), given.HostConfig.Memory, "bytes, as they were asked for")
		assert.Equal(t, container.NetworkMode("backend"), given.HostConfig.NetworkMode)
		assert.Equal(t, []nat.PortBinding{{HostPort: ""}}, given.HostConfig.PortBindings["6379/tcp"], "the host port is docker's to choose")
		require.Len(t, given.HostConfig.Mounts, 1)
		assert.Equal(t, "data", given.HostConfig.Mounts[0].Source)
	})

	t.Run("a pull that fails part way is a request docker refused, and nothing is made", func(t *testing.T) {
		t.Parallel()

		fake := newFakeDocker()
		fake.pullError = "manifest for redis:7 not found: manifest unknown"

		daemons, _ := dockerVM(t, fake, nil, time.Minute)

		_, err := daemons.Daemon("vm-1").CreateContainer(t.Context(), spec)
		assert.ErrorIs(t, err, docker.ErrInvalid)
		assert.Contains(t, err.Error(), "manifest unknown")
		assert.Empty(t, fake.containers)
	})

	t.Run("an image no registry has is a request docker refused, not something that is gone", func(t *testing.T) {
		t.Parallel()

		fake := newFakeDocker()
		fake.pullRefusal = "pull access denied for redis, repository does not exist or may require 'docker login'"

		daemons, _ := dockerVM(t, fake, nil, time.Minute)

		_, err := daemons.Daemon("vm-1").CreateContainer(t.Context(), spec)
		assert.ErrorIs(t, err, docker.ErrInvalid)
		assert.NotErrorIs(t, err, domain.ErrNotExists)
		assert.Contains(t, err.Error(), "repository does not exist")
		assert.Empty(t, fake.containers)
	})
}

func TestDaemon_PullImage(t *testing.T) {
	t.Parallel()

	t.Run("an image is pulled, and what the VM now holds is said", func(t *testing.T) {
		t.Parallel()

		fake := newFakeDocker()
		daemons, _ := dockerVM(t, fake, nil, time.Minute)

		pulled, err := daemons.Daemon("vm-1").PullImage(t.Context(), "redis:7")
		require.NoError(t, err)

		assert.Equal(t, []string{"docker.io/library/redis:7"}, fake.pulled)
		assert.Equal(t, []string{"redis:7"}, pulled.Tags)
	})

	t.Run("an image no registry has is a request docker refused, not something that is gone", func(t *testing.T) {
		t.Parallel()

		fake := newFakeDocker()
		fake.pullRefusal = "pull access denied for nope, repository does not exist or may require 'docker login'"

		daemons, _ := dockerVM(t, fake, nil, time.Minute)

		_, err := daemons.Daemon("vm-1").PullImage(t.Context(), "nope:1")
		assert.ErrorIs(t, err, docker.ErrInvalid)
		assert.NotErrorIs(t, err, domain.ErrNotExists)
		assert.Contains(t, err.Error(), "repository does not exist")

		// dockerd answered, so the connection it answered on is kept.
		_, err = daemons.Daemon("vm-1").Containers(t.Context(), docker.ContainerFilter{})
		require.NoError(t, err)
	})
}

func TestDaemon_Containers(t *testing.T) {
	t.Parallel()

	fake := newFakeDocker()
	daemons, _ := dockerVM(t, fake, nil, time.Minute)

	containers, err := daemons.Daemon("vm-1").Containers(t.Context(), docker.ContainerFilter{All: true, Stack: "shop-abcde"})
	require.NoError(t, err)

	assert.Contains(t, fake.listFilter, docker.LabelComposeProject+"=shop-abcde", "a stack's containers are the ones compose labelled with its project")

	require.Len(t, containers, 1)
	assert.Equal(t, docker.Container{
		ID:       "c1",
		Name:     "web-1",
		Image:    "nginx:alpine",
		State:    "running",
		Status:   "Up 2 minutes",
		Command:  "nginx -g 'daemon off;'",
		Ports:    []docker.PortBinding{{ContainerPort: port.Port(80), HostPort: port.Port(8080), Protocol: "tcp"}},
		Networks: []string{"bridge", "shop_default"},
		Labels: map[string]string{
			docker.LabelComposeProject: "shop-abcde",
			docker.LabelComposeService: "web",
		},
		Stack:     "shop-abcde",
		Service:   "web",
		CreatedAt: time.Unix(1791115749, 0).UTC(),
	}, containers[0])
}

func TestDaemon_Containers_label(t *testing.T) {
	t.Parallel()

	fake := newFakeDocker()
	daemons, _ := dockerVM(t, fake, nil, time.Minute)

	_, err := daemons.Daemon("vm-1").Containers(t.Context(), docker.ContainerFilter{All: true, Label: "workload.stack"})
	require.NoError(t, err)

	assert.Contains(t, fake.listFilter, `"workload.stack":true`, "the containers carrying a label, whatever its value")
	assert.NotContains(t, fake.listFilter, docker.LabelComposeProject)

	_, err = daemons.Daemon("vm-1").Containers(t.Context(), docker.ContainerFilter{})
	require.NoError(t, err)

	assert.Empty(t, fake.listFilter, "and every one when it is asked for none")
}

func TestDaemon_Container(t *testing.T) {
	t.Parallel()

	daemons, _ := dockerVM(t, newFakeDocker(), nil, time.Minute)

	_, err := daemons.Daemon("vm-1").Container(t.Context(), "nothing")
	assert.ErrorIs(t, err, domain.ErrNotExists)
	assert.Contains(t, err.Error(), "No such container: nothing", "in docker's own words")
}

func TestDaemon_ContainerLogs(t *testing.T) {
	t.Parallel()

	fake := newFakeDocker()
	fake.containers["c9"] = &container.CreateRequest{Config: &container.Config{}, HostConfig: &container.HostConfig{}}

	daemons, _ := dockerVM(t, fake, nil, time.Minute)

	lines, err := daemons.Daemon("vm-1").ContainerLogs(t.Context(), "c9", docker.LogOptions{Tail: 10})
	require.NoError(t, err)

	assert.Equal(t, []docker.LogLine{
		{At: time.Date(2026, 10, 4, 12, 0, 0, 1, time.UTC), Stream: "stdout", Line: "ready to accept connections"},
		{At: time.Date(2026, 10, 4, 12, 0, 1, 0, time.UTC), Stream: "stderr", Line: "warning: no config file"},
		{At: time.Date(2026, 10, 4, 12, 0, 2, 0, time.UTC), Stream: "stdout", Line: "a line with no end"},
	}, lines)
}

func TestDaemon_ContainerStats(t *testing.T) {
	t.Parallel()

	fake := newFakeDocker()
	fake.containers["c9"] = &container.CreateRequest{Config: &container.Config{}, HostConfig: &container.HostConfig{}}

	daemons, _ := dockerVM(t, fake, nil, time.Minute)

	stats, err := daemons.Daemon("vm-1").ContainerStats(t.Context(), "c9")
	require.NoError(t, err)

	assert.Equal(t, docker.Stats{
		// 2s of CPU over 4s of the system's 4 CPUs: docker says 200%, which
		// is half of all of them.
		CPUPercent:  50,
		MemoryUsed:  200 << 20,
		MemoryLimit: 1 << 30,
		NetworkRx:   11,
		NetworkTx:   22,
		PIDs:        7,
		SampledAt:   time.Date(2026, 10, 4, 12, 0, 1, 0, time.UTC),
	}, stats)
}

func TestDaemons_Connections(t *testing.T) {
	t.Parallel()

	t.Run("one client's connections serve one request after another", func(t *testing.T) {
		t.Parallel()

		daemons, e := dockerVM(t, newFakeDocker(), nil, time.Minute)
		daemon := daemons.Daemon("vm-1")

		for range 5 {
			_, err := daemon.Containers(t.Context(), docker.ContainerFilter{})
			require.NoError(t, err)
		}

		assert.LessOrEqual(t, e.Sessions("vm-1"), idleConnections, "a connection is a command in the VM, and a few are kept")

		daemons.Forget("vm-1")

		assert.Eventually(t, func() bool { return e.Sessions("vm-1") == 0 }, 2*time.Second, 5*time.Millisecond, "a client let go of ends the commands it kept")
	})

	t.Run("a VM that went down and came back is reached anew", func(t *testing.T) {
		t.Parallel()

		daemons, e := dockerVM(t, newFakeDocker(), nil, time.Minute)
		daemon := daemons.Daemon("vm-1")

		_, err := daemon.Containers(t.Context(), docker.ContainerFilter{})
		require.NoError(t, err)

		// the restart ends every command running in the VM, the kept
		// connections with them, behind the client's back.
		require.NoError(t, e.Restart(t.Context(), "vm-1"))

		require.NoError(t, daemon.Ping(t.Context()))

		_, err = daemon.Containers(t.Context(), docker.ContainerFilter{})
		assert.NoError(t, err)
	})
}

// TestDaemon_Inventory holds what a heartbeat asks of a Docker VM to one
// listing of each sort of object, and to saying which of the containers uses
// each image, network and volume.
func TestDaemon_Inventory(t *testing.T) {
	t.Parallel()

	fake := newFakeDocker()
	daemons, _ := dockerVM(t, fake, nil, time.Minute)

	before := fake.requests.Load()

	inventory, err := daemons.Daemon("vm-1").Inventory(t.Context())
	require.NoError(t, err)

	// the client negotiates its API version once, with a ping.
	assert.LessOrEqual(t, fake.requests.Load()-before, int32(5), "a listing of each sort of object, and no more")
	assert.Empty(t, fake.listFilter, "every container, stopped ones too")

	require.Len(t, inventory.Containers, 1)
	assert.Equal(t, "web-1", inventory.Containers[0].Name)

	assert.Equal(t, []docker.Image{
		{ID: "sha256:nginx", Tags: []string{"nginx:alpine"}, Digests: []string{"nginx@sha256:0123"}, Size: 42 << 20, CreatedAt: time.Unix(1791115749, 0).UTC(), InUse: true},
		{ID: "sha256:redis", Tags: []string{}, Digests: []string{}, Size: 7 << 20, CreatedAt: time.Unix(1791115749, 0).UTC()},
	}, inventory.Images)

	require.Len(t, inventory.Networks, 2)
	assert.Equal(t, []string{"web-1"}, inventory.Networks[0].Containers)
	assert.Empty(t, inventory.Networks[1].Containers)
	assert.True(t, inventory.Networks[1].Internal)

	require.Len(t, inventory.Volumes, 1)
	assert.Equal(t, "data", inventory.Volumes[0].Name)
	assert.Equal(t, time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC), inventory.Volumes[0].CreatedAt)
}

func TestInventoryOf(t *testing.T) {
	t.Parallel()

	inventory := inventoryOf(
		[]container.Summary{
			{ID: "c1", Names: []string{"/web"}, ImageID: "sha256:nginx", NetworkSettings: &container.NetworkSettingsSummary{Networks: map[string]*network.EndpointSettings{"backend": {}}}, Mounts: []container.MountPoint{{Type: "volume", Name: "data", Destination: "/data"}}},
			{ID: "c2", Names: []string{"/api"}, ImageID: "sha256:api", NetworkSettings: &container.NetworkSettingsSummary{Networks: map[string]*network.EndpointSettings{"backend": {}}}},
		},
		[]image.Summary{{ID: "sha256:nginx"}, {ID: "sha256:unused"}, {ID: "sha256:counted", Containers: 1}},
		[]network.Summary{{Name: "backend"}, {Name: "frontend"}},
		[]*volume.Volume{{Name: "data"}, nil, {Name: "spare"}},
	)

	assert.True(t, inventory.Images[0].InUse, "a container was made from it")
	assert.False(t, inventory.Images[1].InUse)
	assert.True(t, inventory.Images[2].InUse, "docker said so itself")

	assert.Equal(t, []string{"api", "web"}, inventory.Networks[0].Containers, "by their names, in order")
	assert.Empty(t, inventory.Networks[1].Containers)

	require.Len(t, inventory.Volumes, 2, "and nothing for nothing")
	assert.True(t, inventory.Volumes[0].InUse, "a container mounts it")
	assert.False(t, inventory.Volumes[1].InUse)
}
