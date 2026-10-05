package getVMLogs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/dashboard/workload/workloadtest"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/controlplane"
)

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	since := workloadtest.At.Add(time.Minute)

	for _, scope := range workloadtest.Scopes {
		t.Run(scope.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("VMLogs", mock.Anything, scope.OwnerUUID, "vm-uuid", vm.LogOptions{Since: since, Tail: 500}).Once().Return([]vm.LogLine{
				{At: since, Source: "kernel", Line: "booted"},
				{At: since.Add(time.Second), Source: "runtime", Line: "agent up"},
			}, nil)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{
				UUID:      "vm-uuid",
				OwnerUUID: scope.OwnerUUID,
				Since:     since,
				Tail:      500,
			})
			require.NoError(t, err)

			presented, err := json.Marshal(response)
			require.NoError(t, err)

			assert.JSONEq(t, `{
				"items": [
					{"at": "2026-10-04T12:01:00Z", "source": "kernel", "line": "booted"},
					{"at": "2026-10-04T12:01:01Z", "source": "runtime", "line": "agent up"}
				],
				"truncated": false
			}`, string(presented))
		})
	}

	t.Run("nothing written yet is no lines rather than none at all", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("VMLogs", mock.Anything, "", "vm-uuid", vm.LogOptions{}).Once().Return(nil, nil)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{UUID: "vm-uuid"})
		require.NoError(t, err)

		presented, err := json.Marshal(response)
		require.NoError(t, err)

		assert.JSONEq(t, `{"items": [], "truncated": false}`, string(presented))
	})

	t.Run("as many lines as an answer carries may be the end of more", func(t *testing.T) {
		t.Parallel()

		var workload controlplane.MockClient
		workload.On("VMLogs", mock.Anything, "", "vm-uuid", vm.LogOptions{}).Once().Return(make([]vm.LogLine, noderequest.MaxLogLines), nil)
		defer workload.AssertExpectations(t)

		response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{UUID: "vm-uuid"})
		require.NoError(t, err)

		assert.True(t, response.Truncated)
	})

	for _, answer := range workloadtest.Answers() {
		t.Run(answer.Name, func(t *testing.T) {
			t.Parallel()

			var workload controlplane.MockClient
			workload.On("VMLogs", mock.Anything, workloadtest.OwnerUUID, "vm-uuid", vm.LogOptions{}).Once().Return(nil, answer.Err)
			defer workload.AssertExpectations(t)

			response, err := NewUseCase(&workload, workloadtest.Translator()).Execute(context.Background(), &Request{
				UUID:      "vm-uuid",
				OwnerUUID: workloadtest.OwnerUUID,
			})

			if answer.Failure != nil {
				assert.ErrorIs(t, err, answer.Failure)
				assert.Nil(t, response)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, answer.Refused, response.ValidationErrors)
		})
	}
}
