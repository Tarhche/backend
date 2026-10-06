package client

import (
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
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

var made = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

// stacksPlane is the control plane's resource API as these tests have it:
// the stacks it holds, the VMs they are in, what a stack's commands are
// answered with, and every request it was asked.
type stacksPlane struct {
	lock sync.Mutex

	stacks map[string]stackManifest
	vms    map[string]vmManifest

	// answer is what a stack's command is answered with: its status, and
	// its body.
	status int
	body   string

	asked []string
	sent  map[string]json.RawMessage
}

func newStacksPlane(t *testing.T) (*Client, *stacksPlane) {
	t.Helper()

	p := &stacksPlane{
		stacks: map[string]stackManifest{},
		vms:    map[string]vmManifest{},
		status: http.StatusAccepted,
		body:   `{"resource":{}}`,
		sent:   map[string]json.RawMessage{},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/stacks/{uuid}", func(rw http.ResponseWriter, r *http.Request) {
		p.record(r)

		s, ok := p.stack(r.PathValue("uuid"))
		if !ok {
			rw.WriteHeader(http.StatusNotFound)

			return
		}

		answer(rw, http.StatusOK, s)
	})

	mux.HandleFunc("GET /api/stacks", func(rw http.ResponseWriter, r *http.Request) {
		p.record(r)

		p.lock.Lock()
		items := []stackManifest{}
		for _, s := range p.stacks {
			if parent := r.URL.Query().Get("parent"); len(parent) == 0 || stackKind.VMOf(s) == parent {
				items = append(items, s)
			}
		}
		p.lock.Unlock()

		answer(rw, http.StatusOK, map[string]any{"items": items, "pagination": map[string]any{"total_pages": 1, "current_page": 1}})
	})

	mux.HandleFunc("POST /api/stacks", func(rw http.ResponseWriter, r *http.Request) {
		p.record(r)

		var asked stackManifest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&asked))

		asked.Metadata.UUID = "stack-uuid"
		asked.Metadata.Slug = "shop-abcde"
		asked.Metadata.OwnerUUID = r.URL.Query().Get("owner")
		asked.Metadata.Owners = []kind.Reference{{Kind: "vm", UUID: "vm-uuid"}}
		asked.Metadata.CreatedAt = made
		asked.Spec.VM.UUID = "vm-uuid"
		asked.Status = stackKind.Status{Status: kind.Status{State: stackKind.Deploying, Expected: stackKind.Running}}

		answer(rw, http.StatusCreated, map[string]any{"resource": asked, "command": map[string]any{"id": "command-1", "action": "create"}})
	})

	mux.HandleFunc("POST /api/stacks/{uuid}/actions/{action}", func(rw http.ResponseWriter, r *http.Request) {
		p.record(r)

		body, _ := io.ReadAll(r.Body)

		p.lock.Lock()
		p.sent[r.PathValue("action")] = body
		status, answered := p.status, p.body
		p.lock.Unlock()

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(status)
		_, _ = io.WriteString(rw, answered)
	})

	mux.HandleFunc("GET /api/vms/{uuid}", func(rw http.ResponseWriter, r *http.Request) {
		p.record(r)

		p.lock.Lock()
		v, ok := p.vms[r.PathValue("uuid")]
		p.lock.Unlock()

		if !ok {
			rw.WriteHeader(http.StatusNotFound)

			return
		}

		answer(rw, http.StatusOK, v)
	})

	mux.HandleFunc("POST /api/vms/{uuid}/docker/containers.list", func(rw http.ResponseWriter, r *http.Request) {
		p.record(r)

		var filter noderequest.ContainersRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&filter))

		result, _ := json.Marshal([]noderequest.Container{{ID: "c1", Name: "shop-abcde-web-1", State: "running", Stack: filter.Stack, Service: "web"}})
		answer(rw, http.StatusOK, map[string]any{"result": json.RawMessage(result)})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	c, err := New(server.URL)
	require.NoError(t, err)

	return c, p
}

func answer(rw http.ResponseWriter, status int, body any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(body)
}

