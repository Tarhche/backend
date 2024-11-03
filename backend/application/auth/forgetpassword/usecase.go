package forgetpassword

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/user"
)

const (
	templateName     = "resources/view/mail/auth/reset-password"
	resetPasswordURL = "https://tarhche.com/auth/reset-password?token="
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

	_, err := uc.userRepository.GetOneByIdentity(request.Identity)
	if err == domain.ErrNotExists {
		return &Response{
			ValidationErrors: validationErrors{
				"identity": "identity (email/username) not exists",
			},
		}, nil
	} else if err != nil {
		return nil, err
	}

	command := SendForgetPasswordEmail{
		Identity: request.Identity,
	}

	payload, err := json.Marshal(command)
	if err != nil {
		return nil, err
	}

	if err := uc.asyncCommandBus.Publish(context.Background(), SendForgetPasswordEmailName, payload); err != nil {
		return nil, err
	}

	return &Response{}, nil
}
