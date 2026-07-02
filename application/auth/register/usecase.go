package register

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	userRepository  user.Repository
	asyncCommandBus domain.ProduceConsumer
	translator      translator.Translator
	validator       domain.Validator
}

func NewUseCase(
	userRepository user.Repository,
	asyncCommandBus domain.ProduceConsumer,
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

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	if exists, err := uc.userExists(ctx, request.Identity); err != nil {
		return nil, err
	} else if exists {
		return &Response{
			ValidationErrors: map[string]string{
				"identity": uc.translator.Translate("email_already_exists"),
			},
		}, nil
	}

	command := &SendRegistrationEmail{
		Identity:     request.Identity,
		LanguageCode: request.LanguageCode,
	}

	payload, err := json.Marshal(command)
	if err != nil {
		return nil, err
	}

	err = uc.asyncCommandBus.Produce(ctx, SendRegisterationEmailName, payload)

	return nil, err
}

func (uc *UseCase) userExists(ctx context.Context, identity string) (bool, error) {
	u, err := uc.userRepository.GetOneByIdentity(ctx, identity)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return u.Email == identity || u.Username == identity, nil
}
