package answerQuery

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// before is what answered node requests before there were kinds: it answers
// everything it is handed, and remembers what that was.
type before struct {
	handed []noderequest.Op
}

var _ noderequest.Handler = &before{}

func (b *before) Handle(_ context.Context, request noderequest.Request) noderequest.Reply {
	b.handed = append(b.handed, request.Op)

	return noderequest.Reply{OK: true, Result: json.RawMessage(`"answered as it always was"`)}
}

// asking is a lamp's query, as the node request it travels as.
func asking(t *testing.T, action string, payload string) noderequest.Request {
	t.Helper()

	request, err := kind.Query{Kind: "lamp", UUID: "lamp-1", Action: action, Payload: json.RawMessage(payload), Resource: aLamp(t)}.Request()
	require.NoError(t, err)

	return request
}

func TestUseCase_Handle(t *testing.T) {
	t.Parallel()

	litLamp := kind.Observed[lampStatus]{
		UUID:   "lamp-1",
		Owners: []kind.Reference{{Kind: "room", UUID: "room-1"}},
		Status: lampStatus{Status: kind.Status{State: lit}, Brightness: 80},
	}

	for name, tt := range map[string]struct {
		strategy *lamps
		request  func(t *testing.T) noderequest.Request

		result string
		code   noderequest.Code
		says   string
		asked  []string
	}{
		"a kind's query is its strategy's to answer, about the resource it carries": {
			strategy: &lamps{},
			request:  func(t *testing.T) noderequest.Request { return asking(t, "readings", `{"last": 2}`) },
			result:   `[3,2]`,
			asked:    []string{"readings lamp-1 40 watts {2}"},
		},
		"its state is read off everything the node holds of the kind": {
			strategy: &lamps{report: kind.Report[lampStatus]{Instances: []kind.Observed[lampStatus]{litLamp}}},
			request:  func(t *testing.T) noderequest.Request { return asking(t, "state", ``) },
			result:   `{"kind":"lamp","uuid":"lamp-1","owners":[{"kind":"room","uuid":"room-1"}],"status":{"state":"lit","brightness":80}}`,
		},
		"one the node does not hold is observed missing": {
			strategy: &lamps{},
			request:  func(t *testing.T) noderequest.Request { return asking(t, "state", ``) },
			result:   `{"kind":"lamp","uuid":"lamp-1","status":{"state":"missing"}}`,
		},
		"one inside a parent that did not answer is not known either way": {
			strategy: &lamps{report: kind.Report[lampStatus]{Unseen: []string{"room-1"}}},
			request:  func(t *testing.T) noderequest.Request { return asking(t, "state", ``) },
			code:     noderequest.CodeInternal,
			says:     "could not be seen",
		},
		"what the strategy does not find is not found": {
			strategy: &lamps{failure: domain.ErrNotExists},
			request:  func(t *testing.T) noderequest.Request { return asking(t, "readings", `{"last": 1}`) },
			code:     noderequest.CodeNotFound,
			asked:    []string{"readings lamp-1 40 watts {1}"},
		},
		"and what it could not answer says why": {
			strategy: &lamps{failure: errors.New("the meter is unplugged")},
			request:  func(t *testing.T) noderequest.Request { return asking(t, "readings", `{"last": 1}`) },
			code:     noderequest.CodeInternal,
			says:     "the meter is unplugged",
			asked:    []string{"readings lamp-1 40 watts {1}"},
		},
		"a payload that is not valid is refused, with why": {
			strategy: &lamps{},
			request:  func(t *testing.T) noderequest.Request { return asking(t, "readings", `{"last": 500}`) },
			code:     noderequest.CodeInvalid,
			says:     "last: out_of_range",
		},
		"an action the kind does not have is refused": {
			strategy: &lamps{},
			request:  func(t *testing.T) noderequest.Request { return asking(t, "explode", ``) },
			code:     noderequest.CodeInvalid,
			says:     "unknown action",
		},
		"and so is a command asked as a query": {
			strategy: &lamps{},
			request:  func(t *testing.T) noderequest.Request { return asking(t, "light", ``) },
			code:     noderequest.CodeInvalid,
			says:     "unknown action",
		},
		"a query that cannot be read is refused": {
			strategy: &lamps{},
			request: func(*testing.T) noderequest.Request {
				return noderequest.Request{Op: "lamp.readings", VMUUID: "lamp-1", Payload: json.RawMessage(`"nothing like a query"`)}
			},
			code: noderequest.CodeInvalid,
			says: "invalid payload",
		},
		"and so is one about a resource it does not carry": {
			strategy: &lamps{},
			request: func(t *testing.T) noderequest.Request {
				request := asking(t, "readings", `{"last": 1}`)
				request.VMUUID = "lamp-2"

				return request
			},
			code: noderequest.CodeInvalid,
			says: `carries "lamp-1"`,
		},
		"and so is one about nothing in particular": {
			strategy: &lamps{},
			request: func(t *testing.T) noderequest.Request {
				request := asking(t, "readings", `{"last": 1}`)
				request.VMUUID = ""

				return request
			},
			code: noderequest.CodeInvalid,
			says: "vm_uuid is required",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			next := &before{}

			answered := NewUseCase(running(t, tt.strategy), next).Handle(t.Context(), tt.request(t))

			assert.Empty(t, next.handed, "a kind's query is the kind's")
			assert.Equal(t, tt.asked, tt.strategy.asked, "asked")

			if len(tt.code) > 0 {
				assert.False(t, answered.OK)
				require.NotNil(t, answered.Error)
				assert.Equal(t, tt.code, answered.Error.Code)
				assert.Contains(t, answered.Error.Message, tt.says)

				return
			}

			require.True(t, answered.OK, "%v", answered.Error)
			assert.JSONEq(t, tt.result, string(answered.Result))
		})
	}

	t.Run("an error that crossed into the domain is the domain's again on the other side", func(t *testing.T) {
		t.Parallel()

		answered := NewUseCase(running(t, &lamps{failure: domain.ErrNotExists}), &before{}).Handle(t.Context(), asking(t, "readings", `{"last": 1}`))

		assert.ErrorIs(t, answered.Err(), domain.ErrNotExists)
	})
}

func TestUseCase_Handle_TheNodesOwn(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct {
		kinds func(t *testing.T) *kind.Registry[kind.NodeBinding]
		op    noderequest.Op
	}{
		"an op named after a kind that is not registered here is the node's, as it always was": {
			kinds: func(t *testing.T) *kind.Registry[kind.NodeBinding] { return running(t, &lamps{}) },
			op:    noderequest.OpVMLogs,
		},
		"and so is one that names no kind's action": {
			kinds: func(t *testing.T) *kind.Registry[kind.NodeBinding] { return running(t, &lamps{}) },
			op:    noderequest.OpContainersList,
		},
		"and with no kinds at all, every request is": {
			kinds: func(*testing.T) *kind.Registry[kind.NodeBinding] { return kind.NewRegistry[kind.NodeBinding]() },
			op:    "lamp.state",
		},
		"even with no registry": {
			kinds: func(*testing.T) *kind.Registry[kind.NodeBinding] { return nil },
			op:    "lamp.state",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			next := &before{}

			answered := NewUseCase(tt.kinds(t), next).Handle(t.Context(), noderequest.Request{Op: tt.op, VMUUID: "vm-1"})

			assert.Equal(t, []noderequest.Op{tt.op}, next.handed)
			assert.True(t, answered.OK)
			assert.JSONEq(t, `"answered as it always was"`, string(answered.Result))
		})
	}
}
