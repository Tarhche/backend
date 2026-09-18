package impersonateuser

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
)

// UseCase hands somebody a session that is not theirs.
//
// The tokens are the impersonated user's in every way that matters -- their
// permissions, their language, their articles -- so the dashboard shows what
// they would see. What the tokens also carry is who asked for them, which is
// how the dashboard can say whose eyes these are, and how the session can be
// refreshed without quietly turning into the impersonated user's own.
//
// Whether the caller may do this at all is the permission on the route, the
// same as every other thing the dashboard does.
type UseCase struct {
	userRepository     user.Repository
	authTokenGenerator *auth.AuthTokenGenerator
	translator         translator.Translator
	validator          domain.Validator
}

func NewUseCase(
	userRepository user.Repository,
	authTokenGenerator *auth.AuthTokenGenerator,
	translator translator.Translator,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		userRepository:     userRepository,
		authTokenGenerator: authTokenGenerator,
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

	// a shadow session says who is behind it, and one name is all it holds. Were
	// it allowed to nest, leaving would hand the caller back to somebody they
	// never were.
	if request.CallerIsAlreadyImpersonating {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"uuid": uc.translator.Translate("impersonation_does_not_nest"),
			},
		}, nil
	}

	if request.UserUUID == request.ImpersonatorUUID {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"uuid": uc.translator.Translate("already_signed_in_as_this_user"),
			},
		}, nil
	}

	u, err := uc.userRepository.GetOne(ctx, request.UserUUID)
	if err == domain.ErrNotExists {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"uuid": uc.translator.Translate("identity_not_exists"),
			},
		}, nil
	} else if err != nil {
		return nil, err
	}

	// a banned user cannot act, and a session opened as them would be refused at
	// the next request anyway. It is somebody else's account being spoken of
	// here, so it is not the message that account's owner would be given.
	if u.IsBanned() {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"uuid": uc.translator.Translate("impersonated_user_is_banned"),
			},
		}, nil
	}

	accessToken, err := uc.authTokenGenerator.GenerateAccessToken(ctx, &u, auth.OnBehalfOf(request.ImpersonatorUUID))
	if err != nil {
		return nil, err
	}

	refreshToken, err := uc.authTokenGenerator.GenerateRefreshToken(ctx, u.UUID, auth.OnBehalfOf(request.ImpersonatorUUID))
	if err != nil {
		return nil, err
	}

	return NewResponse(u, accessToken, refreshToken), nil
}
