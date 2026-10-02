package runCode

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/task/events"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

func TestRunCode_Runtime(t *testing.T) {
	t.Parallel()

	asked := func(t *testing.T, class runtime.Class) []byte {
		t.Helper()

		accepts := &validator.MockValidator{}
		accepts.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

		var (
			producer messagingMock.MockProduceConsumer
			replyer  messagingMock.MockReplyer
			payload  []byte
		)

		producer.On("Produce", mock.Anything, events.TaskRunRequestedName, mock.Anything).
			Run(func(args mock.Arguments) { payload = args.Get(2).([]byte) }).
			Return(nil).Once()
		defer producer.AssertExpectations(t)

		handler := NewRunCodeHandler(accepts, &producer, &replyer, class, slog.New(slog.NewTextHandler(io.Discard, nil)))

		require.NoError(t, handler.Handle(context.Background(), []byte(`{"id": "request-id", "code": "package main", "runner": "go-1.24"}`)))

		return payload
	}

	t.Run("a snippet is run with the class the platform runs snippets with", func(t *testing.T) {
		t.Parallel()

		var requested events.TaskRunRequested
		require.NoError(t, json.Unmarshal(asked(t, runtime.Firecracker), &requested))

		assert.Equal(t, runtime.Firecracker, requested.Runtime)
		assert.Equal(t, "request-id", requested.Name)
	})

	t.Run("with none configured, the workload's default decides", func(t *testing.T) {
		t.Parallel()

		payload := asked(t, "")

		var raw map[string]any
		require.NoError(t, json.Unmarshal(payload, &raw))
		assert.NotContains(t, raw, "runtime")
	})
}
