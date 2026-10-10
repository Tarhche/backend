package noderequest

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestSubject(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "workloadNodeRequest.workload-orchestrator-01", Subject("workload-orchestrator-01"))
	assert.NotEqual(t, Subject("a"), Subject("b"), "every node is asked on a subject of its own")
}

func TestReply_Err(t *testing.T) {
	t.Parallel()

	t.Run("an answered request failed at nothing", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, Reply{OK: true, Result: json.RawMessage(`[]`)}.Err())
	})

	t.Run("a refusal is the node's own error", func(t *testing.T) {
		t.Parallel()

		err := Reply{Error: &Error{Code: CodeNotRunning, Message: "the vm is not running"}}.Err()

		require.Error(t, err)
		assert.ErrorIs(t, err, vm.ErrNotRunning)
	})

	t.Run("a refusal that does not say why is still a refusal", func(t *testing.T) {
		t.Parallel()

		err := Reply{}.Err()

		var replied *Error
		require.True(t, errors.As(err, &replied))
		assert.Equal(t, CodeInternal, replied.Code)
	})
}

func TestRequest(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(Request{Op: "vm.logs", VMUUID: "vm-uuid", Payload: json.RawMessage(`{"tail":20}`)})
	require.NoError(t, err)

	// the payload travels as it was given, inside the request rather than as a
	// string of it.
	assert.JSONEq(t, `{"op":"vm.logs","vm_uuid":"vm-uuid","payload":{"tail":20}}`, string(encoded))
}
