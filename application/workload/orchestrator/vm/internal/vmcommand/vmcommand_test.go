package vmcommand

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/vm/events"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

func TestSpec(t *testing.T) {
	t.Parallel()

	sent := vm.Spec{
		ID:    "whatever was sent",
		Kind:  vm.KindDocker,
		Image: "docker:29-dind",
		Ports: []port.Port{80},
		Labels: map[string]string{
			vm.LabelOwner: "owner-uuid",
			vm.LabelSlug:  "box-abcde",
		},
	}

	given := Spec("vm-1", sent)

	assert.Equal(t, "vm-1", given.ID, "a VM is named by its uuid")
	assert.Equal(t, map[string]string{
		vm.LabelOwner:   "owner-uuid",
		vm.LabelSlug:    "box-abcde",
		vm.LabelVM:      "vm-1",
		vm.LabelPurpose: vm.PurposeVM,
	}, given.Labels)

	assert.Equal(t, map[string]string{vm.LabelOwner: "owner-uuid", vm.LabelSlug: "box-abcde"}, sent.Labels, "what was sent is left as it was")

	unlabelled := Spec("vm-2", vm.Spec{})
	assert.Equal(t, map[string]string{vm.LabelVM: "vm-2", vm.LabelPurpose: vm.PurposeVM}, unlabelled.Labels)
}

func TestFailedAndRefused(t *testing.T) {
	t.Parallel()

	t.Run("a failure is said against its VM, in the engine's words", func(t *testing.T) {
		t.Parallel()

		producer := &messaging.Recorder{}
		require.NoError(t, Failed(t.Context(), producer, "workload-orchestrator-01", "vm-1", errors.New("no capacity")))

		failed, err := messaging.Produced[events.VMFailed](producer, events.VMFailedName)
		require.NoError(t, err)
		require.Len(t, failed, 1)

		assert.Equal(t, "vm-1", failed[0].VMUUID)
		assert.Equal(t, "workload-orchestrator-01", failed[0].NodeName)
		assert.Equal(t, "no capacity", failed[0].Reason)
		assert.False(t, failed[0].At.IsZero())
	})

	t.Run("a refusal says every field that was wrong", func(t *testing.T) {
		t.Parallel()

		producer := &messaging.Recorder{}
		require.NoError(t, Refused(t.Context(), producer, "workload-orchestrator-01", "vm-1", domain.ValidationErrors{
			"spec.kind":  "invalid_value",
			"spec.image": "required_field",
		}))

		failed, err := messaging.Produced[events.VMFailed](producer, events.VMFailedName)
		require.NoError(t, err)
		assert.Equal(t, "the command was refused: spec.image: required_field; spec.kind: invalid_value", failed[0].Reason)
	})

	t.Run("nothing is said when there is nothing to say, or nobody to say it about", func(t *testing.T) {
		t.Parallel()

		producer := &messaging.Recorder{}
		require.NoError(t, Refused(t.Context(), producer, "workload-orchestrator-01", "vm-1", nil))
		require.NoError(t, Refused(t.Context(), producer, "workload-orchestrator-01", "", domain.ValidationErrors{"vm_uuid": "required_field"}))

		assert.Empty(t, producer.Messages())
	})
}
