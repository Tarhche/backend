package runStackAction

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/stack/events"
	messaging "github.com/khanzadimahdi/testproject/infrastructure/messaging/mock"
	mocks "github.com/khanzadimahdi/testproject/infrastructure/repository/mocks/workload/docker"
)

const (
	nodeName    = "workload-orchestrator-01"
	composeYAML = "services:\n  web:\n    image: nginx:alpine\n"
)

type validates struct{}

func (validates) Validate(value any) domain.ValidationErrors {
	return value.(domain.Validatable).Validate()
}

// composers is one Docker VM's dockerd and compose.
type composers struct {
	daemon  *mocks.MockDaemon
	compose *mocks.MockCompose
}

func (c composers) Daemon(string) docker.Daemon {
	return c.daemon
}

func (c composers) Compose(string) docker.Compose {
	return c.compose
}

func request(action stack.Action) Request {
	return Request{StackUUID: "stack-1", VMUUID: "vm-1", Action: action, Project: "shop-abcde", Compose: composeYAML}
}

func TestUseCase_Execute(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name    string
		request Request
		expect  func(daemon *mocks.MockDaemon, compose *mocks.MockCompose)

		wantSubject string
		wantOutput  string
		wantReason  string
	}{
		{
			name:    "up brings the project up once dockerd answers",
			request: request(stack.ActionUp),
			expect: func(d *mocks.MockDaemon, c *mocks.MockCompose) {
				d.On("Ping", mock.Anything).Return(nil)
				c.On("Up", mock.Anything, "shop-abcde", composeYAML).Return("Container shop-abcde-web-1  Started\n", nil)
			},
			wantSubject: events.StackCompletedName,
			wantOutput:  "Container shop-abcde-web-1  Started\n",
		},
		{
			name:    "start",
			request: request(stack.ActionStart),
			expect: func(d *mocks.MockDaemon, c *mocks.MockCompose) {
				d.On("Ping", mock.Anything).Return(nil)
				c.On("Start", mock.Anything, "shop-abcde", composeYAML).Return("", nil)
			},
			wantSubject: events.StackCompletedName,
		},
		{
			name:    "stop",
			request: request(stack.ActionStop),
			expect: func(d *mocks.MockDaemon, c *mocks.MockCompose) {
				d.On("Ping", mock.Anything).Return(nil)
				c.On("Stop", mock.Anything, "shop-abcde", composeYAML).Return("", nil)
			},
			wantSubject: events.StackCompletedName,
		},
		{
			name:    "restart",
			request: request(stack.ActionRestart),
			expect: func(d *mocks.MockDaemon, c *mocks.MockCompose) {
				d.On("Ping", mock.Anything).Return(nil)
				c.On("Restart", mock.Anything, "shop-abcde", composeYAML).Return("", nil)
			},
			wantSubject: events.StackCompletedName,
		},
		{
			name: "down takes the volumes when asked to",
			request: func() Request {
				r := request(stack.ActionDown)
				r.RemoveVolumes = true

				return r
			}(),
			expect: func(d *mocks.MockDaemon, c *mocks.MockCompose) {
				d.On("Ping", mock.Anything).Return(nil)
				c.On("Down", mock.Anything, "shop-abcde", composeYAML, true).Return("", nil)
			},
			wantSubject: events.StackCompletedName,
		},
		{
			name:    "a dockerd that never came up fails the action, and compose is not run",
			request: request(stack.ActionUp),
			expect: func(d *mocks.MockDaemon, _ *mocks.MockCompose) {
				d.On("Ping", mock.Anything).Return(docker.ErrUnavailable)
			},
			wantSubject: events.StackFailedName,
			wantReason:  docker.ErrUnavailable.Error(),
		},
		{
			name:    "compose failing says why, with what compose printed",
			request: request(stack.ActionUp),
			expect: func(d *mocks.MockDaemon, c *mocks.MockCompose) {
				d.On("Ping", mock.Anything).Return(nil)
				c.On("Up", mock.Anything, "shop-abcde", composeYAML).
					Return("service \"web\" refers to undefined network backend\n", errors.New("docker compose up exited with 15"))
			},
			wantSubject: events.StackFailedName,
			wantReason:  "docker compose up exited with 15",
			wantOutput:  "service \"web\" refers to undefined network backend\n",
		},
	}

	for _, tt := range testcases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var daemon mocks.MockDaemon
			var compose mocks.MockCompose
			tt.expect(&daemon, &compose)
			defer daemon.AssertExpectations(t)
			defer compose.AssertExpectations(t)

			producer := &messaging.Recorder{}

			_, err := NewUseCase(composers{daemon: &daemon, compose: &compose}, producer, validates{}, nodeName, time.Minute).
				Execute(t.Context(), &tt.request)
			require.NoError(t, err)

			require.Equal(t, []string{tt.wantSubject}, producer.Subjects())

			if tt.wantSubject == events.StackCompletedName {
				completed, err := messaging.Produced[events.StackCompleted](producer, events.StackCompletedName)
				require.NoError(t, err)

				assert.Equal(t, "stack-1", completed[0].StackUUID)
				assert.Equal(t, nodeName, completed[0].NodeName)
				assert.Equal(t, tt.request.Action, completed[0].Action)
				assert.Equal(t, tt.wantOutput, completed[0].Output)

				return
			}

			failed, err := messaging.Produced[events.StackFailed](producer, events.StackFailedName)
			require.NoError(t, err)

			assert.Equal(t, "stack-1", failed[0].StackUUID)
			assert.Equal(t, tt.request.Action, failed[0].Action)
			assert.Contains(t, failed[0].Reason, tt.wantReason)
			assert.Equal(t, tt.wantOutput, failed[0].Output)
		})
	}

	t.Run("only the last 16 KiB of what compose printed goes back", func(t *testing.T) {
		t.Parallel()

		var daemon mocks.MockDaemon
		var compose mocks.MockCompose
		daemon.On("Ping", mock.Anything).Return(nil)
		compose.On("Up", mock.Anything, mock.Anything, mock.Anything).Return(strings.Repeat("x", stack.MaxOutput)+"the end", nil)

		producer := &messaging.Recorder{}

		_, err := NewUseCase(composers{daemon: &daemon, compose: &compose}, producer, validates{}, nodeName, time.Minute).
			Execute(t.Context(), &Request{StackUUID: "stack-1", VMUUID: "vm-1", Action: stack.ActionUp, Project: "p", Compose: composeYAML})
		require.NoError(t, err)

		completed, err := messaging.Produced[events.StackCompleted](producer, events.StackCompletedName)
		require.NoError(t, err)
		assert.Len(t, completed[0].Output, stack.MaxOutput)
		assert.True(t, strings.HasSuffix(completed[0].Output, "the end"))
	})

	t.Run("saying how it went failing is worth another delivery", func(t *testing.T) {
		t.Parallel()

		var daemon mocks.MockDaemon
		var compose mocks.MockCompose
		daemon.On("Ping", mock.Anything).Return(nil)
		compose.On("Stop", mock.Anything, mock.Anything, mock.Anything).Return("", nil)

		producer := &messaging.Recorder{Err: errors.New("nats is away")}

		_, err := NewUseCase(composers{daemon: &daemon, compose: &compose}, producer, validates{}, nodeName, time.Minute).
			Execute(t.Context(), &Request{StackUUID: "stack-1", VMUUID: "vm-1", Action: stack.ActionStop, Project: "p", Compose: composeYAML})
		assert.Error(t, err)
	})
}

