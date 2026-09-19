package login

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/oauth"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
)

// UseCase signs somebody in, whichever way they are proving who they are.
//
// There are two, and they meet here because everything after the proof is the
// same: a user who may not be banned, and a pair of tokens. A password is
// checked against what this estate stored; a provider -- google, github,
// linkedin -- is asked, and answers with an account of its own that this estate
// either knows already or opens on the spot.
type UseCase struct {
	userRepository     user.Repository
	authTokenGenerator *auth.AuthTokenGenerator
	identities         *auth.Identities
	providers          oauth.Providers
	Hasher             password.Hasher
	translator         translator.Translator
	validator          domain.Validator
}

func NewUseCase(
	userRepository user.Repository,
	authTokenGenerator *auth.AuthTokenGenerator,
	identities *auth.Identities,
	providers oauth.Providers,
	hasher password.Hasher,
	translator translator.Translator,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		userRepository:     userRepository,
		authTokenGenerator: authTokenGenerator,
		identities:         identities,
		providers:          providers,
		Hasher:             hasher,
		translator:         translator,
		validator:          validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	u, response, err := uc.identify(ctx, request)
	if err != nil || response != nil {
		return response, err
	}

	if u.IsBanned() {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"identity": uc.translator.Translate("user_is_banned"),
			},
		}, nil
	}

	accessToken, err := uc.authTokenGenerator.GenerateAccessToken(ctx, &u)
	if err != nil {
		return nil, err
	}

	refreshToken, err := uc.authTokenGenerator.GenerateRefreshToken(ctx, u.UUID)
	if err != nil {
		return nil, err
	}

	return &Response{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// identify is who the request is from, or the answer to give when nobody is.
// A response and a user are never both returned: a refusal is not somebody.
func (uc *UseCase) identify(ctx context.Context, request *Request) (user.User, *Response, error) {
	if request.SignsInWithProvider() {
		return uc.identifyByProvider(ctx, request)
	}

	return uc.identifyByPassword(ctx, request)
}

func (uc *UseCase) identifyByPassword(ctx context.Context, request *Request) (user.User, *Response, error) {
	u, err := uc.userRepository.GetOneByIdentity(ctx, request.Identity)
	if errors.Is(err, domain.ErrNotExists) {
		return user.User{}, uc.refuse("identity", "invalid_identity_or_password"), nil
	} else if err != nil {
		return user.User{}, nil, err
	}

	if !uc.passwordIsValid(ctx, u, []byte(request.Password)) {
		return user.User{}, uc.refuse("identity", "invalid_identity_or_password"), nil
	}

	return u, nil, nil
}

// identifyByProvider takes the request's word for nothing: the code is the only
// thing read from it, and who that code stands for is the provider's to say.
func (uc *UseCase) identifyByProvider(ctx context.Context, request *Request) (user.User, *Response, error) {
	provider, err := uc.providers.Get(request.Provider)
	if errors.Is(err, oauth.ErrUnknownProvider) {
		return user.User{}, uc.refuse("provider", "unknown_login_provider"), nil
	} else if err != nil {
		return user.User{}, nil, err
	}

	identity, err := provider.Identify(ctx, request.Code)
	if err != nil {
		// a code that will not trade is a login that did not happen, which is
		// the caller's to hear about rather than a failure of this estate
		return user.User{}, uc.refuse("code", "invalid_login_provider_code"), nil
	}

	u, err := uc.identities.Resolve(ctx, identity)
	if errors.Is(err, auth.ErrNoVerifiedEmail) {
		return user.User{}, uc.refuse("provider", "provider_email_not_verified"), nil
	} else if err != nil {
		return user.User{}, nil, err
	}

	return u, nil, nil
}

func (uc *UseCase) refuse(field string, message string) *Response {
	return &Response{
		ValidationErrors: domain.ValidationErrors{
			field: uc.translator.Translate(message),
		},
	}
}

func (uc *UseCase) passwordIsValid(ctx context.Context, u user.User, password []byte) bool {
	return uc.Hasher.Equal(ctx, password, u.PasswordHash.Value, u.PasswordHash.Salt)
}