func (p *stacksPlane) record(r *http.Request) {
	p.lock.Lock()
	defer p.lock.Unlock()

	asked := r.Method + " " + r.URL.Path
	if len(r.URL.RawQuery) > 0 {
		asked += "?" + r.URL.RawQuery
	}

	p.asked = append(p.asked, asked)
}

func (p *stacksPlane) stack(uuid string) (stackManifest, bool) {
	p.lock.Lock()
	defer p.lock.Unlock()

	s, ok := p.stacks[uuid]

	return s, ok
}

func (p *stacksPlane) holding(s stackManifest, v vmManifest) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.stacks[s.Metadata.UUID] = s
	if len(v.Metadata.UUID) > 0 {
		p.vms[v.Metadata.UUID] = v
	}
}

func (p *stacksPlane) requests() []string {
	p.lock.Lock()
	defer p.lock.Unlock()

	return append([]string(nil), p.asked...)
}

func (p *stacksPlane) commanded(action string) (json.RawMessage, bool) {
	p.lock.Lock()
	defer p.lock.Unlock()

	body, ok := p.sent[action]

	return body, ok
}

// shopIn is the stack shop-abcde, in vmUUID, doing state.
func shopIn(vmUUID string, state kind.State) stackManifest {
	return stackManifest{
		Kind: stackKind.Name,
		Metadata: kind.Metadata{
			UUID:      "stack-uuid",
			Name:      "shop",
			Slug:      "shop-abcde",
			OwnerUUID: "owner-uuid",
			Owners:    []kind.Reference{{Kind: "vm", UUID: vmUUID}},
			Node:      "node-1",
			CreatedAt: made,
			UpdatedAt: made.Add(time.Minute),
		},
		Spec:   stackKind.Spec{VM: stackKind.VMChoice{UUID: vmUUID}, Compose: "services: {web: {image: nginx}}"},
		Status: stackKind.Status{Status: kind.Status{State: state, Expected: stackKind.Running, Reason: "a reason"}, Output: "compose said"},
	}
}

// dockerVM is a Docker VM on node-1, doing state, wanted expected.
func dockerVM(uuid string, state kind.State, expected kind.State) vmManifest {
	return vmManifest{
		Kind:     vmKind.Name,
		Metadata: kind.Metadata{UUID: uuid, Name: "docker-1", Labels: map[string]string{vmKind.LabelFlavor: string(vmKind.FlavorDocker)}, Node: "node-1"},
		Spec:     vmKind.Spec{Flavor: vmKind.FlavorDocker},
		Status:   vmKind.Status{Status: kind.Status{State: state, Expected: expected}},
	}
}

func TestClient_Stacks(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("a page of stacks is read off their manifests, each named as its vm is now, which is read once", func(t *testing.T) {
		t.Parallel()

		c, p := newStacksPlane(t)

		another := shopIn("vm-uuid", stackKind.Degraded)
		another.Metadata.UUID = "another-uuid"

		p.holding(shopIn("vm-uuid", stackKind.Running), dockerVM("vm-uuid", vmKind.Running, vmKind.Running))
		p.holding(another, vmManifest{})

		listed, err := c.Stacks(ctx, "owner-uuid", "vm-uuid", 2)
		require.NoError(t, err)

		require.Len(t, listed.Items, 2)
		assert.Equal(t, uint(1), listed.TotalPages)

		byUUID := map[string]stack.Stack{}
		for _, s := range listed.Items {
			byUUID[s.UUID] = s
		}

		assert.Equal(t, stack.Stack{
			UUID:          "stack-uuid",
			Name:          "shop",
			OwnerUUID:     "owner-uuid",
			VMUUID:        "vm-uuid",
			VMName:        "docker-1",
			Slug:          "shop-abcde",
			Compose:       "services: {web: {image: nginx}}",
			ExpectedState: stack.Running,
			State:         stack.Running,
			Reason:        "a reason",
			Output:        "compose said",
			CreatedAt:     made,
			UpdatedAt:     made.Add(time.Minute),
		}, byUUID["stack-uuid"])
		assert.Equal(t, stack.Degraded, byUUID["another-uuid"].State)

		assert.Equal(t, []string{
			"GET /api/stacks?owner=owner-uuid&page=2&parent=vm-uuid",
			"GET /api/vms/vm-uuid?owner=owner-uuid",
		}, p.requests())
	})

	t.Run("one whose vm is gone is named by nothing", func(t *testing.T) {
		t.Parallel()

		c, p := newStacksPlane(t)
		p.holding(shopIn("gone", stackKind.Waiting), vmManifest{})

		listed, err := c.Stacks(ctx, "", "", 1)
		require.NoError(t, err)
		require.Len(t, listed.Items, 1)
		assert.Empty(t, listed.Items[0].VMName)
		assert.Equal(t, stack.Waiting, listed.Items[0].State)
	})
}

