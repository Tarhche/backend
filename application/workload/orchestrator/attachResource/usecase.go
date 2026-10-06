// Package attachResource opens a stream, a terminal say, in a resource this
// node holds, whatever its kind.
package attachResource

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// UseCase opens streams in resources, for their owners alone.
//
// Whose a resource is, its kind's node strategy reads off the instance
// itself, as a VM's owner is read off its labels: the node answers without a
// database and without taking anybody's word for it. So all this does is say
// who is asking. Nobody is nobody's owner, so a stream is opened for nobody
// who is not asking as somebody; and somebody who may not open it, because it
// is not theirs, because its kind serves no streams or not this one, or
// because this node runs no such kind, is told it is not there, so knowing a
// uuid says nothing about whether one exists.
type UseCase struct {
	kinds     *kind.Registry[kind.NodeBinding]
	validator domain.Validator
}

func NewUseCase(kinds *kind.Registry[kind.NodeBinding], validator domain.Validator) *UseCase {
	return &UseCase{kinds: kinds, validator: validator}
}

// Execute opens the stream. One that is there and cannot be opened now, in a
// resource that is not running, is kind.ErrUnreachable, with why.
func (uc *UseCase) Execute(ctx context.Context, request *Request) (kind.Session, domain.ValidationErrors, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return nil, validationErrors, nil
	}

	notThere := fmt.Errorf("%w: no %s %q of theirs", domain.ErrNotExists, request.Kind, request.UUID)

	binding, runs := uc.kinds.Lookup(request.Kind)
	if !runs || !binding.Attaches() || len(request.OwnerUUID) == 0 {
		return nil, nil, notThere
	}

	session, err := binding.Attach(ctx, request.Action, request.UUID, request.OwnerUUID)
	if errors.Is(err, kind.ErrUnknownAction) {
		return nil, nil, notThere
	}

	if err != nil {
		return nil, nil, err
	}

	return session, nil, nil
}
