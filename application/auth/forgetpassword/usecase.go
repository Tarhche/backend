package forgetpassword

import (
	"context"
	"encoding/json"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
)

const (
	templateName     = "resources/view/mail/auth/reset-password"
	resetPasswordURL = "https://tarhche.com/auth/reset-password?token="
)

type UseCase struct {
	userRepository  user.Repository
	asyncCommandBus domain.PublishSubscriber
	translator      translator.Translator
	validator       domain.Validator
}

func NewUseCase(
	userRepository user.Repository,
	asyncCommandBus domain.PublishSubscriber,
	translator translator.Translator,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		userRepository:  userRepository,
		asyncCommandBus: asyncCommandBus,
		translator:      translator,
		validator:       validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	_, err := uc.userRepository.GetOneByIdentity(request.Identity)
	if err == domain.ErrNotExists {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"identity": uc.translator.Translate("identity_not_exists"),
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

	err = uc.asyncCommandBus.Publish(context.Background(), SendForgetPasswordEmailName, payload)

	return nil, err
}
