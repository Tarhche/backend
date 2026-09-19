package providerredirect

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

// stateBytes is how much randomness a state carries. It is compared, never
// parsed, so it only has to be unguessable.
const stateBytes = 32

// UseCase says where to send somebody who wants to sign in with somebody else's
// account, and gives out the state that will prove the answer is to this
// question.
type UseCase struct {
	providers  oauth.Providers
	translator translator.Translator
	validator  domain.Validator
}

func NewUseCase(
	providers oauth.Providers,
	translator translator.Translator,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		providers:  providers,
		translator: translator,
		validator:  validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	provider, err := uc.providers.Get(request.Provider)
	if errors.Is(err, oauth.ErrUnknownProvider) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"provider": uc.translator.Translate("unknown_login_provider"),
			},
		}, nil
	} else if err != nil {
		return nil, err
	}

	state := make([]byte, stateBytes)
	if _, err := rand.Read(state); err != nil {
		return nil, err
	}

	encoded := base64.RawURLEncoding.EncodeToString(state)

	return &Response{
		URL:   provider.AuthorizationURL(encoded),
		State: encoded,
	}, nil
}
