package container

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api"
	containerTypes "github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

func TestDockerManager_ResourceLimits(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name   string
		limits task.ResourceLimits

		// what docker is given, and what is read back from it.
		memory   int64
		nanoCPUs int64
		readBack task.ResourceLimits
	}{
		{
			name:     "a limit goes in and comes back out in bytes",
			limits:   task.ResourceLimits{Cpu: 0.5, Memory: 256 << 20, Disk: 100 << 20},
			memory:   268435456,
			nanoCPUs: 500_000_000,

			// the disk limit is the one that does not come back: docker is
			// never given one.
			readBack: task.ResourceLimits{Cpu: 0.5, Memory: 256 << 20},
		},
		{
			name:     "no limit is docker's no limit, and reads back as none",
			limits:   task.ResourceLimits{},
			memory:   0,
			nanoCPUs: 0,
			readBack: task.ResourceLimits{},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			daemon := &fakeDaemon{}
			server := httptest.NewServer(daemon)
			defer server.Close()

			cli, err := newClient("tcp://" + server.Listener.Addr().String())
			require.NoError(t, err)

			manager := NewDockerManager(cli, Scope{Node: "node-1"}, slog.New(slog.DiscardHandler))

			id, err := manager.Create(context.Background(), &task.Execution{
				Name:           "nginx-xkfqz",
				Image:          "nginx:alpine",
				ResourceLimits: tt.limits,
			})
			require.NoError(t, err)

			given := daemon.hostConfig()
			require.NotNil(t, given)
			assert.Equal(t, tt.memory, given.Memory)
			assert.Equal(t, tt.nanoCPUs, given.NanoCPUs)

			inspected, err := manager.Inspect(context.Background(), id)
			require.NoError(t, err)

			assert.Equal(t, tt.readBack, inspected.ResourceLimits)
		})
	}
}

// fakeContainerID is the one container a fakeDaemon ever creates.
const fakeContainerID = "4f2c9d0b7a1e"

// fakeDaemon is as much of docker's engine API as creating a container and
// inspecting it takes. Like dockerd, it keeps what a container was created
// with and hands it back on inspection, so what a test reads back has been
// through the same client, and the same json, as what it sent.
type fakeDaemon struct {
	mu      sync.Mutex
	created *containerTypes.CreateRequest
}

// hostConfig is what the container was created with, or nil before it was.
func (d *fakeDaemon) hostConfig() *containerTypes.HostConfig {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.created == nil {
		return nil
	}

	return d.created.HostConfig
}

func (d *fakeDaemon) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rw.Header().Set("Api-Version", api.DefaultVersion)

	switch path := r.URL.Path; {
	case strings.HasSuffix(path, "/_ping"):
		rw.WriteHeader(http.StatusOK)

	// the image is already there, so nothing is pulled.
	case strings.HasSuffix(path, "/images/json"):
		writeJSON(rw, http.StatusOK, []image.Summary{{ID: "sha256:nginx"}})

	case strings.HasSuffix(path, "/containers/create"):
		var request containerTypes.CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)

			return
		}

		d.created = &request

		writeJSON(rw, http.StatusCreated, containerTypes.CreateResponse{ID: fakeContainerID})

	case strings.HasSuffix(path, "/containers/"+fakeContainerID+"/json") && d.created != nil:
		writeJSON(rw, http.StatusOK, containerTypes.InspectResponse{
			ContainerJSONBase: &containerTypes.ContainerJSONBase{
				ID:         fakeContainerID,
				Name:       "/nginx-xkfqz",
				Created:    time.Now().UTC().Format(time.RFC3339Nano),
				State:      &containerTypes.State{Status: "created"},
				HostConfig: d.created.HostConfig,
			},
			Config:          d.created.Config,
			NetworkSettings: &containerTypes.NetworkSettings{},
		})

	default:
		http.NotFound(rw, r)
	}
}

func writeJSON(rw http.ResponseWriter, status int, body any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(body)
}