func TestClient_Stack(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("a stack comes with the containers compose made for it, read from its vm now", func(t *testing.T) {
		t.Parallel()

		c, p := newStacksPlane(t)
		p.holding(shopIn("vm-uuid", stackKind.Running), dockerVM("vm-uuid", vmKind.Running, vmKind.Running))

		detail, err := c.Stack(ctx, "owner-uuid", "stack-uuid")
		require.NoError(t, err)

		assert.Equal(t, "docker-1", detail.VMName)
		assert.False(t, detail.VMNotRunning)
		require.Len(t, detail.Containers, 1)
		assert.Equal(t, "shop-abcde", detail.Containers[0].Stack, "the containers of its own project")
		assert.Equal(t, "web", detail.Containers[0].Service)
	})

	for name, tt := range map[string]struct {
		vm vmManifest
	}{
		"one whose vm is stopped has none to show, and says why": {vm: dockerVM("vm-uuid", vmKind.Stopped, vmKind.Stopped)},
		"and so does one whose vm is gone":                       {},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, p := newStacksPlane(t)
			p.holding(shopIn("vm-uuid", stackKind.Waiting), tt.vm)

			detail, err := c.Stack(ctx, "owner-uuid", "stack-uuid")
			require.NoError(t, err)

			assert.True(t, detail.VMNotRunning)
			assert.NotNil(t, detail.Containers)
			assert.Empty(t, detail.Containers)
			assert.NotContains(t, p.requests(), "POST /api/vms/vm-uuid/docker/containers.list?owner=owner-uuid", "its dockerd is not asked")
		})
	}

	t.Run("a stack that is not there is not", func(t *testing.T) {
		t.Parallel()

		c, _ := newStacksPlane(t)

		_, err := c.Stack(ctx, "owner-uuid", "stack-uuid")
		assert.ErrorIs(t, err, domain.ErrNotExists)
	})
}

func TestClient_CreateStack(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	c, p := newStacksPlane(t)
	p.holding(stackManifest{Metadata: kind.Metadata{UUID: "unrelated"}}, dockerVM("vm-uuid", vmKind.Scheduled, vmKind.Running))

	created, err := c.CreateStack(ctx, "owner-uuid", workloadControlPlane.StackRequest{
		Name:    "shop",
		Compose: "services: {web: {image: nginx}}",
		VM: workloadControlPlane.DockerVMChoice{New: &workloadControlPlane.NewDockerVM{
			Name:      "builds",
			Resources: &vm.Resources{Memory: 4 << 30},
			Ports:     []port.Port{},
			Network:   &vm.Network{Egress: vm.AccessDeny},
		}},
	})
	require.NoError(t, err)

	assert.Equal(t, workloadControlPlane.ChosenVM{UUID: "vm-uuid", Name: "docker-1", Created: true}, created.VM)
	assert.Equal(t, "stack-uuid", created.Stack.UUID)
	assert.Equal(t, "docker-1", created.Stack.VMName)
	assert.Equal(t, stack.Deploying, created.Stack.State)
	assert.Equal(t, stack.Running, created.Stack.ExpectedState)

	require.Contains(t, p.requests(), "POST /api/stacks?owner=owner-uuid")
}

