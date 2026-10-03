package vm

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
)

func TestIsID(t *testing.T) {
	t.Parallel()

	assert.True(t, IsID("0123456789abcdef"))

	for _, invalid := range []string{"", "0123456789ABCDEF", "0123456789abcde", "0123456789abcdef0", "../../etc/passwd0"} {
		assert.False(t, IsID(invalid), invalid)
	}
}

func TestIsNetworkName(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{PublicNetwork, "workload-isolated", "workload-stack-my-stack-xkfqz"} {
		assert.True(t, IsNetworkName(valid), valid)
	}

	for _, invalid := range []string{"", "-public", "Public", "a/b", "workload_isolated"} {
		assert.False(t, IsNetworkName(invalid), invalid)
	}
}

func TestState(t *testing.T) {
	t.Parallel()

	for _, ended := range []State{StateExited, StateDead, StateRemoving} {
		assert.True(t, ended.Ended(), ended)
		assert.False(t, ended.Up(), ended)
	}

	for _, up := range []State{StateRunning, StateRestarting} {
		assert.True(t, up.Up(), up)
		assert.False(t, up.Ended(), up)
	}

	assert.False(t, StateCreated.Up())
	assert.False(t, StateCreated.Ended())
}

func TestVM_Endpoints(t *testing.T) {
	t.Parallel()

	networked := []Interface{{Network: "workload-isolated", Address: "10.250.0.2/24"}}

	t.Run("a running VM on a network is reached on every port it exposes", func(t *testing.T) {
		t.Parallel()

		v := VM{State: StateRunning, Interfaces: networked, Spec: Spec{ExposedPorts: []uint16{8080, 80, 8080}}}

		assert.Equal(t, []uint16{80, 8080}, v.Endpoints())
	})

	t.Run("a VM that is not running is reached on none", func(t *testing.T) {
		t.Parallel()

		v := VM{State: StateExited, Interfaces: networked, Spec: Spec{ExposedPorts: []uint16{80}}}

		assert.Empty(t, v.Endpoints())
	})

	t.Run("a VM on no network is reached on none", func(t *testing.T) {
		t.Parallel()

		v := VM{State: StateRunning, Spec: Spec{ExposedPorts: []uint16{80}}}

		assert.Empty(t, v.Endpoints())
	})
}

func TestVM_Matches(t *testing.T) {
	t.Parallel()

	v := VM{Spec: Spec{Labels: map[string]string{"node.name": "workload-orchestrator-01", "task.uuid": "u-1"}}}

	assert.True(t, v.Matches(nil))
	assert.True(t, v.Matches([]string{"node.name=workload-orchestrator-01"}))
	assert.True(t, v.Matches([]string{"node.name=workload-orchestrator-01", "task.uuid=u-1"}))
	assert.False(t, v.Matches([]string{"node.name=workload-orchestrator-02"}))
	assert.False(t, v.Matches([]string{"task.slug="}))
}

func TestPaths(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "/v1/vms/0123456789abcdef", PathVM("0123456789abcdef"))
	assert.Equal(t, "/v1/vms/0123456789abcdef/stop", PathVMAction("0123456789abcdef", "stop"))
	assert.Equal(t, "/v1/vms/0123456789abcdef/exec/e1/end", PathEndExec("0123456789abcdef", "e1"))
	assert.Equal(t, "/v1/networks/workload-stack-a", PathNetwork("workload-stack-a"))
	assert.Equal(t, "/v1/images/sha256:abc", PathImage("sha256:abc"))
}

func TestErrors(t *testing.T) {
	t.Parallel()

	t.Run("an error reads back as the error it was", func(t *testing.T) {
		t.Parallel()

		for _, sent := range []error{ErrNotFound, ErrInvalid, ErrConflict, ErrCapacity, ErrNotRunning, ErrNetworkInUse, ErrImage, ErrUnavailable} {
			status, answer := Describe(fmt.Errorf("making 0123456789abcdef: %w", sent))

			assert.GreaterOrEqual(t, status, http.StatusBadRequest)

			encoded, err := json.Marshal(answer)
			require.NoError(t, err)

			var received ErrorResponse
			require.NoError(t, json.Unmarshal(encoded, &received))

			assert.ErrorIs(t, received.Err(), sent)
		}
	})

	t.Run("a VM that is not there is not there in the domain's words too", func(t *testing.T) {
		t.Parallel()

		_, answer := Describe(ErrNotFound)

		assert.ErrorIs(t, answer.Err(), domain.ErrNotExists)
	})

	t.Run("capacity is a conflict, which another node may not have", func(t *testing.T) {
		t.Parallel()

		status, answer := Describe(ErrCapacity)

		assert.Equal(t, http.StatusConflict, status)
		assert.Equal(t, CodeCapacity, answer.Code)
	})

	t.Run("anything else is vmhost's own failure", func(t *testing.T) {
		t.Parallel()

		status, answer := Describe(errors.New("disk full"))

		assert.Equal(t, http.StatusInternalServerError, status)
		assert.Equal(t, CodeInternal, answer.Code)
		assert.EqualError(t, answer.Err(), "vmhost failed: disk full")
	})
}
