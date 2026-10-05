package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	workloadControlPlane "github.com/khanzadimahdi/testproject/domain/workload/controlplane"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// answering is a control plane that answers every request with status and
// body, and remembers what it was asked.
type answering struct {
	method string
	path   string
	query  string
	body   []byte
}

func controlPlane(t *testing.T, status int, body string) (*Client, *answering) {
	t.Helper()

	asked := &answering{}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		asked.method = r.Method
		asked.path = r.URL.Path
		asked.query = r.URL.RawQuery
		asked.body, _ = io.ReadAll(r.Body)

		if len(body) > 0 {
			rw.Header().Set("Content-Type", "application/json")
		}

		rw.WriteHeader(status)
		_, _ = io.WriteString(rw, body)
	}))
	t.Cleanup(server.Close)

	c, err := New(server.URL)
	require.NoError(t, err)

	return c, asked
}

func TestClient_errors(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		status int
		body   string

		// is, when set, is what errors.Is has to read the error as.
		is error

		// refused, when set, is the refusal the error has to carry.
		refused domain.ValidationErrors

		// node, when set, is the node's code the error has to carry.
		node noderequest.Code
	}{
		"what is not there": {
			status: http.StatusNotFound,
			is:     domain.ErrNotExists,
		},
		"a request the control plane would not take": {
			status:  http.StatusBadRequest,
			body:    `{"errors":{"resources.memory":"quota_exceeded","vm":"vm_required"}}`,
			refused: domain.ValidationErrors{"resources.memory": "quota_exceeded", "vm": "vm_required"},
		},
		"a vm that is not running": {
			status:  http.StatusConflict,
			body:    `{"error":{"code":"not_running","message":"the vm is not running"}}`,
			is:      vm.ErrNotRunning,
			refused: domain.ValidationErrors{"vm": "not_running"},
			node:    noderequest.CodeNotRunning,
		},
		"a vm that is not a docker vm": {
			status:  http.StatusConflict,
			body:    `{"error":{"code":"not_docker"}}`,
			is:      vm.ErrNotDocker,
			refused: domain.ValidationErrors{"vm": "not_docker"},
			node:    noderequest.CodeNotDocker,
		},
		"a dockerd that did not come up": {
			status:  http.StatusConflict,
			body:    `{"error":{"code":"docker_unavailable"}}`,
			is:      docker.ErrUnavailable,
			refused: domain.ValidationErrors{"vm": "docker_unavailable"},
			node:    noderequest.CodeDockerUnavailable,
		},
		"a docker object that is not there": {
			status: http.StatusNotFound,
			body:   `{"error":{"code":"not_found","message":"No such container: web"}}`,
			is:     domain.ErrNotExists,
			node:   noderequest.CodeNotFound,
		},
		"a request dockerd refused": {
			status: http.StatusUnprocessableEntity,
			body:   `{"error":{"code":"invalid","message":"Conflict. The container name \"/web\" is already in use"}}`,
			is:     docker.ErrInvalid,
			node:   noderequest.CodeInvalid,
		},
		"a node that took too long": {
			status: http.StatusGatewayTimeout,
			body:   `{"error":{"code":"timeout"}}`,
			is:     context.DeadlineExceeded,
			node:   noderequest.CodeTimeout,
		},
		"a node that is not answering": {
			status: http.StatusBadGateway,
			body:   `{"error":{"code":"internal","message":"the node holding the vm is not answering"}}`,
			node:   noderequest.CodeInternal,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, _ := controlPlane(t, tt.status, tt.body)

			err := c.Docker("owner-uuid", "vm-uuid").Ping(context.Background())
			require.Error(t, err)

			if tt.is != nil {
				assert.ErrorIs(t, err, tt.is)
			}

			var refusal *ValidationError
			if tt.refused != nil {
				require.ErrorAs(t, err, &refusal, "the blog reads a refusal as the client's ValidationError")
				assert.Equal(t, tt.refused, refusal.ValidationErrors)
			} else {
				assert.False(t, errors.As(err, &refusal))
			}

			var nodeError *noderequest.Error
			if len(tt.node) > 0 {
				require.ErrorAs(t, err, &nodeError)
				assert.Equal(t, tt.node, nodeError.Code)
			} else {
				assert.False(t, errors.As(err, &nodeError))
			}
		})
	}

	t.Run("anything else is said as it was answered", func(t *testing.T) {
		t.Parallel()

		c, _ := controlPlane(t, http.StatusInternalServerError, "")

		err := c.StartVM(context.Background(), "", "vm-uuid")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
		assert.NotErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestClient_requests(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("an owner narrows what is asked, and nobody is everybody", func(t *testing.T) {
		t.Parallel()

		c, asked := controlPlane(t, http.StatusOK, `{"items":[],"pagination":{"total_pages":0,"current_page":2}}`)

		_, err := c.VMs(ctx, "owner-uuid", vm.KindDocker, 2)
		require.NoError(t, err)
		assert.Equal(t, "/api/vms", asked.path)
		assert.Equal(t, "kind=docker&owner=owner-uuid&page=2", asked.query)

		_, err = c.VMs(ctx, "", "", 0)
		require.NoError(t, err)
		assert.Empty(t, asked.query)
	})

	t.Run("a vm is asked for in the shape the control plane reads", func(t *testing.T) {
		t.Parallel()

		c, asked := controlPlane(t, http.StatusCreated, `{"uuid":"vm-uuid","name":"box","kind":"machine","state":"scheduled","expected_state":"running","lifetime_seconds":3600,"resources":{"cpus":2,"memory":2147483648,"disk":10737418240},"ports":[22,80]}`)

		created, err := c.CreateVM(ctx, "owner-uuid", workloadControlPlane.VMRequest{
			Name:      "box",
			Kind:      vm.KindMachine,
			Resources: vm.Resources{CPUs: 2, Memory: 2 << 30, Disk: 10 << 30},
			Ports:     []port.Port{22, 80},
			Network:   vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
			Lifetime:  time.Hour,
		})
		require.NoError(t, err)

		assert.Equal(t, http.MethodPost, asked.method)
		assert.Equal(t, "/api/vms", asked.path)
		assert.Equal(t, "owner=owner-uuid", asked.query)
		assert.JSONEq(t, `{
			"name": "box",
			"kind": "machine",
			"resources": {"cpus": 2, "memory": 2147483648, "disk": 10737418240},
			"ports": [22, 80],
			"network": {"ingress": "allow", "egress": "deny"},
			"persistent_disk": false,
			"lifetime_seconds": 3600
		}`, string(asked.body))

		assert.Equal(t, "vm-uuid", created.UUID)
		assert.Equal(t, vm.Scheduled, created.CurrentState)
		assert.Equal(t, vm.Running, created.ExpectedState)
		assert.Equal(t, time.Hour, created.Lifetime)
		assert.Equal(t, uint64(2<<30), created.Resources.Memory)
	})

	t.Run("an update says only what changes", func(t *testing.T) {
		t.Parallel()

		c, asked := controlPlane(t, http.StatusOK, `{"uuid":"vm-uuid"}`)

		forever := time.Duration(0)
		_, err := c.UpdateVM(ctx, "", "vm-uuid", workloadControlPlane.VMUpdate{Lifetime: &forever, Resources: &vm.Resources{CPUs: 1, Memory: 1 << 30, Disk: 20 << 30}})
		require.NoError(t, err)

		assert.Equal(t, http.MethodPatch, asked.method)
		assert.Equal(t, "/api/vms/vm-uuid", asked.path)
		assert.JSONEq(t, `{"lifetime_seconds":0,"resources":{"cpus":1,"memory":1073741824,"disk":21474836480}}`, string(asked.body))
	})

	t.Run("a stack is deleted with its volumes when that is asked", func(t *testing.T) {
		t.Parallel()

		c, asked := controlPlane(t, http.StatusAccepted, "")

		require.NoError(t, c.DeleteStack(ctx, "owner-uuid", "stack-uuid", true))
		assert.Equal(t, http.MethodDelete, asked.method)
		assert.Equal(t, "/api/stacks/stack-uuid", asked.path)
		assert.Equal(t, "owner=owner-uuid&volumes=true", asked.query)
	})

	t.Run("a log is asked for from a moment, and its last lines", func(t *testing.T) {
		t.Parallel()

		c, asked := controlPlane(t, http.StatusOK, `{"lines":[{"at":"2026-10-04T12:00:00Z","source":"kernel","line":"booted"}],"truncated":false}`)

		since := time.Date(2026, 10, 4, 11, 0, 0, 0, time.UTC)
		lines, err := c.VMLogs(ctx, "", "vm-uuid", vm.LogOptions{Since: since, Tail: 50})
		require.NoError(t, err)

		assert.Equal(t, "/api/vms/vm-uuid/logs", asked.path)
		assert.Equal(t, "since=2026-10-04T11%3A00%3A00Z&tail=50", asked.query)
		require.Len(t, lines, 1)
		assert.Equal(t, vm.LogLine{At: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC), Source: vm.LogSourceKernel, Line: "booted"}, lines[0])
	})
}

func TestDaemon(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, tt := range map[string]struct {
		call    func(d docker.Daemon) error
		op      string
		payload string
		answer  string
	}{
		"ping": {
			call: func(d docker.Daemon) error { return d.Ping(ctx) },
			op:   "ping",
		},
		"containers": {
			call: func(d docker.Daemon) error {
				containers, err := d.Containers(ctx, docker.ContainerFilter{All: true, Stack: "web-abcde"})
				if err == nil && (len(containers) != 1 || containers[0].ID != "c1") {
					return errors.New("not the container the node listed")
				}

				return err
			},
			op:      "containers.list",
			payload: `{"all":true,"stack":"web-abcde"}`,
			answer:  `[{"id":"c1","name":"web","ports":[],"networks":[],"mounts":[],"restart_policy":"","created_at":"0001-01-01T00:00:00Z","image":"","state":"","status":"","command":""}]`,
		},
		"create a container": {
			call: func(d docker.Daemon) error {
				created, err := d.CreateContainer(ctx, docker.ContainerSpec{Image: "nginx:1.27", Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 8080, Protocol: "tcp"}}})
				if err == nil && created.ID != "c1" {
					return errors.New("not the container the node made")
				}

				return err
			},
			op:      "containers.create",
			payload: `{"image":"nginx:1.27","ports":[{"container_port":80,"host_port":8080,"protocol":"tcp"}]}`,
			answer:  `{"id":"c1"}`,
		},
		"remove a container": {
			call:    func(d docker.Daemon) error { return d.RemoveContainer(ctx, "c1", true) },
			op:      "containers.remove",
			payload: `{"id":"c1","force":true}`,
		},
		"a container's log": {
			call: func(d docker.Daemon) error {
				lines, err := d.ContainerLogs(ctx, "c1", docker.LogOptions{Tail: 10})
				if err == nil && (len(lines) != 1 || lines[0].Stream != "stdout") {
					return errors.New("not the lines the node read")
				}

				return err
			},
			op:      "containers.logs",
			payload: `{"id":"c1","since":"0001-01-01T00:00:00Z","tail":10}`,
			answer:  `[{"at":"2026-10-04T12:00:00Z","stream":"stdout","line":"ready"}]`,
		},
		"connect a network": {
			call:    func(d docker.Daemon) error { return d.ConnectNetwork(ctx, "backend", "c1", []string{"api"}) },
			op:      "containers.connect",
			payload: `{"network":"backend","container":"c1","aliases":["api"]}`,
		},
		"pull an image": {
			call:    func(d docker.Daemon) error { _, err := d.PullImage(ctx, "redis:7"); return err },
			op:      "images.pull",
			payload: `{"reference":"redis:7"}`,
			answer:  `{"id":"sha256:abc","tags":["redis:7"],"size":1,"created_at":"0001-01-01T00:00:00Z","in_use":false}`,
		},
		"remove a volume by its name": {
			call:    func(d docker.Daemon) error { return d.RemoveVolume(ctx, "data", false) },
			op:      "volumes.remove",
			payload: `{"id":"data","force":false}`,
		},
		"create a network": {
			call: func(d docker.Daemon) error {
				_, err := d.CreateNetwork(ctx, docker.NetworkSpec{Name: "backend", Internal: true})
				return err
			},
			op:      "networks.create",
			payload: `{"name":"backend","internal":true}`,
			answer:  `{"id":"n1","name":"backend"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			result := `{}`
			if len(tt.answer) > 0 {
				answer, err := json.Marshal(json.RawMessage(tt.answer))
				require.NoError(t, err)

				result = `{"result":` + string(answer) + `}`
			}

			c, asked := controlPlane(t, http.StatusOK, result)

			require.NoError(t, tt.call(c.Docker("owner-uuid", "vm-uuid")))

			assert.Equal(t, http.MethodPost, asked.method)
			assert.Equal(t, "/api/vms/vm-uuid/docker/"+tt.op, asked.path)
			assert.Equal(t, "owner=owner-uuid", asked.query)

			if len(tt.payload) > 0 {
				assert.JSONEq(t, tt.payload, string(asked.body))
			} else {
				assert.Empty(t, asked.body)
			}
		})
	}
}