func TestClient_CreateStack_asked(t *testing.T) {
	t.Parallel()

	var body json.RawMessage

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ = io.ReadAll(r.Body)
		}

		rw.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(rw, `{"errors":{"compose":"invalid_value"}}`)
	}))
	t.Cleanup(server.Close)

	c, err := New(server.URL)
	require.NoError(t, err)

	_, err = c.CreateStack(t.Context(), "owner-uuid", workloadControlPlane.StackRequest{
		Name:    "shop",
		Compose: "nope: [",
		VM:      workloadControlPlane.DockerVMChoice{New: &workloadControlPlane.NewDockerVM{Ports: []port.Port{}}},
	})

	var refused *ValidationError
	require.ErrorAs(t, err, &refused)
	assert.Equal(t, domain.ValidationErrors{"compose": "invalid_value"}, refused.ValidationErrors)

	assert.JSONEq(t, `{
		"kind": "stack",
		"metadata": {"name": "shop"},
		"spec": {"vm": {"new": {"ports": []}}, "compose": "nope: ["}
	}`, string(body), "a manifest of what was asked, ports given empty kept empty")
}

func TestClient_StackCommands(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	type command func(c *Client) error

	start := func(c *Client) error { return c.StartStack(ctx, "owner-uuid", "stack-uuid") }
	stop := func(c *Client) error { return c.StopStack(ctx, "owner-uuid", "stack-uuid") }
	restart := func(c *Client) error { return c.RestartStack(ctx, "owner-uuid", "stack-uuid") }

	for name, tt := range map[string]struct {
		state   kind.State
		vm      vmManifest
		command command
		action  string

		// asked says the command was sent; refused is what it is refused as.
		asked   bool
		refused domain.ValidationErrors
	}{
		"a stopped stack is started":                                {state: stackKind.Stopped, vm: dockerVM("vm-uuid", vmKind.Running, vmKind.Running), command: start, action: "start", asked: true},
		"a degraded one is started too":                             {state: stackKind.Degraded, vm: dockerVM("vm-uuid", vmKind.Running, vmKind.Running), command: start, action: "start", asked: true},
		"a running one is left as it is":                            {state: stackKind.Running, vm: dockerVM("vm-uuid", vmKind.Running, vmKind.Running), command: start, action: "start"},
		"as is one on its way up":                                   {state: stackKind.Deploying, vm: dockerVM("vm-uuid", vmKind.Running, vmKind.Running), command: restart, action: "restart"},
		"a running one is stopped":                                  {state: stackKind.Running, vm: dockerVM("vm-uuid", vmKind.Running, vmKind.Running), command: stop, action: "stop", asked: true},
		"a stopped one is left stopped":                             {state: stackKind.Stopped, vm: dockerVM("vm-uuid", vmKind.Running, vmKind.Running), command: stop, action: "stop"},
		"a running one is restarted":                                {state: stackKind.Running, vm: dockerVM("vm-uuid", vmKind.Running, vmKind.Running), command: restart, action: "restart", asked: true},
		"one waiting for its vm to come up is deployed then":        {state: stackKind.Waiting, vm: dockerVM("vm-uuid", vmKind.Starting, vmKind.Running), command: start, action: "start"},
		"and cannot be stopped before":                              {state: stackKind.Waiting, vm: dockerVM("vm-uuid", vmKind.Starting, vmKind.Running), command: stop, action: "stop", refused: domain.ValidationErrors{"stack": "invalid_state_transition"}},
		"one not in its vm, which runs, is started, which makes it": {state: stackKind.Waiting, vm: dockerVM("vm-uuid", vmKind.Running, vmKind.Running), command: start, action: "start", asked: true},
		"one in a vm that is stopped is refused":                    {state: stackKind.Waiting, vm: dockerVM("vm-uuid", vmKind.Stopped, vmKind.Stopped), command: start, action: "start", refused: domain.ValidationErrors{"vm": "vm_not_running"}},
		"as is one whose vm is gone":                                {state: stackKind.Stopped, command: start, action: "start", refused: domain.ValidationErrors{"vm": "vm_not_running"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c, p := newStacksPlane(t)
			p.holding(shopIn("vm-uuid", tt.state), tt.vm)

			err := tt.command(c)

			if len(tt.refused) > 0 {
				var refused *ValidationError
				require.ErrorAs(t, err, &refused)
				assert.Equal(t, tt.refused, refused.ValidationErrors)
			} else {
				require.NoError(t, err)
			}

			_, asked := p.commanded(tt.action)
			assert.Equal(t, tt.asked, asked, "%v", p.requests())
		})
	}

	t.Run("what the control plane refuses is said as the dashboard always said it", func(t *testing.T) {
		t.Parallel()

		c, p := newStacksPlane(t)
		p.holding(shopIn("vm-uuid", stackKind.Removing), dockerVM("vm-uuid", vmKind.Running, vmKind.Running))
		p.status, p.body = http.StatusBadRequest, `{"errors":{"action":"invalid_state_transition"}}`

		err := c.StopStack(ctx, "owner-uuid", "stack-uuid")

		var refused *ValidationError
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, domain.ValidationErrors{"stack": "invalid_state_transition"}, refused.ValidationErrors)

		p.status, p.body = http.StatusConflict, `{"error":{"code":"not_running","message":"the stack is on no node yet"}}`

		err = c.RestartStack(ctx, "owner-uuid", "stack-uuid")
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, domain.ValidationErrors{"vm": "vm_not_running"}, refused.ValidationErrors)
		assert.ErrorIs(t, err, vm.ErrNotRunning)
	})

	t.Run("a stack that is not there is not", func(t *testing.T) {
		t.Parallel()

		c, _ := newStacksPlane(t)

		assert.ErrorIs(t, c.StartStack(ctx, "owner-uuid", "stack-uuid"), domain.ErrNotExists)
	})
}

