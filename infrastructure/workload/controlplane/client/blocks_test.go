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
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
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
				Spec:     vmKind.Spec{Image: "docker:29-dind"},
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

	c, err := New(server.URL, "docker:29-dind")
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

	t.Run("an image removed by its id is in one repository however many digests it is also listed under", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.lists("/api/images", []imageKind.Image{
			{Metadata: kind.Metadata{UUID: "tag"}, Status: imageKind.Status{Docker: &imageKind.Docker{ID: "sha256:abcdef", Reference: "busybox:1.36"}}},
			{Metadata: kind.Metadata{UUID: "digest", Labels: map[string]string{"workload.managed-by": "nobody"}}, Status: imageKind.Status{Docker: &imageKind.Docker{ID: "sha256:abcdef", Reference: "busybox@sha256:0123"}}},
		})
		p.answers(http.MethodPost, "/api/images/tag/actions/delete", http.StatusNoContent, nil)

		require.NoError(t, c.Docker("owner-uuid", "vm-uuid").RemoveImage(ctx, "sha256:abcdef", false))

		var deleted []string

		p.lock.Lock()
		for _, r := range p.asked {
			if r.method == http.MethodPost {
				deleted = append(deleted, r.path)
			}
		}
		p.lock.Unlock()

		assert.Equal(t, []string{"/api/images/tag/actions/delete"}, deleted, "its digest goes with its last tag")
	})

	t.Run("an image held by its digests alone is removed under each", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.lists("/api/images", []imageKind.Image{
			{Metadata: kind.Metadata{UUID: "digest", Labels: map[string]string{"workload.managed-by": "nobody"}}, Status: imageKind.Status{Docker: &imageKind.Docker{ID: "sha256:abcdef", Reference: "busybox@sha256:0123"}}},
		})
		p.answers(http.MethodPost, "/api/images/digest/actions/delete", http.StatusNoContent, nil)

		require.NoError(t, c.Docker("owner-uuid", "vm-uuid").RemoveImage(ctx, "abcdef", false))
		assert.Equal(t, "/api/images/digest/actions/delete", p.last(http.MethodPost).path)
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
		Spec:   containerKind.Spec{Name: "web", Image: "nginx:1.27", RestartPolicy: containerKind.RestartAlways, Ports: []containerKind.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}, {ContainerPort: 9090, HostPort: 9090, HostIP: "127.0.0.1", Protocol: "tcp"}}},
		Status: containerKind.Status{Status: kind.Status{State: containerKind.Pending, Reason: "its vm is starting"}},
	})

	assert.Equal(t, "web", pending.Name)
	assert.Equal(t, "created", pending.State, "one being made, in docker's words")
	assert.Equal(t, "Pending: its vm is starting", pending.Status)
	assert.Equal(t, containerKind.RestartAlways, pending.RestartPolicy)
	assert.Equal(t, []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}, {ContainerPort: 9090, HostPort: 9090, HostIP: "127.0.0.1", Protocol: "tcp"}}, pending.Ports, "as it was asked for, on the address asked for")

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

	// a stack's, as its stack's graph wrote it on its record.
	stacked := kind.Metadata{Labels: map[string]string{stackKind.LabelStack: "s-uuid", stackKind.LabelService: "web"}}

	making := containerOf(containerManifest{
		Metadata: stacked,
		Spec:     containerKind.Spec{Name: "shop-abcde_web", Image: "nginx:1.27"},
		Status:   containerKind.Status{Status: kind.Status{State: containerKind.Pending, Reason: "its vm is being made"}},
	})

	assert.Equal(t, "s-uuid", making.Stack, "one of a stack's its vm's dockerd has said nothing of yet is its stack's, as its record says")
	assert.Equal(t, "web", making.Service)
	assert.Equal(t, stacked.Labels, making.Labels, "labelled as its node labels it")
	assert.True(t, passes(making, docker.ContainerFilter{All: true, Label: stackKind.LabelStack + "=s-uuid"}), "so that its stack's detail, which lists its containers by its label, lists it")

	said := map[string]string{"workload.managed": "true", stackKind.LabelStack: "s-uuid", stackKind.LabelService: "cache"}
	labelled := containerOf(containerManifest{
		Metadata: stacked,
		Status:   containerKind.Status{Status: kind.Status{State: containerKind.Running}, Docker: &containerKind.Docker{ID: "c3", State: "running", Labels: said}},
	})

	assert.Equal(t, "cache", labelled.Service, "where its vm's dockerd says one too, what it says is what it is")
	assert.Equal(t, said, labelled.Labels)

	unlabelled := containerOf(containerManifest{
		Metadata: stacked,
		Status:   containerKind.Status{Status: kind.Status{State: containerKind.Running}, Docker: &containerKind.Docker{ID: "c4", State: "running", Labels: map[string]string{"workload.managed": "true"}}},
	})

	assert.Equal(t, "s-uuid", unlabelled.Stack, "and where it says none, its record's is")
	assert.Equal(t, "web", unlabelled.Service)
	assert.Equal(t, map[string]string{"workload.managed": "true", stackKind.LabelStack: "s-uuid", stackKind.LabelService: "web"}, unlabelled.Labels)

	// what its record says of it is read as its node would make it: its
	// mounts as they are asked for, and only the labels that say something.
	asked := containerOf(containerManifest{
		Metadata: kind.Metadata{Labels: map[string]string{stackKind.LabelStack: "", stackKind.LabelService: "web"}},
		Spec:     containerKind.Spec{Name: "shop-abcde_web", Image: "nginx:1.27", Mounts: []containerKind.Mount{{Type: containerKind.MountVolume, Source: "shop-abcde_data", Target: "/data"}, {Type: containerKind.MountBind, Source: "/srv", Target: "/srv", ReadOnly: true}}},
		Status:   containerKind.Status{Status: kind.Status{State: containerKind.Pending}},
	})

	assert.Equal(t, []docker.Mount{{Type: containerKind.MountVolume, Source: "shop-abcde_data", Target: "/data"}, {Type: containerKind.MountBind, Source: "/srv", Target: "/srv", ReadOnly: true}}, asked.Mounts)
	assert.Equal(t, map[string]string{stackKind.LabelService: "web"}, asked.Labels, "a label that says nothing is none")
	assert.Empty(t, asked.Stack)
}

