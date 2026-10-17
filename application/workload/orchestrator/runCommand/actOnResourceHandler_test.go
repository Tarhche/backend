package runCommand

import (
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/lock"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
)

func TestActOnResourceHandler_Handle(t *testing.T) {
	t.Parallel()

	handling := func(t *testing.T) (*ActOnResourceHandler, *[]string, *messaging.Recorder) {
		t.Helper()

		var executed []string
		recorder := &messaging.Recorder{}

		useCase := NewUseCase(running(t, lights(&executed)), lock.New(), recorder)

		return NewActOnResourceHandler(useCase, "node-1", slog.New(slog.DiscardHandler)), &executed, recorder
	}

	t.Run("a command addressed here is carried out", func(t *testing.T) {
		t.Parallel()

		handler, executed, recorder := handling(t)

		message, err := json.Marshal(aCommand(t, "lamp-1", "light", `{"brightness": 80}`))
		require.NoError(t, err)

		require.NoError(t, handler.Handle(t.Context(), message))

		assert.Equal(t, []string{"light lamp-1 {80}"}, *executed)

		said := results(t, recorder)
		require.Len(t, said, 1)
		assert.True(t, said[0].OK)
	})

	t.Run("one addressed to another node is left to it", func(t *testing.T) {
		t.Parallel()

		handler, executed, recorder := handling(t)

		command := aCommand(t, "lamp-1", "light", `{"brightness": 80}`)
		command.Node = "node-2"

		message, err := json.Marshal(command)
		require.NoError(t, err)

		require.NoError(t, handler.Handle(t.Context(), message))

		assert.Empty(t, *executed)
		assert.Empty(t, recorder.Messages(), "it is not this node's to answer")
	})

	t.Run("an unreadable one is let go of rather than read again", func(t *testing.T) {
		t.Parallel()

		handler, executed, recorder := handling(t)

		require.NoError(t, handler.Handle(t.Context(), []byte(`{"kind": `)))

		assert.Empty(t, *executed)
		assert.Empty(t, recorder.Messages())
	})
}
