package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// request is one request the blocks' control plane was asked.
type request struct {
	method string
	path   string
	query  string
	body   string
}

// blocksPlane is the control plane's resource API as the Docker facade's
// tests have it: a running Docker VM, vm-uuid, what each kind lists in it,
// what an admission or a command is answered with, and every request.
type blocksPlane struct {
	lock sync.Mutex

	listed map[string]any
	answer map[string]any
	status map[string]int

	asked []request
}

func newBlocksPlane(t *testing.T) (*Client, *blocksPlane) {
	t.Helper()

	p := &blocksPlane{listed: map[string]any{}, answer: map[string]any{}, status: map[string]int{}}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		p.lock.Lock()
		p.asked = append(p.asked, request{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, body: string(body)})
		key := r.Method + " " + r.URL.Path
		answer, answered := p.answer[key]
		status, statused := p.status[key]
		listed, lists := p.listed[r.URL.Path]
		p.lock.Unlock()

		write := func(status int, body any) {
			rw.Header().Set("Content-Type", "application/json")
			rw.WriteHeader(status)
			_ = json.NewEncoder(rw).Encode(body)
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/vms/vm-uuid":
			write(http.StatusOK, vmKind.VM{
				Kind:     vmKind.Name,
				Metadata: kind.Metadata{UUID: "vm-uuid", Name: "docker-1", OwnerUUID: "owner-uuid", Node: "node-1"},
				Spec:     vmKind.Spec{Flavor: vmKind.FlavorDocker},
				Status:   vmKind.Status{Status: kind.Status{State: vmKind.Running}},
			})
		case answered:
			if !statused {
				status = http.StatusOK
			}

			write(status, answer)
		case r.Method == http.MethodGet && lists:
			write(http.StatusOK, map[string]any{"items": listed, "pagination": map[string]any{"total_pages": 1, "current_page": 1}})
		default:
			rw.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	c, err := New(server.URL)
	require.NoError(t, err)

	return c, p
}

// answers has the plane answer what a method and a path name with body.
func (p *blocksPlane) answers(method string, path string, status int, body any) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.answer[method+" "+path] = body
	p.status[method+" "+path] = status
}

// lists has the plane list items under path.
func (p *blocksPlane) lists(path string, items any) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.listed[path] = items
}

// last is the last request made of a method.
func (p *blocksPlane) last(method string) request {
	p.lock.Lock()
	defer p.lock.Unlock()

	for i := len(p.asked) - 1; i >= 0; i-- {
		if p.asked[i].method == method {
			return p.asked[i]
		}
	}

	return request{}
}

// done is what a command that was carried out is answered with.
func done(resource any) map[string]any {
	return map[string]any{"resource": resource, "command": map[string]any{"id": "command-1"}, "result": map[string]any{"id": "command-1", "ok": true}}
}