func TestAnswered(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		status any
		want   bool
	}{
		"one its node made, which docker says beside its state": {
			status: containerKind.Status{Status: kind.Status{State: containerKind.Running}, Docker: &containerKind.Docker{ID: "c1", Name: "web", State: "running"}},
			want:   true,
		},
		"a volume, which docker knows by its name alone": {
			status: volumeKind.Status{Status: kind.Status{State: volumeKind.Present}, Docker: &volumeKind.Docker{Name: "data"}},
			want:   true,
		},
		"one its node has said nothing of yet": {
			status: containerKind.Status{Status: kind.Status{State: containerKind.Pending}},
		},
		"one still being made": {
			status: imageKind.Status{Status: kind.Status{State: imageKind.Pulling}, Docker: &imageKind.Docker{ID: "sha256:abc"}},
		},
		"one that failed": {
			status: networkKind.Status{Status: kind.Status{State: kind.Failed}},
			want:   true,
		},
		"one refused, in the codes every side knows": {
			status: imageKind.Status{Status: kind.Status{State: imageKind.Pending}, Failure: &noderequest.Error{Code: noderequest.CodeNotFound}},
			want:   true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			status, err := json.Marshal(tt.status)
			require.NoError(t, err)

			assert.Equal(t, tt.want, answered(status))
		})
	}
}

