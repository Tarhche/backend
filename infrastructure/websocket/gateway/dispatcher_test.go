package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/khanzadimahdi/testproject/domain"
	messagingMock "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestInjectRequestID(t *testing.T) {
	t.Parallel()

	t.Run("replaces the client's id with the server-side one", func(t *testing.T) {
		t.Parallel()

		payload, err := injectRequestID([]byte(`{"id":"client-1","code":"print(1)"}`), "server-1")
		require.NoError(t, err)

		var decoded map[string]any
		require.NoError(t, json.Unmarshal(payload, &decoded))

		assert.Equal(t, "server-1", decoded["id"])
		assert.Equal(t, "print(1)", decoded["code"])
	})

	t.Run("carries an id into a payload that is a bare null", func(t *testing.T) {
		t.Parallel()

		payload, err := injectRequestID([]byte("null"), "server-1")
		require.NoError(t, err)

		assert.JSONEq(t, `{"id":"server-1"}`, string(payload))
	})

	t.Run("reports a payload that is not an object", func(t *testing.T) {
		t.Parallel()

		for _, payload := range []string{`"a string"`, `[1,2,3]`, `42`, `not json`, ``} {
			_, err := injectRequestID([]byte(payload), "server-1")

			assert.Error(t, err, "payload %q", payload)
		}
	})
}

func TestDispatcher(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("dispatches a request whose payload is a bare null", func(t *testing.T) {
		t.Parallel()

		// a []byte field is read from base64, and bnVsbA== decodes to null.
		var request domain.Request
		require.NoError(t, json.Unmarshal([]byte(`{"id":"1","subject":"runCode","payload":"bnVsbA=="}`), &request))
		require.Equal(t, []byte("null"), request.Payload)

		var producerMock messagingMock.MockProduceConsumer
		producerMock.On("Produce", mock.Anything, "websocket_runCode", mock.Anything).Return(nil).Once()
		defer producerMock.AssertExpectations(t)

		subjects := newSubjects()
		subjects.add("runCode")

		registry := NewInMemoryRequestRegistry(8)

		d := &dispatcher{
			validator:     newRequestValidator(registry, subjects, echoTranslator(), defaultMaxInFlightRequests),
			registry:      registry,
			producer:      &producerMock,
			subjectPrefix: defaultSubjectPrefix,
			logger:        logger,
		}

		serverSideID, validationErrors, err := d.dispatch(context.Background(), &request)

		require.NoError(t, err)
		assert.Empty(t, validationErrors)
		assert.NotEmpty(t, serverSideID)
	})
}