func TestBlocks_requests(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("a container's command is its kind's action, by what names it inside its vm", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodPost, "/api/containers/web/actions/connect", http.StatusOK, done(nil))

		require.NoError(t, c.Docker("owner-uuid", "vm-uuid").ConnectNetwork(ctx, "backend", "web", []string{"api"}))

		asked := p.last(http.MethodPost)
		assert.Equal(t, "/api/containers/web/actions/connect", asked.path)
		assert.Equal(t, "owner=owner-uuid&parent=vm-uuid&wait=30s", asked.query)
		assert.JSONEq(t, `{"network":"backend","aliases":["api"]}`, asked.body)
	})

	t.Run("what its node refused is said in its words", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodPost, "/api/containers/web/actions/start", http.StatusOK, map[string]any{
			"resource": containerKind.Container{Status: containerKind.Status{Status: kind.Status{State: containerKind.Failed}, Failure: &noderequest.Error{Code: noderequest.CodeInvalid, Message: "port is already allocated"}}},
			"result":   map[string]any{"ok": false, "reason": "port is already allocated"},
		})

		err := c.Docker("owner-uuid", "vm-uuid").StartContainer(ctx, "web")
		assert.ErrorIs(t, err, docker.ErrInvalid)

		var refused *noderequest.Error
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, "port is already allocated", refused.Message)
	})

	t.Run("one started where it runs already is left as it is, as docker leaves it", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodPost, "/api/containers/web/actions/start", http.StatusBadRequest, map[string]any{"errors": map[string]string{"action": "invalid_state_transition"}})
		p.answers(http.MethodGet, "/api/containers/web", http.StatusOK, containerKind.Container{Status: containerKind.Status{Status: kind.Status{State: containerKind.Running}, Docker: &containerKind.Docker{ID: "c1", State: "running"}}})

		assert.NoError(t, c.Docker("owner-uuid", "vm-uuid").StartContainer(ctx, "web"))
	})

	t.Run("a container's log is its kind's query, its options as parameters", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodGet, "/api/containers/web/logs", http.StatusOK, map[string]any{"result": containerKind.Logs{Lines: []containerKind.LogLine{{Stream: "stdout", Line: "ready"}}}})

		lines, err := c.Docker("owner-uuid", "vm-uuid").ContainerLogs(ctx, "web", docker.LogOptions{Since: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC), Tail: 10})
		require.NoError(t, err)

		require.Len(t, lines, 1)
		assert.Equal(t, "ready", lines[0].Line)
		assert.Equal(t, "owner=owner-uuid&parent=vm-uuid&since=2026-10-06T12%3A00%3A00Z&tail=10", p.last(http.MethodGet).query)
	})

	t.Run("an image not kept is admitted, and is what its node pulled", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodPost, "/api/images", http.StatusCreated, done(imageKind.Image{
			Kind:   imageKind.Name,
			Status: imageKind.Status{Status: kind.Status{State: imageKind.Present}, Docker: &imageKind.Docker{ID: "sha256:abc", Reference: "redis:7", Tags: []string{"redis:7"}}},
		}))

		pulled, err := c.Docker("owner-uuid", "vm-uuid").PullImage(ctx, "redis:7")
		require.NoError(t, err)

		assert.Equal(t, "sha256:abc", pulled.ID)

		asked := p.last(http.MethodPost)
		assert.Equal(t, "/api/images", asked.path)
		assert.Equal(t, "owner=owner-uuid&parent=vm-uuid&wait=14m0s", asked.query)
		assert.JSONEq(t, `{"kind":"image","metadata":{},"spec":{"reference":"redis:7"}}`, asked.body)
	})

	t.Run("and one kept is pulled again", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodGet, "/api/images/redis:7", http.StatusOK, imageKind.Image{Kind: imageKind.Name, Metadata: kind.Metadata{UUID: "i-uuid"}})
		p.answers(http.MethodPost, "/api/images/i-uuid/actions/pull", http.StatusOK, done(imageKind.Image{
			Status: imageKind.Status{Status: kind.Status{State: imageKind.Present}, Docker: &imageKind.Docker{ID: "sha256:def", Reference: "redis:7"}},
		}))

		pulled, err := c.Docker("owner-uuid", "vm-uuid").PullImage(ctx, "redis:7")
		require.NoError(t, err)
		assert.Equal(t, "sha256:def", pulled.ID)
	})

	t.Run("images are listed once each, under every reference, unmanaged only when no reference is kept", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.lists("/api/images", []imageKind.Image{
			{Status: imageKind.Status{Docker: &imageKind.Docker{ID: "sha256:abc", Reference: "nginx:1.27", Tags: []string{"nginx:1.27", "nginx:latest"}}}},
			{Metadata: kind.Metadata{Labels: map[string]string{"workload.managed-by": "nobody"}}, Status: imageKind.Status{Docker: &imageKind.Docker{ID: "sha256:abc", Reference: "nginx:latest", Tags: []string{"nginx:1.27", "nginx:latest"}}}},
			{Metadata: kind.Metadata{Labels: map[string]string{"workload.managed-by": "nobody"}}, Status: imageKind.Status{Docker: &imageKind.Docker{ID: "sha256:old"}}},
			{Status: imageKind.Status{Status: kind.Status{State: imageKind.Pending}}},
		})

		images, err := c.Docker("owner-uuid", "vm-uuid").Images(ctx)
		require.NoError(t, err)

		require.Len(t, images, 2, "and one not pulled yet is not there yet")
		assert.Equal(t, docker.Image{ID: "sha256:abc", Tags: []string{"nginx:1.27", "nginx:latest"}}, images[0])
		assert.True(t, images[1].Unmanaged)
	})

	t.Run("an image removed by its id is removed under every reference, by force alone", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.lists("/api/images", []imageKind.Image{
			{Metadata: kind.Metadata{UUID: "one"}, Status: imageKind.Status{Docker: &imageKind.Docker{ID: "sha256:abcdef", Reference: "nginx:1.27"}}},
			{Metadata: kind.Metadata{UUID: "two"}, Status: imageKind.Status{Docker: &imageKind.Docker{ID: "sha256:abcdef", Reference: "nginx:latest"}}},
		})
		p.answers(http.MethodPost, "/api/images/one/actions/delete", http.StatusNoContent, nil)
		p.answers(http.MethodPost, "/api/images/two/actions/delete", http.StatusNoContent, nil)

		err := c.Docker("owner-uuid", "vm-uuid").RemoveImage(ctx, "sha256:abcdef", false)
		assert.ErrorIs(t, err, docker.ErrInvalid, "an image of several references is removed only by force")

		require.NoError(t, c.Docker("owner-uuid", "vm-uuid").RemoveImage(ctx, "abcd", true))
		assert.Equal(t, "/api/images/two/actions/delete", p.last(http.MethodPost).path)

		require.NoError(t, c.Docker("owner-uuid", "vm-uuid").RemoveImage(ctx, "nginx:1.27", false), "and by one of its references, that one")
		assert.Equal(t, "/api/images/one/actions/delete", p.last(http.MethodPost).path)

		assert.ErrorIs(t, c.Docker("owner-uuid", "vm-uuid").RemoveImage(ctx, "redis", false), domain.ErrNotExists)
	})

	t.Run("a network and a volume are made as their kinds', in the vm", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodPost, "/api/networks", http.StatusCreated, done(networkKind.Network{Status: networkKind.Status{Status: kind.Status{State: networkKind.Present}, Docker: &networkKind.Docker{ID: "n1", Name: "backend"}}}))
		p.answers(http.MethodPost, "/api/volumes", http.StatusCreated, done(volumeKind.Volume{Status: volumeKind.Status{Status: kind.Status{State: volumeKind.Present}, Docker: &volumeKind.Docker{Name: "data"}}}))

		made, err := c.Docker("owner-uuid", "vm-uuid").CreateNetwork(ctx, docker.NetworkSpec{Name: "backend", Internal: true, Labels: map[string]string{"team": "shop"}})
		require.NoError(t, err)
		assert.Equal(t, "n1", made.ID)
		assert.JSONEq(t, `{"kind":"network","metadata":{},"spec":{"name":"backend","internal":true,"labels":{"team":"shop"}}}`, p.last(http.MethodPost).body)

		volume, err := c.Docker("owner-uuid", "vm-uuid").CreateVolume(ctx, docker.VolumeSpec{Name: "data"})
		require.NoError(t, err)
		assert.Equal(t, "data", volume.Name)
		assert.Equal(t, "owner=owner-uuid&parent=vm-uuid&wait=30s", p.last(http.MethodPost).query)
	})

	t.Run("one refused is not kept, and is answered as it was refused", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodPost, "/api/volumes", http.StatusCreated, map[string]any{
			"resource": volumeKind.Volume{Metadata: kind.Metadata{UUID: "v-uuid"}, Status: volumeKind.Status{Status: kind.Status{State: volumeKind.Failed}, Failure: &noderequest.Error{Code: noderequest.CodeInvalid, Message: "a volume named data is there already"}}},
			"result":   map[string]any{"ok": false, "reason": "a volume named data is there already"},
		})
		p.answers(http.MethodPost, "/api/volumes/v-uuid/actions/delete", http.StatusNoContent, nil)

		_, err := c.Docker("owner-uuid", "vm-uuid").CreateVolume(ctx, docker.VolumeSpec{Name: "data"})
		assert.ErrorIs(t, err, docker.ErrInvalid)

		assert.Equal(t, "/api/volumes/v-uuid/actions/delete", p.last(http.MethodPost).path, "taken away again")
	})
}