func TestStackRequestedHandler_Handle(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name         string
		payload      string
		ran          bool
		wantSubjects []string
	}{
		{
			name:         "an action on this node's VM is run here",
			payload:      `{"stack_uuid":"stack-1","vm_uuid":"vm-1","node_name":"` + nodeName + `","action":"stop","project":"shop-abcde","compose":"services: {}"}`,
			ran:          true,
			wantSubjects: []string{events.StackCompletedName},
		},
		{
			name:    "one on another node's VM is not this node's",
			payload: `{"stack_uuid":"stack-1","vm_uuid":"vm-1","node_name":"workload-orchestrator-02","action":"stop","project":"shop-abcde","compose":"services: {}"}`,
		},
		{
			name:         "one this node refuses is said to have failed",
			payload:      `{"stack_uuid":"stack-1","vm_uuid":"vm-1","node_name":"` + nodeName + `","action":"scale","project":"shop-abcde","compose":"services: {}"}`,
			wantSubjects: []string{events.StackFailedName},
		},
		{
			name:    "a message that cannot be read is dropped",
			payload: `{`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var daemon mocks.MockDaemon
			var compose mocks.MockCompose

			if tt.ran {
				daemon.On("Ping", mock.Anything).Return(nil)
				compose.On("Stop", mock.Anything, "shop-abcde", "services: {}").Return("", nil)
			}
			defer daemon.AssertExpectations(t)
			defer compose.AssertExpectations(t)

			producer := &messaging.Recorder{}
			useCase := NewUseCase(composers{daemon: &daemon, compose: &compose}, producer, validates{}, nodeName, time.Minute)

			require.NoError(t, NewStackRequestedHandler(useCase, nodeName, slog.New(slog.DiscardHandler)).Handle(t.Context(), []byte(tt.payload)))
			assert.Equal(t, tt.wantSubjects, producer.Subjects())
		})
	}
}
