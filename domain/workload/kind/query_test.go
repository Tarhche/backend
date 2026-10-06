package kind

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

func TestOp(t *testing.T) {
	t.Parallel()

	assert.Equal(t, noderequest.Op("stack.state"), Op("stack", "state"))
	assert.Equal(t, noderequest.Op("vm.logs"), Op("vm", "logs"), "a VM's log is asked as it always was")
}

func TestParseOp(t *testing.T) {
	t.Parallel()

	for op, want := range map[noderequest.Op]struct {
		kind   string
		action string
		ok     bool
	}{
		"stack.state":            {kind: "stack", action: "state", ok: true},
		"vm.logs":                {kind: "vm", action: "logs", ok: true},
		"docker.containers.list": {},
		"stack":                  {},
		"stack.":                 {},
		".state":                 {},
		"":                       {},
	} {
		t.Run(string(op), func(t *testing.T) {
			t.Parallel()

			kindName, action, ok := ParseOp(op)

			assert.Equal(t, want.ok, ok, "ok")
			assert.Equal(t, want.kind, kindName, "kind")
			assert.Equal(t, want.action, action, "action")
		})
	}
}

func TestQuery(t *testing.T) {
	t.Parallel()

	raw, err := Encode(aBox())
	require.NoError(t, err)

	asked := Query{
		Kind:     "box",
		UUID:     "box-uuid",
		Action:   "logs",
		Payload:  json.RawMessage(`{"tail":20}`),
		Resource: raw,
	}

	t.Run("a query travels as a node request and arrives as it left", func(t *testing.T) {
		t.Parallel()

		request, err := asked.Request()
		require.NoError(t, err)

		arrived, err := QueryOf(travel(t, request))
		require.NoError(t, err)

		assert.Equal(t, asked.Kind, arrived.Kind)
		assert.Equal(t, asked.UUID, arrived.UUID)
		assert.Equal(t, asked.Action, arrived.Action)
		assert.JSONEq(t, string(asked.Payload), string(arrived.Payload))

		typed, err := Decode[boxSpec, boxStatus](arrived.Resource)
		require.NoError(t, err)
		assert.Equal(t, aBox(), typed)
	})

	t.Run("its op names the kind and the action, and its vm_uuid the resource", func(t *testing.T) {
		t.Parallel()

		request, err := asked.Request()
		require.NoError(t, err)

		assert.Equal(t, noderequest.Op("box.logs"), request.Op)
		assert.Equal(t, "box-uuid", request.VMUUID)

		named := fields(t, request)
		assert.JSONEq(t, `"box.logs"`, string(named["op"]))
		assert.JSONEq(t, `"box-uuid"`, string(named["vm_uuid"]))

		var payload map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(named["payload"], &payload))
		assert.JSONEq(t, `{"tail":20}`, string(payload["payload"]), "the action's own payload, as it was given")
		assert.Contains(t, payload, "resource")
	})

	t.Run("a request that is not a kind's is not a query", func(t *testing.T) {
		t.Parallel()

		_, err := QueryOf(noderequest.Request{Op: "docker.containers.list", VMUUID: "vm-uuid"})

		assert.ErrorIs(t, err, ErrUnknownAction)
	})

	t.Run("nor is one whose payload is not a query's", func(t *testing.T) {
		t.Parallel()

		_, err := QueryOf(noderequest.Request{Op: "box.logs", VMUUID: "box-uuid", Payload: json.RawMessage(`[1]`)})

		assert.ErrorIs(t, err, ErrInvalidPayload)
	})
}
