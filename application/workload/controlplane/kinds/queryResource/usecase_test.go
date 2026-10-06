package queryResource_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/kindstest"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/observe"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/queryResource"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	resourcesMemory "github.com/khanzadimahdi/testproject/infrastructure/repository/memory/workload/resources"
)

type fixture struct {
	resources *resourcesMemory.Repository
	requester *messagingMock.Requester
	node      *kindstest.Node
	clock     *kindstest.Clock
	useCase   *queryResource.UseCase
}

// newFixture is a query over records of the fan as descriptor has it, asked
// of a node that answers through the fan's own node binding, as an
// orchestrator does.
func newFixture(t *testing.T, descriptor kind.Descriptor, records ...resource.Record) *fixture {
	t.Helper()

	f := &fixture{
		resources: resourcesMemory.NewRepository(),
		requester: &messagingMock.Requester{},
		node:      &kindstest.Node{},
		clock:     kindstest.NewClock(),
	}

	for _, r := range records {
		_, err := f.resources.Create(context.Background(), r)
		require.NoError(t, err)
	}

	nodeBinding := kind.BindNode[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), f.node)

	f.requester.Answer = func(ctx context.Context, _ string, request noderequest.Request) (noderequest.Reply, error) {
		query, err := kind.QueryOf(request)
		if err != nil {
			return noderequest.Failed(err), nil
		}

		answer, err := nodeBinding.Query(ctx, query)
		if err != nil {
			return noderequest.Failed(err), nil
		}

		return noderequest.Reply{OK: true, Result: answer}, nil
	}

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](descriptor, &kindstest.Fans{})))

	observer := observe.NewObserver(registry, f.resources, slog.New(slog.DiscardHandler))
	f.useCase = queryResource.NewUseCase(registry, f.resources, f.requester, observer, f.clock.Now)

	return f
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	running := func(f *kindstest.Fan) { f.Metadata.Name = "kitchen" }

	t.Run("a query is asked of the node holding the resource, as the kind's node request", func(t *testing.T) {
		t.Parallel()

		record := kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running, running)

		f := newFixture(t, kindstest.Descriptor(), record)
		f.node.Hold(kindstest.Typed(record))

		response, err := f.useCase.Execute(ctx, &queryResource.Request{
			Kind:      kindstest.Kind,
			OwnerUUID: kindstest.OwnerUUID,
			UUID:      "fan-uuid",
			Action:    "logs",
			Payload:   json.RawMessage(` {"tail": 10} `),
		})
		require.NoError(t, err)

		require.Nil(t, response.NodeError)
		assert.JSONEq(t, `["the last 10 lines of kitchen"]`, string(response.Result))

		asked := f.requester.Asked()
		require.Len(t, asked, 1)
		assert.Equal(t, kindstest.NodeName, asked[0].NodeName)
		assert.Equal(t, noderequest.Op("fan.logs"), asked[0].Request.Op)
		assert.Equal(t, "fan-uuid", asked[0].Request.VMUUID, "the resource's uuid, whatever its kind")

		query, err := kind.QueryOf(asked[0].Request)
		require.NoError(t, err)
		assert.Equal(t, `{"tail":10}`, string(query.Payload))
		assert.Equal(t, "fan-uuid", query.Resource.Metadata.UUID, "it carries the resource as it is recorded")
	})

	t.Run("its state, asked of its node, is written down as an observation", func(t *testing.T) {
		t.Parallel()

		record := kindstest.AFan("fan-uuid", kindstest.Starting, kindstest.Running)
		record.Pending = &resource.Pending{Action: "start", IDs: []string{"command-1"}}

		f := newFixture(t, kindstest.Descriptor(), record)

		held := kindstest.Typed(record)
		held.Status.State = kindstest.Running
		held.Status.Speed = 2
		f.node.Hold(held)

		response, err := f.useCase.Execute(ctx, &queryResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "state"})
		require.NoError(t, err)

		var observed kind.Observed[kindstest.Status]
		require.NoError(t, json.Unmarshal(response.Result, &observed))
		assert.Equal(t, kindstest.Running, observed.Status.State)

		stored, err := f.resources.GetOne(ctx, kindstest.Kind, "fan-uuid")
		require.NoError(t, err)

		fan := kindstest.Typed(stored)
		assert.Equal(t, kindstest.Running, fan.Status.State, "it arrived, as its node says")
		assert.Equal(t, 2, fan.Status.Speed)
		assert.Equal(t, f.clock.Now(), fan.Status.ObservedAt)
		assert.Nil(t, stored.Pending)
	})

	t.Run("the state of a kind known in the control plane is answered from its record", func(t *testing.T) {
		t.Parallel()

		d := kindstest.Descriptor()
		d.StateBy = kind.OnControlPlane
		for i := range d.Actions {
			if d.Actions[i].Name == "state" {
				d.Actions[i].Runs = kind.OnControlPlane
			}
		}

		f := newFixture(t, d, kindstest.AFan("fan-uuid", kindstest.Stopped, kindstest.Stopped))

		response, err := f.useCase.Execute(ctx, &queryResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "state"})
		require.NoError(t, err)

		var observed kind.Observed[kindstest.Status]
		require.NoError(t, json.Unmarshal(response.Result, &observed))

		assert.Equal(t, kindstest.Kind, observed.Kind)
		assert.Equal(t, "fan-uuid", observed.UUID)
		assert.Equal(t, kindstest.Stopped, observed.Status.State)
		assert.Equal(t, []kind.Reference{{Kind: kindstest.Parent, UUID: kindstest.House}}, observed.Owners)
		assert.Empty(t, f.requester.Asked(), "no node is asked")
	})

	for name, tt := range map[string]struct {
		state   kind.State
		node    string
		answer  func(ctx context.Context, nodeName string, request noderequest.Request) (noderequest.Reply, error)
		request queryResource.Request
		invalid domain.ValidationErrors
		code    noderequest.Code
	}{
		"a query its state does not allow is not asked": {
			state:   kindstest.Stopped,
			request: queryResource.Request{Action: "logs"},
			code:    noderequest.CodeNotRunning,
		},
		"nor one of a resource on no node": {
			state:   kindstest.Running,
			node:    "-",
			request: queryResource.Request{Action: "logs"},
			code:    noderequest.CodeNotRunning,
		},
		"nor one asked with what is not valid": {
			state:   kindstest.Running,
			request: queryResource.Request{Action: "logs", Payload: json.RawMessage(`{"tail": 5000}`)},
			invalid: domain.ValidationErrors{"tail": "invalid_value"},
		},
		"nor one asked with what cannot be read": {
			state:   kindstest.Running,
			request: queryResource.Request{Action: "logs", Payload: json.RawMessage(`{"tail": "all"}`)},
			invalid: domain.ValidationErrors{"payload": "invalid_value"},
		},
		"what its node refused is said as it said it": {
			state:   kindstest.Running,
			request: queryResource.Request{Action: "logs"},
			code:    noderequest.CodeNotFound,
		},
		"a node that does not answer in time is a timeout": {
			state:   kindstest.Running,
			request: queryResource.Request{Action: "logs"},
			answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
				return noderequest.Reply{}, context.DeadlineExceeded
			},
			code: noderequest.CodeTimeout,
		},
		"and one that is not there to ask is not answering": {
			state:   kindstest.Running,
			request: queryResource.Request{Action: "logs"},
			answer: func(context.Context, string, noderequest.Request) (noderequest.Reply, error) {
				return noderequest.Reply{}, errors.New("no responders")
			},
			code: noderequest.CodeInternal,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, kindstest.Descriptor(), kindstest.AFan("fan-uuid", tt.state, tt.state, func(fan *kindstest.Fan) {
				if tt.node == "-" {
					fan.Metadata.Node = ""
				}
			}))

			if tt.answer != nil {
				f.requester.Answer = tt.answer
			}

			request := tt.request
			request.Kind = kindstest.Kind
			request.UUID = "fan-uuid"

			response, err := f.useCase.Execute(ctx, &request)
			require.NoError(t, err)

			assert.Equal(t, tt.invalid, response.ValidationErrors)
			assert.Nil(t, response.Result)

			if len(tt.code) > 0 {
				require.NotNil(t, response.NodeError)
				assert.Equal(t, tt.code, response.NodeError.Code)
			}
		})
	}

	for name, tt := range map[string]struct {
		request queryResource.Request
		err     error
	}{
		"a command is not a query":               {request: queryResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "stop"}, err: kind.ErrUnknownAction},
		"nor is what the kind does not have":     {request: queryResource.Request{Kind: kindstest.Kind, UUID: "fan-uuid", Action: "explode"}, err: kind.ErrUnknownAction},
		"somebody else's is not there":           {request: queryResource.Request{Kind: kindstest.Kind, OwnerUUID: "somebody-else", UUID: "fan-uuid", Action: "state"}, err: domain.ErrNotExists},
		"nor is anything of a kind not run here": {request: queryResource.Request{Kind: "kettle", UUID: "fan-uuid", Action: "state"}, err: kind.ErrUnknownKind},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, kindstest.Descriptor(), kindstest.AFan("fan-uuid", kindstest.Running, kindstest.Running))

			_, err := f.useCase.Execute(ctx, &tt.request)

			assert.ErrorIs(t, err, tt.err)
			assert.Empty(t, f.requester.Asked())
		})
	}
}

