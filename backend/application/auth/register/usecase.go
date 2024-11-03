package register

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	userRepository  user.Repository
	asyncCommandBus domain.PublishSubscriber
}

func NewUseCase(
	userRepository user.Repository,
	asyncCommandBus domain.PublishSubscriber,
) *UseCase {
	return &UseCase{
		userRepository:  userRepository,
		asyncCommandBus: asyncCommandBus,
	}
}

func (uc *UseCase) Execute(request Request) (*Response, error) {
	if ok, validation := request.Validate(); !ok {
		return &Response{
			ValidationErrors: validation,
		}, nil
	}

	if exists, err := uc.userExists(request.Identity); err != nil {
		return nil, err
	} else if exists {
		return &Response{
			ValidationErrors: map[string]string{
				"identity": "user with given email already exists",
			},
		}, nil
	}

	command := &SendRegistrationEmail{
		Identity: request.Identity,
	}

	payload, err := json.Marshal(command)
	if err != nil {
		return nil, err
	}

	if err := uc.asyncCommandBus.Publish(context.Background(), SendRegisterationEmailName, payload); err != nil {
		return nil, err
	}

	return &Response{}, nil
}

func (uc *UseCase) userExists(identity string) (bool, error) {
	u, err := uc.userRepository.GetOneByIdentity(identity)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return u.Email == identity || u.Username == identity, nil
}