// TestClient_CreateContainer_vmToBeMade holds a container that names no VM to
// being answered as one a VM is made for, though its graph makes that VM after
// the control plane answered, and its record may not be there yet: it is
// called what the container keeps of it.
func TestClient_CreateContainer_vmToBeMade(t *testing.T) {
	t.Parallel()

	c, p := newBlocksPlane(t)

	made := containerKind.Container{
		Kind:     containerKind.Name,
		Metadata: kind.Metadata{UUID: "c-uuid", Name: "web", Refs: []kind.Reference{{Kind: vmKind.Name, UUID: "planned-uuid"}}},
		Spec:     containerKind.Spec{Name: "web", Image: "nginx:1.27", VM: stackKind.VMChoice{UUID: "planned-uuid", Name: "builds"}},
		Status: containerKind.Status{
			Status: kind.Status{State: containerKind.Running},
			Docker: &containerKind.Docker{ID: "c1", Name: "web", State: "running"},
		},
	}

	p.answers(http.MethodPost, "/api/containers", http.StatusCreated, done(made))

	created, err := c.CreateContainer(context.Background(), "owner-uuid", workloadControlPlane.ContainerRequest{
		VM:        workloadControlPlane.DockerVMChoice{New: &workloadControlPlane.NewDockerVM{Name: "builds"}},
		Container: docker.ContainerSpec{Name: "web", Image: "nginx:1.27"},
	})
	require.NoError(t, err)

	assert.Equal(t, workloadControlPlane.ChosenVM{UUID: "planned-uuid", Name: "builds", Created: true}, created.VM)
	assert.Equal(t, "c1", created.Container.ID)
}

// TestClient_deletes holds a delete of what containers need to asking with
// cascade when it is asked to, and to reading the 202 its answer is, with the
// containers that need it and whether one makes it again, beside a refusal
// among it.
func TestClient_deletes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	needs := map[string]any{
		"dependents": []map[string]any{{"kind": "container", "uuid": "c-uuid", "name": "web", "recreates": true}},
		"recreated":  true,
	}

	wanted := workloadControlPlane.Deleted{
		Dependents: []workloadControlPlane.Dependent{{Kind: "container", UUID: "c-uuid", Name: "web", Recreates: true}},
		Recreated:  true,
	}

	t.Run("a vm's delete reads the 202 with what needs it, and asks with cascade when it is asked to", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodDelete, "/api/vms/vm-uuid", http.StatusAccepted, needs)

		deleted, err := c.DeleteVM(ctx, "owner-uuid", "vm-uuid", false)
		require.NoError(t, err)
		assert.Equal(t, wanted, deleted)
		assert.Equal(t, "owner=owner-uuid", p.last(http.MethodDelete).query)

		_, err = c.DeleteVM(ctx, "owner-uuid", "vm-uuid", true)
		require.NoError(t, err)
		assert.Equal(t, "cascade=true&owner=owner-uuid", p.last(http.MethodDelete).query)
	})

	t.Run("and one nothing needs, answered with nothing, is nothing", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodDelete, "/api/vms/vm-uuid", http.StatusNoContent, nil)

		deleted, err := c.DeleteVM(ctx, "owner-uuid", "vm-uuid", false)
		require.NoError(t, err)
		assert.Equal(t, workloadControlPlane.Deleted{}, deleted)
	})

	t.Run("a network's delete is its kind's, with cascade when it is asked to, and is what needs it", func(t *testing.T) {
		t.Parallel()

		answer := done(nil)
		answer["dependents"] = needs["dependents"]
		answer["recreated"] = false

		c, p := newBlocksPlane(t)
		p.answers(http.MethodPost, "/api/networks/backend/actions/delete", http.StatusAccepted, answer)

		deleted, err := c.DeleteNetwork(ctx, "owner-uuid", "vm-uuid", "backend", true)
		require.NoError(t, err)
		assert.Equal(t, workloadControlPlane.Deleted{Dependents: wanted.Dependents}, deleted)

		asked := p.last(http.MethodPost)
		assert.Equal(t, "cascade=true&owner=owner-uuid&parent=vm-uuid&wait=30s", asked.query)
	})

	t.Run("a volume's refused, as its node refused it, is what needs it all the same", func(t *testing.T) {
		t.Parallel()

		answer := map[string]any{
			"resource":   map[string]any{"kind": "volume", "status": map[string]any{"state": "present", "failure": map[string]any{"code": "invalid", "message": "remove data: volume is in use - [c-1]"}}},
			"command":    map[string]any{"id": "command-1"},
			"result":     map[string]any{"id": "command-1", "ok": false, "reason": "remove data: volume is in use - [c-1]"},
			"dependents": needs["dependents"],
			"recreated":  true,
		}

		c, p := newBlocksPlane(t)
		p.answers(http.MethodPost, "/api/volumes/data/actions/delete", http.StatusAccepted, answer)

		deleted, err := c.DeleteVolume(ctx, "owner-uuid", "vm-uuid", "data", false, false)
		require.ErrorIs(t, err, docker.ErrInvalid, "refused, in its node's words")
		assert.Equal(t, wanted, deleted)

		asked := p.last(http.MethodPost)
		assert.Equal(t, "owner=owner-uuid&parent=vm-uuid&wait=30s", asked.query, "and with no cascade, none is asked for")
		assert.JSONEq(t, `{}`, asked.body, "not by force")
	})
}

