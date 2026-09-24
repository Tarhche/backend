package task

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	attachtask "github.com/khanzadimahdi/testproject/application/runner/orchestrator/task/attachTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/runner/runtime"
	"github.com/khanzadimahdi/testproject/infrastructure/validator"
)

// session stands for a command that writes what it was given and then ends.
type session struct {
	output io.Reader

	lock  sync.Mutex
	ended bool
}

var _ task.ExecSession = &session{}

func (s *session) Read(p []byte) (int, error)                             { return s.output.Read(p) }
func (s *session) Write(p []byte) (int, error)                            { return len(p), nil }
func (s *session) Resize(ctx context.Context, rows uint, cols uint) error { return nil }
func (s *session) Close() error                                           { return nil }

func (s *session) End(ctx context.Context) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.ended = true

	return nil
}

func (s *session) wasEnded() bool {
	s.lock.Lock()
	defer s.lock.Unlock()

	return s.ended
}

func TestAttachHandler(t *testing.T) {
	t.Parallel()

	t.Run("a command that ends closes its terminal, after everything it wrote", func(t *testing.T) {
		t.Parallel()

		command := &session{output: strings.NewReader("goodbye\n")}

		var taskManager runtime.MockRuntime
		taskManager.On("Of", mock.Anything, "task-uuid").
			Return([]task.Execution{{ID: "execution-id", Status: task.StatusRunning}}, nil)
		taskManager.On("Exec", mock.Anything, "execution-id", mock.Anything).Return(command, nil)

		accepts := &validator.MockValidator{}
		accepts.On("Validate", mock.Anything).Return(domain.ValidationErrors{})

		mux := http.NewServeMux()
		mux.Handle("GET /api/tasks/{uuid}/attach", NewAttachHandler(attachtask.NewUseCase(&taskManager, accepts), slog.New(slog.DiscardHandler)))

		server := httptest.NewServer(mux)
		defer server.Close()

		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/api/tasks/task-uuid/attach", nil)
		require.NoError(t, err)
		defer conn.Close()

		// nothing is typed: the terminal ends because the command did.
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))

		kind, payload, err := conn.ReadMessage()
		require.NoError(t, err)
		assert.Equal(t, websocket.BinaryMessage, kind)
		assert.Equal(t, "goodbye\n", string(payload))

		_, _, err = conn.ReadMessage()
		assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "closed as a terminal that is over, not left open: %v", err)

		// and what it left running is ended, as for a client that walked away.
		assert.Eventually(t, command.wasEnded, 5*time.Second, 10*time.Millisecond)
	})
}
