// Package watchStacks keeps the dashboard's list of stacks as it is, without
// asking for it again.
//
// One request opens the stream and the reply to it is that stream: a message
// for each stack that changed, and one for each that is gone, until the client
// says it has seen enough. A stack's state is read off its services, so a stack
// changes whenever one of them does.
package watchStacks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/owners"
	"github.com/khanzadimahdi/testproject/application/dashboard/runner/presenter"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	runnerManager "github.com/khanzadimahdi/testproject/domain/runner/manager"
	"github.com/khanzadimahdi/testproject/infrastructure/websocket/gateway"
)

// WatchName is the subject a client opens a stacks watch on.
const WatchName = "runnerStacksWatch"

// the kinds of change a client is told about.
const (
	kindChanged = "changed"
	kindDeleted = "deleted"
)

// Request opens a watch on the stacks.
//
// The access token travels in the payload because a websocket handshake from a
// browser carries no Authorization header, and one connection is shared by
// every request on it: the person is established per request, not per socket.
type Request struct {
	ID          string `json:"id"`
	AccessToken string `json:"access_token"`
}

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.AccessToken) == 0 {
		validationErrors["access_token"] = "required_field"
	}

	return validationErrors
}

// ChangeResponse is what became of one stack, as the dashboard shows it. A
// stack that is gone is reported by uuid alone, because there is nothing left
// to describe.
type ChangeResponse struct {
	Kind  string           `json:"kind"`
	UUID  string           `json:"uuid"`
	Stack *presenter.Stack `json:"stack,omitempty"`
}

// UseCase watches the stacks on behalf of the clients showing them.
type UseCase struct {
	runner        runnerManager.Client
	authenticator *auth.Authenticator
	authorizer    domain.Authorizer
	validator     domain.Validator
	replyer       domain.Replyer
	streams       *gateway.Streams
	owners        *owners.Directory
	ingressDomain string
	logger        *slog.Logger
}

var _ domain.MessageHandler = &UseCase{}

func NewUseCase(
	runner runnerManager.Client,
	authenticator *auth.Authenticator,
	authorizer domain.Authorizer,
	validator domain.Validator,
	replyer domain.Replyer,
	streams *gateway.Streams,
	ownerDirectory *owners.Directory,
	ingressDomain string,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		runner:        runner,
		authenticator: authenticator,
		authorizer:    authorizer,
		validator:     validator,
		replyer:       replyer,
		streams:       streams,
		owners:        ownerDirectory,
		ingressDomain: ingressDomain,
		logger:        logger,
	}
}

func (uc *UseCase) Handle(ctx context.Context, data []byte) error {
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		return nil
	}

	if validationErrors := uc.validator.Validate(&request); len(validationErrors) > 0 {
		return uc.fail(ctx, request.ID, validationErrors)
	}

	user, err := uc.authenticator.Authenticate(ctx, request.AccessToken)
	if err != nil {
		return uc.fail(ctx, request.ID, domain.ValidationErrors{"access_token": "unauthenticated"})
	}

	// a watch shows what a listing shows, so it is the listing's permission —
	// and somebody who may only list their own is only told about their own.
	all, err := uc.authorizer.Authorize(ctx, user.UUID, permission.RunnerStacksIndex)
	if err != nil {
		return err
	}

	// what they are told about: everybody's, or their own alone.
	var watching string

	if !all {
		own, err := uc.authorizer.Authorize(ctx, user.UUID, permission.SelfRunnerStacksIndex)
		if err != nil {
			return err
		}

		if !own {
			return uc.fail(ctx, request.ID, domain.ValidationErrors{"access_token": "forbidden"})
		}

		watching = user.UUID
	}

	stream, err := uc.runner.WatchStacks(ctx)
	if err != nil {
		return err
	}

	uc.serve(ctx, request.ID, watching, stream)

	return nil
}

// serve carries the stacks' changes to the client watching them.
func (uc *UseCase) serve(ctx context.Context, requestID string, watching string, stream runnerManager.StackStream) {
	// detached from the message that asked for it: the watch lives as long as
	// the person watching, not as long as the request.
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))

	uc.streams.Add(requestID, cancel)

	go func() {
		defer func() {
			uc.streams.Remove(requestID)
			_ = stream.Close()
			cancel()

			uc.end(ctx, requestID)
		}()

		for {
			change, err := stream.Next(ctx)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					uc.logger.WarnContext(ctx, "a stack watch ended", "error", err)
				}

				return
			}

			// a change to somebody else's stack is not this client's news.
			if len(watching) > 0 && !change.Deleted && change.Stack.OwnerUUID != watching {
				continue
			}

			response, err := uc.response(ctx, change)
			if err != nil {
				uc.logger.ErrorContext(ctx, "error on reading who a change belongs to", "error", err)

				return
			}

			payload, err := json.Marshal(response)
			if err != nil {
				uc.logger.ErrorContext(ctx, "error on marshalling a stack change", "error", err)

				return
			}

			if err := uc.replyer.Reply(ctx, &domain.Reply{
				RequestID: requestID,
				Kind:      domain.ReplyChunk,
				Payload:   payload,
			}); err != nil {
				uc.logger.ErrorContext(ctx, "error on writing a stack change", "error", err)

				return
			}
		}
	}()
}

func (uc *UseCase) response(ctx context.Context, change runnerManager.StackChange) (ChangeResponse, error) {
	if change.Deleted {
		return ChangeResponse{Kind: kindDeleted, UUID: change.UUID}, nil
	}

	ownerUUIDs := make([]string, 0, len(change.Stack.Services)+1)
	ownerUUIDs = append(ownerUUIDs, change.Stack.OwnerUUID)
	for i := range change.Stack.Services {
		ownerUUIDs = append(ownerUUIDs, change.Stack.Services[i].OwnerUUID)
	}

	people, err := uc.owners.Of(ctx, ownerUUIDs...)
	if err != nil {
		return ChangeResponse{}, err
	}

	stack := presenter.NewStack(change.Stack, uc.ingressDomain, people)

	return ChangeResponse{Kind: kindChanged, UUID: change.UUID, Stack: &stack}, nil
}

func (uc *UseCase) end(ctx context.Context, requestID string) {
	if err := uc.replyer.Reply(ctx, &domain.Reply{RequestID: requestID, Kind: domain.ReplyEOF}); err != nil {
		uc.logger.ErrorContext(ctx, "error on ending a stack watch", "error", err)
	}
}

func (uc *UseCase) fail(ctx context.Context, requestID string, validationErrors domain.ValidationErrors) error {
	payload, err := json.Marshal(map[string]any{"errors": validationErrors})
	if err != nil {
		return err
	}

	return uc.replyer.Reply(ctx, &domain.Reply{
		RequestID: requestID,
		Kind:      domain.ReplyEOF,
		Payload:   payload,
	})
}