func TestContainerOf(t *testing.T) {
	t.Parallel()

	pending := containerOf(containerManifest{
		Spec:   containerKind.Spec{Name: "web", Image: "nginx:1.27", RestartPolicy: containerKind.RestartAlways, Ports: []containerKind.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}}},
		Status: containerKind.Status{Status: kind.Status{State: containerKind.Pending, Reason: "its vm is starting"}},
	})

	assert.Equal(t, "web", pending.Name)
	assert.Equal(t, "created", pending.State, "one being made, in docker's words")
	assert.Equal(t, "Pending: its vm is starting", pending.Status)
	assert.Equal(t, containerKind.RestartAlways, pending.RestartPolicy)
	assert.Equal(t, []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}}, pending.Ports)

	missing := containerOf(containerManifest{
		Spec:   containerKind.Spec{Name: "web"},
		Status: containerKind.Status{Status: kind.Status{State: containerKind.Missing}, Docker: &containerKind.Docker{ID: "c1", Name: "web", State: "running"}},
	})

	assert.Equal(t, "c1", missing.ID, "what it was last seen as")
	assert.Equal(t, "created", missing.State, "and is made again")

	seen := containerOf(containerManifest{
		Metadata: kind.Metadata{Labels: map[string]string{"workload.managed-by": "nobody"}},
		Status:   containerKind.Status{Status: kind.Status{State: containerKind.Completed}, Docker: &containerKind.Docker{ID: "c2", State: "exited", Status: "Exited (0) 1 second ago"}},
	})

	assert.Equal(t, "exited", seen.State, "docker's own word")
	assert.Equal(t, "Exited (0) 1 second ago", seen.Status)
	assert.True(t, seen.Unmanaged)
}