func TestClient_DeleteStack(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("a stack is deleted with its volumes when that is asked", func(t *testing.T) {
		t.Parallel()

		c, p := newStacksPlane(t)
		p.holding(shopIn("vm-uuid", stackKind.Running), dockerVM("vm-uuid", vmKind.Running, vmKind.Running))

		require.NoError(t, c.DeleteStack(ctx, "owner-uuid", "stack-uuid", true))

		body, asked := p.commanded("delete")
		require.True(t, asked)
		assert.JSONEq(t, `{"remove_volumes":true}`, string(body))
		assert.Contains(t, p.requests(), "POST /api/stacks/stack-uuid/actions/delete?owner=owner-uuid")
	})

	t.Run("and without them otherwise", func(t *testing.T) {
		t.Parallel()

		c, p := newStacksPlane(t)
		p.holding(shopIn("vm-uuid", stackKind.Deploying), dockerVM("vm-uuid", vmKind.Starting, vmKind.Running))

		require.NoError(t, c.DeleteStack(ctx, "owner-uuid", "stack-uuid", false))

		body, _ := p.commanded("delete")
		assert.JSONEq(t, `{}`, string(body))
	})

	t.Run("one in a vm that is stopped, and is to stay so, is refused: only its dockerd can take it down", func(t *testing.T) {
		t.Parallel()

		c, p := newStacksPlane(t)
		p.holding(shopIn("vm-uuid", stackKind.Waiting), dockerVM("vm-uuid", vmKind.Stopped, vmKind.Stopped))

		err := c.DeleteStack(ctx, "owner-uuid", "stack-uuid", false)

		var refused *ValidationError
		require.ErrorAs(t, err, &refused)
		assert.Equal(t, domain.ValidationErrors{"vm": "vm_not_running"}, refused.ValidationErrors)

		_, asked := p.commanded("delete")
		assert.False(t, asked)
	})

	t.Run("one whose vm is gone is deleted all the same", func(t *testing.T) {
		t.Parallel()

		c, p := newStacksPlane(t)
		p.holding(shopIn("gone", stackKind.Waiting), vmManifest{})
		p.status, p.body = http.StatusNoContent, ""

		require.NoError(t, c.DeleteStack(ctx, "owner-uuid", "stack-uuid", false))

		_, asked := p.commanded("delete")
		assert.True(t, asked)
	})
}
