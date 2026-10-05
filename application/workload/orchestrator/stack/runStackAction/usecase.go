// Package runStackAction runs a compose command on a stack in one of this
// node's Docker VMs.
package runStackAction

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/stack/events"
)

// Composers are the dockerds of this node's Docker VMs and docker compose in
// them.
type Composers interface {
	Daemon(vmUUID string) docker.Daemon
	Compose(vmUUID string) docker.Compose
}

// UseCase runs stack actions.
//
// The VM's dockerd is waited for first, since a stack deployed into a Docker
// VM that is still coming up would otherwise fail for being early. Then
// compose runs the action, and what it printed goes back with whether it
// worked: the end of compose's output is what says why a service did not come
// up, so a failure carries it as much as a success does. Either way the
// result is said rather than returned, because an action compose refused
// once it refuses again.
type UseCase struct {
	composers Composers
	producer  domain.Producer
	validator domain.Validator
	nodeName  string

	// timeout bounds waiting for dockerd and running the action, which may
	// have images to pull.
	timeout time.Duration
}

func NewUseCase(composers Composers, producer domain.Producer, validator domain.Validator, nodeName string, timeout time.Duration) *UseCase {
	return &UseCase{composers: composers, producer: producer, validator: validator, nodeName: nodeName, timeout: timeout}
}

// Execute runs the action and says how it went. The error it returns is only
// ever a failure to say so, which is worth asking again for.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	running, cancel := context.WithTimeout(ctx, uc.timeout)
	defer cancel()

	output, err := uc.run(running, request)
	output = lastOf(output, stack.MaxOutput)

	if err != nil {
		// a node going away has not failed the action: whoever takes the
		// command next runs it.
		if ctx.Err() != nil {
			return nil, err
		}

		return &Response{}, uc.failed(ctx, request.StackUUID, request.Action, err, output)
	}

	return &Response{}, uc.publish(ctx, events.StackCompletedName, events.StackCompleted{
		StackUUID: request.StackUUID,
		NodeName:  uc.nodeName,
		Action:    request.Action,
		Output:    output,
		At:        time.Now(),
	})
}

func (uc *UseCase) run(ctx context.Context, request *Request) (string, error) {
	if err := uc.composers.Daemon(request.VMUUID).Ping(ctx); err != nil {
		return "", err
	}

	compose := uc.composers.Compose(request.VMUUID)

	switch request.Action {
	case stack.ActionUp:
		return compose.Up(ctx, request.Project, request.Compose)
	case stack.ActionStart:
		return compose.Start(ctx, request.Project, request.Compose)
	case stack.ActionStop:
		return compose.Stop(ctx, request.Project, request.Compose)
	case stack.ActionRestart:
		return compose.Restart(ctx, request.Project, request.Compose)
	default:
		return compose.Down(ctx, request.Project, request.Compose, request.RemoveVolumes)
	}
}

// failed says a stack action failed, why, and what compose printed before it
// did.
func (uc *UseCase) failed(ctx context.Context, stackUUID string, action stack.Action, cause error, output string) error {
	return uc.publish(ctx, events.StackFailedName, events.StackFailed{
		StackUUID: stackUUID,
		NodeName:  uc.nodeName,
		Action:    action,
		Reason:    cause.Error(),
		Output:    output,
		At:        time.Now(),
	})
}

// refused says a stack action was not run because the command did not say
// what it had to.
func (uc *UseCase) refused(ctx context.Context, request *Request, validationErrors domain.ValidationErrors) error {
	if len(validationErrors) == 0 || len(request.StackUUID) == 0 {
		return nil
	}

	fields := slices.Sorted(maps.Keys(validationErrors))

	reasons := make([]string, len(fields))
	for n, field := range fields {
		reasons[n] = field + ": " + validationErrors[field]
	}

	return uc.failed(ctx, request.StackUUID, request.Action, fmt.Errorf("the command was refused: %s", strings.Join(reasons, "; ")), "")
}

func (uc *UseCase) publish(ctx context.Context, subject string, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return uc.producer.Produce(context.WithoutCancel(ctx), subject, payload)
}

// lastOf is the last limit bytes of output, starting at a whole character.
func lastOf(output string, limit int) string {
	if len(output) <= limit {
		return output
	}

	output = output[len(output)-limit:]
	for len(output) > 0 && !utf8.RuneStart(output[0]) {
		output = output[1:]
	}

	return output
}
