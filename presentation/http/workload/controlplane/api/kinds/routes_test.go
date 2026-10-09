package kinds

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/actOnResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getKinds"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResources"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/queryResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/recordResult"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/waiters"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

// node is the node fans are held on: it carries out the commands it is sent
// through the fan's own node binding, as an orchestrator does, and sends what
// came of them back to the control plane's result handler. A silent one
// hears its commands and carries none of them out.
type node struct {
	binding kind.NodeBinding
	results domain.MessageHandler
	silent  atomic.Bool

	lock sync.Mutex
	sent []kind.ActOnResource
}

func (n *node) Produce(_ context.Context, subject string, payload []byte) error {
	if subject != kind.ActOnResourceName {
		return nil
	}

	var command kind.ActOnResource
	if err := json.Unmarshal(payload, &command); err != nil {
		return err
	}

	n.lock.Lock()
	n.sent = append(n.sent, command)
	n.lock.Unlock()

	if n.silent.Load() {
		return nil
	}

	go func() {
		result := n.binding.Execute(context.Background(), command)
		result.At = time.Now()

		answer, _ := json.Marshal(result)
		_ = n.results.Handle(context.Background(), answer)
	}()

	return nil
}

func (n *node) commands() []kind.ActOnResource {
	n.lock.Lock()
	defer n.lock.Unlock()

	return append([]kind.ActOnResource(nil), n.sent...)
}

// api is the control plane's resource API with fans registered in it, over
// fans kept in memory and a node that holds them.
type api struct {
	handler   http.Handler
	resources *resourcesMemory.Repository
	fans      *kindstest.Fans
	held      *kindstest.Node
	node      *node
}

func newAPI(t *testing.T) *api {
	t.Helper()

	a := &api{
		resources: resourcesMemory.NewRepository(),
		fans:      &kindstest.Fans{Node: kindstest.NodeName},
		held:      &kindstest.Node{},
	}

	registry := kindstest.Registry(a.fans)
	logger := slog.New(slog.DiscardHandler)
	waiting := waiters.New()

	nodeBinding := kind.BindNode[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), a.held)

	a.node = &node{binding: nodeBinding, results: recordResult.NewResult(registry, a.resources, waiting, logger, nil)}

	requester := &messagingMock.Requester{Answer: func(ctx context.Context, _ string, request noderequest.Request) (noderequest.Reply, error) {
		query, err := kind.QueryOf(request)
		if err != nil {
			return noderequest.Failed(err), nil
		}

		answer, err := nodeBinding.Query(ctx, query)
		if err != nil {
			return noderequest.Failed(err), nil
		}

		return noderequest.Reply{OK: true, Result: answer}, nil
	}}

	dispatcher := dispatch.New(a.resources, a.node, waiting, nil, dispatch.PollEvery(10*time.Millisecond))
	observer := observe.NewObserver(registry, a.resources, logger)

	mux := http.NewServeMux()
	require.NoError(t, Route(mux, registry.Descriptors(), UseCases{
		Admit:  admitResource.NewUseCase(registry, a.resources, dispatcher, logger),
		Act:    actOnResource.NewUseCase(registry, a.resources, dispatcher, slog.New(slog.DiscardHandler)),
		Delete: deleteResource.NewUseCase(registry, a.resources, dispatcher),
		Get:    getResource.NewUseCase(registry, a.resources),
		List:   getResources.NewUseCase(registry, a.resources),
		Query:  queryResource.NewUseCase(registry, a.resources, requester, observer, nil),
		Kinds:  getKinds.NewUseCase(registry),
	}))

	a.handler = mux

	return a
}

// do makes a request of the API, and is its status and its body.
func (a *api) do(t *testing.T, method string, target string, body string) (int, string) {
	t.Helper()

	var reader io.Reader
	if len(body) > 0 {
		reader = strings.NewReader(body)
	}

	recorder := httptest.NewRecorder()
	a.handler.ServeHTTP(recorder, httptest.NewRequest(method, target, reader))

	return recorder.Code, recorder.Body.String()
}

// keep keeps a fan, and puts it on its node as it is.
func (a *api) keep(t *testing.T, r resource.Record) {
	t.Helper()

	_, err := a.resources.Create(context.Background(), r)
	require.NoError(t, err)

	a.held.Hold(kindstest.Typed(r))
}