// TestClient_dependents holds what needs a VM, a network and a volume to being
// read with a GET of what needs it, inside the VM for what lives in one, and
// to nothing else being asked: no delete, so that a control plane that does
// not know the question deletes nothing for it.
func TestClient_dependents(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	needs := map[string]any{
		"dependents": []map[string]any{{"kind": "stack", "uuid": "s-uuid", "name": "shop", "recreates": true}},
		"recreated":  true,
	}

	wanted := workloadControlPlane.Deleted{
		Dependents: []workloadControlPlane.Dependent{{Kind: "stack", UUID: "s-uuid", Name: "shop", Recreates: true}},
		Recreated:  true,
	}

	t.Run("a vm's is read with a get of what needs it, as its owner's", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodGet, "/api/vms/vm-uuid/dependents", http.StatusOK, needs)

		needed, err := c.VMDependents(ctx, "owner-uuid", "vm-uuid")
		require.NoError(t, err)
		assert.Equal(t, wanted, needed)

		asked := p.last(http.MethodGet)
		assert.Equal(t, "/api/vms/vm-uuid/dependents", asked.path)
		assert.Equal(t, "owner=owner-uuid", asked.query)

		assert.Empty(t, p.last(http.MethodDelete), "nothing is deleted")
		assert.Empty(t, p.last(http.MethodPost), "nor asked")
	})

	t.Run("and one nothing needs is nothing", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodGet, "/api/vms/vm-uuid/dependents", http.StatusOK, map[string]any{"dependents": []any{}, "recreated": false})

		needed, err := c.VMDependents(ctx, "", "vm-uuid")
		require.NoError(t, err)
		assert.Equal(t, workloadControlPlane.Deleted{}, needed)
	})

	t.Run("a vm that is not there, or a control plane that does not know the question, is not there", func(t *testing.T) {
		t.Parallel()

		c, _ := newBlocksPlane(t)

		_, err := c.VMDependents(ctx, "owner-uuid", "gone-uuid")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	t.Run("a network's is read inside its vm, by its id or its name", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodGet, "/api/networks/backend/dependents", http.StatusOK, needs)

		needed, err := c.NetworkDependents(ctx, "owner-uuid", "vm-uuid", "backend")
		require.NoError(t, err)
		assert.Equal(t, wanted, needed)
		assert.Equal(t, "owner=owner-uuid&parent=vm-uuid", p.last(http.MethodGet).query)
		assert.Empty(t, p.last(http.MethodPost), "nothing is asked of it")
	})

	t.Run("a volume's is read inside its vm, by its name", func(t *testing.T) {
		t.Parallel()

		c, p := newBlocksPlane(t)
		p.answers(http.MethodGet, "/api/volumes/data/dependents", http.StatusOK, needs)

		needed, err := c.VolumeDependents(ctx, "", "vm-uuid", "data")
		require.NoError(t, err)
		assert.Equal(t, wanted, needed)
		assert.Equal(t, "parent=vm-uuid", p.last(http.MethodGet).query)
	})

	t.Run("a network of a vm that is not there, and a volume that is not there, are not there", func(t *testing.T) {
		t.Parallel()

		c, _ := newBlocksPlane(t)

		_, err := c.NetworkDependents(ctx, "owner-uuid", "gone-uuid", "backend")
		assert.ErrorIs(t, err, domain.ErrNotExists)

		_, err = c.VolumeDependents(ctx, "owner-uuid", "vm-uuid", "nowhere")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}