func TestUseCase_Execute_extras(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	shelf := kindstest.NewShelf(kindstest.AShelvedFan("shelved"))
	requester := &messagingMock.Requester{}

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), &kindstest.Shelved{Fans: &kindstest.Fans{}, Shelf: shelf})))

	useCase := queryResource.NewUseCase(registry, resourcesMemory.NewRepository(), requester, nil, nil)

	t.Run("a uuid that names none of the kind's records may name one of its extras, which answers for itself", func(t *testing.T) {
		t.Parallel()

		response, err := useCase.Execute(ctx, &queryResource.Request{Kind: kindstest.Kind, UUID: "shelved", Action: "logs"})
		require.NoError(t, err)

		assert.Empty(t, response.ValidationErrors)
		assert.Nil(t, response.NodeError)
		assert.JSONEq(t, `["the shelf's log of shelved"]`, string(response.Result))
	})

	t.Run("its state is what it says it is doing", func(t *testing.T) {
		t.Parallel()

		response, err := useCase.Execute(ctx, &queryResource.Request{Kind: kindstest.Kind, UUID: "shelved", Action: "state"})
		require.NoError(t, err)

		var observed kind.Observation
		require.NoError(t, json.Unmarshal(response.Result, &observed))

		assert.Equal(t, "shelved", observed.UUID)

		common, err := resource.Common(observed.Status)
		require.NoError(t, err)
		assert.Equal(t, kindstest.Running, common.State)
	})

	t.Run("an extra is whose it says: to somebody else asking for their own, it is not there", func(t *testing.T) {
		t.Parallel()

		_, err := useCase.Execute(ctx, &queryResource.Request{Kind: kindstest.Kind, OwnerUUID: kindstest.OwnerUUID, UUID: "shelved", Action: "logs"})

		assert.ErrorIs(t, err, domain.ErrNotExists)
	})

	assert.Empty(t, requester.Asked(), "no node is asked about an extra")
}

// An extra's query its node refused is answered with the node's refusal, as
// a record's is.
func TestUseCase_Execute_extraRefused(t *testing.T) {
	t.Parallel()

	refused := &noderequest.Error{Code: noderequest.CodeNotFound, Message: "no such fan"}

	shelf := kindstest.NewShelf(kindstest.AShelvedFan("shelved"))
	shelf.QueryFailure = refused

	registry := kind.NewRegistry[kind.ControlPlaneBinding]()
	require.NoError(t, registry.Register(kind.BindControlPlane[kindstest.Spec, kindstest.Status](kindstest.Descriptor(), &kindstest.Shelved{Fans: &kindstest.Fans{}, Shelf: shelf})))

	useCase := queryResource.NewUseCase(registry, resourcesMemory.NewRepository(), &messagingMock.Requester{}, nil, nil)

	response, err := useCase.Execute(context.Background(), &queryResource.Request{Kind: kindstest.Kind, UUID: "shelved", Action: "logs"})
	require.NoError(t, err)

	assert.Equal(t, refused, response.NodeError)
	assert.Empty(t, response.Result)
}