func (a *api) stored(t *testing.T, uuid string) (kindstest.Fan, bool) {
	t.Helper()

	r, err := a.resources.GetOne(context.Background(), kindstest.Kind, uuid)
	if err != nil {
		return kindstest.Fan{}, false
	}

	return kindstest.Typed(r), true
}

func decode[T any](t *testing.T, body string) T {
	t.Helper()

	var value T
	require.NoError(t, json.Unmarshal([]byte(body), &value), body)

	return value
}

type commandedBody struct {
	Resource *kindstest.Fan `json:"resource"`
	Command  *command       `json:"command"`
	Result   *result        `json:"result"`
}

func TestRoutes(t *testing.T) {
	t.Parallel()

	t.Run("the kinds are served as they describe themselves", func(t *testing.T) {
		t.Parallel()

		status, body := newAPI(t).do(t, http.MethodGet, "/api/kinds", "")
		require.Equal(t, http.StatusOK, status)

		kinds := decode[struct {
			Items []kind.Descriptor `json:"items"`
		}](t, body)

		require.Len(t, kinds.Items, 1)
		assert.Equal(t, "fan", kinds.Items[0].Name)
		assert.Equal(t, "fans", kinds.Items[0].Plural)
		assert.Equal(t, kindstest.Machine(), kinds.Items[0].Machine)
	})

	t.Run("a resource is admitted, made by its node, and answered once it is, when asked to wait", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)

		status, body := a.do(t, http.MethodPost, "/api/fans?owner=owner-uuid&parent=house-1&wait=10s", `{"metadata":{"name":"kitchen","labels":{"room":"kitchen"}},"spec":{"blades":3}}`)
		require.Equal(t, http.StatusCreated, status, body)

		admitted := decode[commandedBody](t, body)

		require.NotNil(t, admitted.Resource)
		assert.Equal(t, "fan", admitted.Resource.Kind)
		assert.Equal(t, "kitchen", admitted.Resource.Metadata.Name)
		assert.Equal(t, "owner-uuid", admitted.Resource.Metadata.OwnerUUID)
		assert.Equal(t, []kind.Reference{{Kind: "house", UUID: "house-1"}}, admitted.Resource.Metadata.Owners)
		assert.Equal(t, kindstest.Running, admitted.Resource.Status.State, "made, and running")
		assert.Equal(t, 1, admitted.Resource.Status.Speed)

		require.NotNil(t, admitted.Command)
		assert.Equal(t, "create", admitted.Command.Action)
		assert.Equal(t, kindstest.NodeName, admitted.Command.Node)

		require.NotNil(t, admitted.Result)
		assert.True(t, admitted.Result.OK)
		assert.Equal(t, admitted.Command.ID, admitted.Result.ID)
		assert.Equal(t, "create done", admitted.Result.Output)
	})

	t.Run("without waiting, it is answered as soon as it is kept", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.node.silent.Store(true)

		status, body := a.do(t, http.MethodPost, "/api/fans?owner=owner-uuid", `{"spec":{"blades":3}}`)
		require.Equal(t, http.StatusCreated, status, body)

		admitted := decode[commandedBody](t, body)
		assert.Equal(t, kindstest.Starting, admitted.Resource.Status.State)
		assert.NotNil(t, admitted.Command)
		assert.Nil(t, admitted.Result)
	})

	for name, tt := range map[string]struct {
		target string
		body   string
		want   string
	}{
		"what is not a manifest is refused": {
			target: "/api/fans?owner=owner-uuid", body: `{"spec":`,
			want: `{"errors":{"body":"invalid_value"}}`,
		},
		"and so is what its kind refuses": {
			target: "/api/fans?owner=owner-uuid", body: `{"spec":{"blades":0}}`,
			want: `{"errors":{"blades":"required_field"}}`,
		},
		"and a resource for nobody": {
			target: "/api/fans", body: `{"spec":{"blades":3}}`,
			want: `{"errors":{"owner":"required_field"}}`,
		},
		"and a wait that is not one": {
			target: "/api/fans?owner=owner-uuid&wait=soon", body: `{"spec":{"blades":3}}`,
			want: `{"errors":{"wait":"invalid_value"}}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			a := newAPI(t)

			status, body := a.do(t, http.MethodPost, tt.target, tt.body)

			assert.Equal(t, http.StatusBadRequest, status)
			assert.JSONEq(t, tt.want, body)
			assert.Equal(t, 0, a.resources.Len(kindstest.Kind))
		})
	}

	t.Run("a kind's resources are listed, narrowed to an owner's, to a parent's and to those labelled so", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.keep(t, kindstest.AFan("fan-1", kindstest.Running, kindstest.Running, func(f *kindstest.Fan) {
			f.Metadata.Labels = map[string]string{"made.by": "hand", "room": "kitchen"}
		}))
		a.keep(t, kindstest.AFan("fan-2", kindstest.Running, kindstest.Running, func(f *kindstest.Fan) {
			f.Metadata.OwnerUUID = "somebody-else"
			f.Metadata.Owners = []kind.Reference{{Kind: "house", UUID: "house-2"}}
			f.Metadata.Labels = map[string]string{"made.by": "hand"}
		}))

		for target, want := range map[string][]string{
			"/api/fans":                                       {"fan-2", "fan-1"},
			"/api/fans?owner=owner-uuid":                      {"fan-1"},
			"/api/fans?parent=house-2":                        {"fan-2"},
			"/api/fans?owner=nobody":                          {},
			"/api/fans?owner=owner-uuid&page=2":               {},
			"/api/fans?label=made.by%3Dhand":                  {"fan-2", "fan-1"},
			"/api/fans?label=made.by%3Dhand&label=room%3D":    {},
			"/api/fans?label=made.by=hand&label=room=kitchen": {"fan-1"},
		} {
			status, body := a.do(t, http.MethodGet, target, "")
			require.Equal(t, http.StatusOK, status, target)

			page := decode[struct {
				Items      []kindstest.Fan `json:"items"`
				Pagination struct {
					TotalPages  uint `json:"total_pages"`
					CurrentPage uint `json:"current_page"`
				} `json:"pagination"`
			}](t, body)

			uuids := []string{}
			for _, item := range page.Items {
				uuids = append(uuids, item.Metadata.UUID)
			}

			assert.Equal(t, want, uuids, target)
		}
	})

	t.Run("a label that is no key and value is refused", func(t *testing.T) {
		t.Parallel()

		status, body := newAPI(t).do(t, http.MethodGet, "/api/fans?label=made.by", "")

		assert.Equal(t, http.StatusBadRequest, status)
		assert.JSONEq(t, `{"errors":{"label":"invalid_value"}}`, body)
	})

	t.Run("one is read by its uuid, as its owner's or anybody's", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.keep(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		status, body := a.do(t, http.MethodGet, "/api/fans/fan-uuid?owner=owner-uuid", "")
		require.Equal(t, http.StatusOK, status)

		fan := decode[kindstest.Fan](t, body)
		assert.Equal(t, "fan-uuid", fan.Metadata.UUID)
		assert.Equal(t, kindstest.Running, fan.Status.State)

		status, _ = a.do(t, http.MethodGet, "/api/fans/fan-uuid", "")
		assert.Equal(t, http.StatusOK, status)

		status, _ = a.do(t, http.MethodGet, "/api/fans/fan-uuid?owner=somebody-else", "")
		assert.Equal(t, http.StatusNotFound, status, "somebody else's is not there")

		status, _ = a.do(t, http.MethodGet, "/api/fans/another-uuid", "")
		assert.Equal(t, http.StatusNotFound, status)
	})

	t.Run("inside a parent, one is read, asked and queried as its parent's, and nowhere else", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.keep(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		status, _ := a.do(t, http.MethodGet, "/api/fans/fan-uuid?parent="+kindstest.House, "")
		assert.Equal(t, http.StatusOK, status)

		status, _ = a.do(t, http.MethodGet, "/api/fans/fan-uuid?parent=house-2", "")
		assert.Equal(t, http.StatusNotFound, status, "it does not live in that house")

		status, _ = a.do(t, http.MethodPost, "/api/fans/fan-uuid/actions/stop?parent=house-2", "")
		assert.Equal(t, http.StatusNotFound, status)

		status, _ = a.do(t, http.MethodGet, "/api/fans/fan-uuid/state?parent=house-2", "")
		assert.Equal(t, http.StatusNotFound, status)

		status, _ = a.do(t, http.MethodDelete, "/api/fans/fan-uuid?parent=house-2", "")
		assert.Equal(t, http.StatusNotFound, status)

		_, kept := a.stored(t, "fan-uuid")
		assert.True(t, kept)
		assert.Empty(t, a.node.commands(), "nothing was asked of it")
	})

	t.Run("a command is sent to its node and, when asked to be, waited for", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.keep(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		status, body := a.do(t, http.MethodPost, "/api/fans/fan-uuid/actions/stop?owner=owner-uuid&wait=10s", "")
		require.Equal(t, http.StatusOK, status, body)

		stopped := decode[commandedBody](t, body)
		require.NotNil(t, stopped.Result)
		assert.True(t, stopped.Result.OK)
		assert.Equal(t, "stop", stopped.Command.Action)
		assert.Equal(t, kindstest.Stopped, stopped.Resource.Status.State)
		assert.Equal(t, kindstest.Stopped, stopped.Resource.Status.Expected)

		status, body = a.do(t, http.MethodPost, "/api/fans/fan-uuid/actions/start?wait=10s", `{"speed": 3}`)
		require.Equal(t, http.StatusOK, status, body)

		started := decode[commandedBody](t, body)
		assert.Equal(t, kindstest.Running, started.Resource.Status.State)
		assert.Equal(t, 3, started.Resource.Status.Speed, "at the speed it was asked for")
	})

	t.Run("one not waited for is answered as on its way", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.node.silent.Store(true)
		a.keep(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		status, body := a.do(t, http.MethodPost, "/api/fans/fan-uuid/actions/stop", "")
		require.Equal(t, http.StatusAccepted, status, body)

		stopping := decode[commandedBody](t, body)
		assert.Equal(t, kindstest.Stopping, stopping.Resource.Status.State)
		require.NotNil(t, stopping.Command)
		assert.Equal(t, []string{stopping.Command.ID}, ids(a.node.commands()))
	})

	t.Run("one run in the control plane is carried out there and then", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.keep(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		status, body := a.do(t, http.MethodPost, "/api/fans/fan-uuid/actions/rename", `{"name":"hall"}`)
		require.Equal(t, http.StatusOK, status, body)

		renamed := decode[commandedBody](t, body)
		assert.Equal(t, "hall", renamed.Resource.Metadata.Name)
		assert.Nil(t, renamed.Command)
		assert.Empty(t, a.node.commands())
	})

	t.Run("a command for a node, of one on none, has what it desires written down, and is sent nowhere", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.keep(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, func(f *kindstest.Fan) {
			f.Metadata.Node = ""
		}))

		status, body := a.do(t, http.MethodPost, "/api/fans/fan-uuid/actions/stop", "")
		require.Equal(t, http.StatusOK, status, body)

		stopped := decode[commandedBody](t, body)
		assert.Nil(t, stopped.Command)

		fan, _ := a.stored(t, "fan-uuid")
		assert.Equal(t, kindstest.Running, fan.Status.State)
		assert.Equal(t, kindstest.Stopped, fan.Status.Expected)
		assert.Empty(t, a.node.commands())
	})

	for name, tt := range map[string]struct {
		target string
		body   string
		status int
		want   string
	}{
		"a command its state does not allow is refused": {
			target: "/api/fans/fan-uuid/actions/start",
			status: http.StatusBadRequest, want: `{"errors":{"action":"invalid_state_transition"}}`,
		},
		"and so is one asked with what is not valid": {
			target: "/api/fans/fan-uuid/actions/rename", body: `{}`,
			status: http.StatusBadRequest, want: `{"errors":{"name":"required_field"}}`,
		},
		"or with what cannot be read": {
			target: "/api/fans/fan-uuid/actions/rename", body: `{"name":`,
			status: http.StatusBadRequest, want: `{"errors":{"payload":"invalid_value"}}`,
		},
		"an internal command is nobody's to ask for": {
			target: "/api/fans/fan-uuid/actions/create",
			status: http.StatusNotFound,
		},
		"nor is a command the kind does not have": {
			target: "/api/fans/fan-uuid/actions/explode",
			status: http.StatusNotFound,
		},
		"nor anything of somebody else's": {
			target: "/api/fans/fan-uuid/actions/stop?owner=somebody-else",
			status: http.StatusNotFound,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			a := newAPI(t)
			a.keep(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

			status, body := a.do(t, http.MethodPost, tt.target, tt.body)

			assert.Equal(t, tt.status, status, body)
			if len(tt.want) > 0 {
				assert.JSONEq(t, tt.want, body)
			}

			fan, _ := a.stored(t, "fan-uuid")
			assert.Equal(t, kindstest.Running, fan.Status.State)
			assert.Empty(t, a.node.commands())
		})
	}

	t.Run("a resource is deleted by its node, and is gone once it says so", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.keep(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		status, body := a.do(t, http.MethodDelete, "/api/fans/fan-uuid?owner=owner-uuid&wait=10s", "")
		require.Equal(t, http.StatusNoContent, status, body)

		_, kept := a.stored(t, "fan-uuid")
		assert.False(t, kept)
	})

	t.Run("not waited for, it is on its way out", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.node.silent.Store(true)
		a.keep(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		status, body := a.do(t, http.MethodDelete, "/api/fans/fan-uuid", "")
		require.Equal(t, http.StatusAccepted, status, body)

		deleting := decode[commandedBody](t, body)
		assert.Equal(t, kindstest.Deleting, deleting.Resource.Status.State)
		assert.Equal(t, "delete", deleting.Command.Action)
	})

	t.Run("and one with nothing anywhere to delete is gone at once", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.keep(t, kindstest.AFan("fan-uuid", kindstest.Pending, kindstest.Running, func(f *kindstest.Fan) { f.Metadata.Node = "" }))

		status, _ := a.do(t, http.MethodDelete, "/api/fans/fan-uuid", "")
		assert.Equal(t, http.StatusNoContent, status)
		assert.Empty(t, a.node.commands())
	})

	t.Run("somebody else's is not there to delete", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.keep(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		status, _ := a.do(t, http.MethodDelete, "/api/fans/fan-uuid?owner=somebody-else", "")
		assert.Equal(t, http.StatusNotFound, status)
	})

	t.Run("a query is asked of its node, its payload as parameters", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.keep(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		status, body := a.do(t, http.MethodGet, "/api/fans/fan-uuid/logs?owner=owner-uuid&tail=10&since=2026-10-06T12:00:00Z", "")
		require.Equal(t, http.StatusOK, status, body)

		assert.JSONEq(t, `{"result":["the last 10 lines of kitchen"]}`, body)
	})

	t.Run("and a resource's state is its node's answer, written down", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)
		a.keep(t, kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

		held := kindstest.Typed(kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))
		held.Status.Speed = 3
		a.held.Hold(held)

		status, body := a.do(t, http.MethodGet, "/api/fans/fan-uuid/state", "")
		require.Equal(t, http.StatusOK, status, body)

		observed := decode[struct {
			Result kind.Observed[kindstest.Status] `json:"result"`
		}](t, body)
		assert.Equal(t, 3, observed.Result.Status.Speed)

		fan, _ := a.stored(t, "fan-uuid")
		assert.Equal(t, 3, fan.Status.Speed)
	})

	for name, tt := range map[string]struct {
		state  kind.State
		target string
		status int
	}{
		"a query its state does not allow is not asked": {state: kindstest.Stopped, target: "/api/fans/fan-uuid/logs", status: http.StatusConflict},
		"nor one asked with what is not valid":          {state: kindstest.Running, target: "/api/fans/fan-uuid/logs?tail=5000", status: http.StatusBadRequest},
		"nor with what cannot be read":                  {state: kindstest.Running, target: "/api/fans/fan-uuid/logs?tail=lots", status: http.StatusBadRequest},
		"a command is not a query":                      {state: kindstest.Running, target: "/api/fans/fan-uuid/stop", status: http.StatusNotFound},
		"nor is what the kind does not have":            {state: kindstest.Running, target: "/api/fans/fan-uuid/temperature", status: http.StatusNotFound},
		"nor anything of somebody else's":               {state: kindstest.Running, target: "/api/fans/fan-uuid/state?owner=somebody-else", status: http.StatusNotFound},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			a := newAPI(t)
			a.keep(t, kindstest.AFan("fan-uuid", tt.state, tt.state))

			status, body := a.do(t, http.MethodGet, tt.target, "")

			assert.Equal(t, tt.status, status, body)
		})
	}

	t.Run("what a node refused is said with the status its code stands for", func(t *testing.T) {
		t.Parallel()

		a := newAPI(t)

		_, err := a.resources.Create(context.Background(), kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))
		require.NoError(t, err)

		// kept, and not on its node.
		status, body := a.do(t, http.MethodGet, "/api/fans/fan-uuid/logs", "")

		assert.Equal(t, http.StatusNotFound, status)
		assert.Contains(t, body, `"code":"not_found"`)
	})
}

func ids(commands []kind.ActOnResource) []string {
	var listed []string
	for _, command := range commands {
		listed = append(listed, command.ID)
	}

	return listed
}

func TestRoute(t *testing.T) {
	t.Parallel()

	useCases := func() UseCases { return UseCases{} }

	t.Run("with no kind registered, only the kinds are served, and today's routes are left as they are", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.Handle("GET /api/vms", http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusTeapot) }))

		require.NoError(t, Route(mux, nil, UseCases{Kinds: getKinds.NewUseCase(kind.NewRegistry[kind.ControlPlaneBinding]())}))

		for target, want := range map[string]int{
			"/api/kinds":         http.StatusOK,
			"/api/vms":           http.StatusTeapot,
			"/api/fans":          http.StatusNotFound,
			"/api/vms/uuid/logs": http.StatusNotFound,
		} {
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))

			assert.Equal(t, want, recorder.Code, target)
		}
	})

	t.Run("a kind registered under a plural whose routes are taken is refused, rather than shadowing them", func(t *testing.T) {
		t.Parallel()

		mux := http.NewServeMux()
		mux.Handle("GET /api/fans", http.NotFoundHandler())

		err := Route(mux, []kind.Descriptor{kindstest.Descriptor()}, useCases())

		assert.ErrorContains(t, err, "/api/fans")
	})

	t.Run("and so is one named after the kinds themselves", func(t *testing.T) {
		t.Parallel()

		d := kindstest.Descriptor()
		d.Plural = "kinds"

		err := Route(http.NewServeMux(), []kind.Descriptor{d}, useCases())

		assert.Error(t, err)
	})
}

func TestWaitOf(t *testing.T) {
	t.Parallel()

	for asked, want := range map[string]struct {
		wait time.Duration
		ok   bool
	}{
		"":       {wait: 0, ok: true},
		"30":     {wait: 30 * time.Second, ok: true},
		"90s":    {wait: 90 * time.Second, ok: true},
		"1m30s":  {wait: 90 * time.Second, ok: true},
		"100000": {wait: MaxWait, ok: true},
		"10h":    {wait: MaxWait, ok: true},
		"-1s":    {ok: false},
		"soon":   {ok: false},
	} {
		t.Run(asked, func(t *testing.T) {
			t.Parallel()

			wait, ok := waitOf(httptest.NewRequest(http.MethodGet, "/api/fans?wait="+asked, nil))

			assert.Equal(t, want.ok, ok)
			assert.Equal(t, want.wait, wait)
		})
	}
}

func TestPayloadOf(t *testing.T) {
	t.Parallel()

	type inner struct {
		Depth int `json:"depth"`
	}

	type payload struct {
		inner

		Since   time.Time `json:"since"`
		Tail    uint      `json:"tail"`
		Name    string    `json:"name,omitempty"`
		Follow  bool      `json:"follow"`
		Tags    []string  `json:"tags"`
		Counts  []int     `json:"counts"`
		Ignored string    `json:"-"`
		Plain   int
	}

	for name, tt := range map[string]struct {
		query string
		want  string
	}{
		"a parameter is its field, as its JSON is": {
			query: "tail=10&since=2026-10-06T12:00:00Z&follow=true&Plain=3",
			want:  `{"tail":10,"since":"2026-10-06T12:00:00Z","follow":true,"Plain":3}`,
		},
		"text is text, even when it reads as a number": {
			query: "name=42",
			want:  `{"name":"42"}`,
		},
		"what is not JSON for a field that is not text is left for its codec to refuse": {
			query: "tail=lots",
			want:  `{"tail":"lots"}`,
		},
		"one given more than once is a list": {
			query: "tags=a&tags=b&counts=1&counts=2",
			want:  `{"tags":["a","b"],"counts":[1,2]}`,
		},
		"an embedded struct's fields are the payload's": {
			query: "depth=2",
			want:  `{"depth":2}`,
		},
		"what names no field is the request's, not the payload's": {
			query: "owner=owner-uuid&wait=10s&Ignored=x",
			want:  ``,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := payloadOf(httptest.NewRequest(http.MethodGet, "/?"+tt.query, nil).URL.Query(), reflect.TypeFor[payload]())
			require.NoError(t, err)

			if len(tt.want) == 0 {
				assert.Nil(t, got)

				return
			}

			assert.JSONEq(t, tt.want, string(got))
		})
	}

	t.Run("an action asked with nothing is asked with nothing", func(t *testing.T) {
		t.Parallel()

		got, err := payloadOf(httptest.NewRequest(http.MethodGet, "/?tail=10", nil).URL.Query(), nil)
		require.NoError(t, err)

		assert.Nil(t, got)
	})
}
