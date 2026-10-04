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

func TestOp(t *testing.T) {
	t.Parallel()

	for op, want := range map[Op]struct {
		valid   bool
		docker  bool
		mayPull bool
	}{
		OpVMLogs:                     {valid: true},
		OpPing:                       {valid: true, docker: true},
		OpContainersList:             {valid: true, docker: true},
		OpContainersInspect:          {valid: true, docker: true},
		OpContainersCreate:           {valid: true, docker: true, mayPull: true},
		OpContainersStart:            {valid: true, docker: true},
		OpContainersStop:             {valid: true, docker: true},
		OpContainersRestart:          {valid: true, docker: true},
		OpContainersRemove:           {valid: true, docker: true},
		OpContainersLogs:             {valid: true, docker: true},
		OpContainersStats:            {valid: true, docker: true},
		OpContainersConnect:          {valid: true, docker: true},
		OpContainersDisconnect:       {valid: true, docker: true},
		OpImagesList:                 {valid: true, docker: true},
		OpImagesPull:                 {valid: true, docker: true, mayPull: true},
		OpImagesRemove:               {valid: true, docker: true},
		OpNetworksList:               {valid: true, docker: true},
		OpNetworksCreate:             {valid: true, docker: true},
		OpNetworksRemove:             {valid: true, docker: true},
		OpVolumesList:                {valid: true, docker: true},
		OpVolumesCreate:              {valid: true, docker: true},
		OpVolumesRemove:              {valid: true, docker: true},
		Op(""):                       {},
		Op("docker.containers.exec"): {},
		Op("vm.delete"):              {},
	} {
		t.Run(string(op), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, want.valid, op.IsValid(), "valid")
			assert.Equal(t, want.docker, op.IsDocker(), "docker")
			assert.Equal(t, want.mayPull, op.MayPull(), "may pull")
		})
	}
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

	payload, err := json.Marshal(NewContainersRequest(dockerFilter()))
	require.NoError(t, err)

	encoded, err := json.Marshal(Request{Op: OpContainersList, VMUUID: "vm-uuid", Payload: payload})
	require.NoError(t, err)

	// the payload travels as it was given, inside the request rather than as a
	// string of it.
	assert.JSONEq(t, `{"op":"docker.containers.list","vm_uuid":"vm-uuid","payload":{"all":true,"stack":"shop-abcde"}}`, string(encoded))
}
