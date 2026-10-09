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

	c, err := New(server.URL, "docker:29-dind")
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
		assert.Equal(t, "is=docker&owner=owner-uuid&page=2", asked.query, "the control plane says which vms are docker vms, as their images do")

		_, err = c.VMs(ctx, "", "", 0)
		require.NoError(t, err)
		assert.Empty(t, asked.query)
	})

	t.Run("a vm is asked for in the shape the control plane reads", func(t *testing.T) {
		t.Parallel()

		c, asked := controlPlane(t, http.StatusCreated, `{"resource":{
			"kind": "vm",
			"metadata": {"uuid": "vm-uuid", "name": "box", "lifetime": 3600000000000},
			"spec": {"image": "ubuntu:24.04", "resources": {"cpus": 2, "memory": 2147483648, "disk": 10737418240}, "ports": [22, 80], "network": {"ingress": "allow", "egress": "deny"}},
			"status": {"state": "scheduled", "expected": "running"}
		}, "command": {"id": "command-1", "action": "create"}}`)

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
			"kind": "vm",
			"metadata": {"name": "box", "lifetime": 3600000000000},
			"spec": {
				"resources": {"cpus": 2, "memory": 2147483648, "disk": 10737418240},
				"ports": [22, 80],
				"network": {"ingress": "allow", "egress": "deny"}
			}
		}`, string(asked.body), "a vm is asked for as its kind's manifest")

		assert.Equal(t, "vm-uuid", created.UUID)
		assert.Equal(t, vm.KindMachine, created.Kind, "as its image says")
		assert.Equal(t, vm.Scheduled, created.CurrentState)
		assert.Equal(t, vm.Running, created.ExpectedState)
		assert.Equal(t, time.Hour, created.Lifetime)
		assert.Equal(t, uint64(2<<30), created.Resources.Memory)
	})

	t.Run("a docker vm is asked for by the docker image, which makes it one", func(t *testing.T) {
		t.Parallel()

		c, asked := controlPlane(t, http.StatusCreated, `{"resource":{"kind":"vm","metadata":{"uuid":"vm-uuid"},"spec":{"image":"docker:29-dind"}}}`)

		created, err := c.CreateVM(ctx, "owner-uuid", workloadControlPlane.VMRequest{Name: "builds", Kind: vm.KindDocker})
		require.NoError(t, err)

		var spec struct {
			Spec map[string]any `json:"spec"`
		}

		require.NoError(t, json.Unmarshal(asked.body, &spec))
		assert.Equal(t, "docker:29-dind", spec.Spec["image"])
		assert.Equal(t, vm.KindDocker, created.Kind, "as its image says")
	})

	for name, request := range map[string]workloadControlPlane.VMRequest{
		"a machine booting the docker image":    {Name: "box", Kind: vm.KindMachine, Image: "docker:29-dind"},
		"a docker vm booting a machine's image": {Name: "box", Kind: vm.KindDocker, Image: "ubuntu:24.04"},
	} {
		t.Run("not asked for: "+name, func(t *testing.T) {
			t.Parallel()

			c, asked := controlPlane(t, http.StatusCreated, `{}`)

			_, err := c.CreateVM(ctx, "owner-uuid", request)

			var refused *ValidationError
			require.ErrorAs(t, err, &refused)
			assert.Equal(t, domain.ValidationErrors{"image": "image_of_another_kind"}, refused.ValidationErrors, "the image would make it another kind than it was asked for as")
			assert.Empty(t, asked.method, "nothing is asked of the control plane")
		})
	}

	t.Run("an update says only what changes", func(t *testing.T) {
		t.Parallel()

		c, asked := controlPlane(t, http.StatusOK, `{"resource":{"kind":"vm","metadata":{"uuid":"vm-uuid"}}}`)

		forever := time.Duration(0)
		_, err := c.UpdateVM(ctx, "", "vm-uuid", workloadControlPlane.VMUpdate{Lifetime: &forever, Resources: &vm.Resources{CPUs: 1, Memory: 1 << 30, Disk: 20 << 30}})
		require.NoError(t, err)

		assert.Equal(t, http.MethodPost, asked.method)
		assert.Equal(t, "/api/vms/vm-uuid/actions/update", asked.path, "a change is the vm kind's update")
		assert.JSONEq(t, `{"lifetime":0,"resources":{"cpus":1,"memory":1073741824,"disk":21474836480}}`, string(asked.body))
	})

	t.Run("a log is asked for from a moment, and its last lines", func(t *testing.T) {
		t.Parallel()

		c, asked := controlPlane(t, http.StatusOK, `{"result":{"lines":[{"at":"2026-10-04T12:00:00Z","source":"kernel","line":"booted"}]}}`)

		since := time.Date(2026, 10, 4, 11, 0, 0, 0, time.UTC)
		lines, err := c.VMLogs(ctx, "", "vm-uuid", vm.LogOptions{Since: since, Tail: 50})
		require.NoError(t, err)

		assert.Equal(t, "/api/vms/vm-uuid/logs", asked.path)
		assert.Equal(t, "since=2026-10-04T11%3A00%3A00Z&tail=50", asked.query)
		require.Len(t, lines, 1)
		assert.Equal(t, vm.LogLine{At: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC), Source: vm.LogSourceKernel, Line: "booted"}, lines[0])
	})
}
