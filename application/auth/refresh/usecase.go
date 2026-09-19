package refresh

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type UseCase struct {
	userRepository     user.Repository
	jwt                *jwt.JWT
	authTokenGenerator *auth.AuthTokenGenerator
	authorizer         domain.Authorizer
	translator         translator.Translator
	validator          domain.Validator
}

func NewUseCase(
	userRepository user.Repository,
	jwt *jwt.JWT,
	authTokenGenerator *auth.AuthTokenGenerator,
	authorizer domain.Authorizer,
	translator translator.Translator,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		userRepository:     userRepository,
		jwt:                jwt,
		authTokenGenerator: authTokenGenerator,
		authorizer:         authorizer,
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

	claims, err := uc.jwt.Verify(ctx, request.Token)
	if err != nil {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"token": err.Error(),
			},
		}, nil
	}

	if audiences, err := claims.GetAudience(); err != nil || len(audiences) == 0 || audiences[0] != auth.RefreshToken {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"token": err.Error(),
			},
		}, nil
	}

	userUUID, err := claims.GetSubject()
	if err != nil {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"token": err.Error(),
			},
		}, nil
	}

	u, err := uc.userRepository.GetOne(ctx, userUUID)
	if err == domain.ErrNotExists {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"identity": uc.translator.Translate("identity_not_exists"),
			},
		}, nil
	} else if err != nil {
		return nil, err
	}

	if u.IsBanned() {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"identity": uc.translator.Translate("user_is_banned"),
			},
		}, nil
	}

	// a shadow session refreshes as itself: the new tokens are the impersonated
	// user's and still say who is behind them. Whoever that is has to still be
	// allowed to be there, which is asked again here rather than once, when the
	// session was opened.
	var tokenOptions []auth.TokenOption
	if impersonatorUUID := jwt.Impersonator(claims); len(impersonatorUUID) > 0 {
		allowed, err := uc.impersonationIsStillAllowed(ctx, impersonatorUUID)
		if err != nil {
			return nil, err
		}

		if !allowed {
			return &Response{
				ValidationErrors: domain.ValidationErrors{
					"token": uc.translator.Translate("impersonation_not_allowed"),
				},
			}, nil
		}

		tokenOptions = append(tokenOptions, auth.OnBehalfOf(impersonatorUUID))
	}

	accessToken, err := uc.authTokenGenerator.GenerateAccessToken(ctx, &u, tokenOptions...)
	if err != nil {
		return nil, err
	}

	refreshToken, err := uc.authTokenGenerator.GenerateRefreshToken(ctx, u.UUID, tokenOptions...)
	if err != nil {
		return nil, err
	}

	return &Response{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

// impersonationIsStillAllowed asks again what was asked when the shadow session
// was opened: that whoever is behind it still exists, may still act, and may
// still be seen as somebody else. A permission taken away or a ban handed down
// ends the session at the next refresh, rather than whenever the refresh token
// happens to run out.
func (uc *UseCase) impersonationIsStillAllowed(ctx context.Context, impersonatorUUID string) (bool, error) {
	impersonator, err := uc.userRepository.GetOne(ctx, impersonatorUUID)
	if err == domain.ErrNotExists {
		return false, nil
	} else if err != nil {
		return false, err
	}

	if impersonator.IsBanned() {
		return false, nil
	}

	return uc.authorizer.Authorize(ctx, impersonatorUUID, permission.UsersImpersonate)
}
