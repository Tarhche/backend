package refresh

import (
	"time"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type UseCase struct {
	userRepository user.Repository
	JWT            *jwt.JWT
}

func NewUseCase(userRepository user.Repository, JWT *jwt.JWT) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		JWT:            JWT,
	}
}

func (uc *UseCase) Execute(request Request) (*RefreshResponse, error) {
	if ok, validation := request.Validate(); !ok {
		return &RefreshResponse{
			ValidationErrors: validation,
		}, nil
	}

	claims, err := uc.JWT.Verify(request.Token)
	if err != nil {
		return &RefreshResponse{
			ValidationErrors: validationErrors{
				"token": err.Error(),
			},
		}, nil
	}

	if audiences, err := claims.GetAudience(); err != nil || len(audiences) == 0 || audiences[0] != auth.RefreshToken {
		return &RefreshResponse{
			ValidationErrors: validationErrors{
				"token": "refresh token is not valid",
			},
		}, nil
	}

	userUUID, err := claims.GetSubject()
	if err != nil {
		return &RefreshResponse{
			ValidationErrors: validationErrors{
				"token": err.Error(),
			},
		}, nil
	}

	u, err := uc.userRepository.GetOne(userUUID)
	if err != nil {
		return nil, err
	}

	accessToken, err := uc.generateAccessToken(u)
	if err != nil {
		return nil, err
	}

	refreshToken, err := uc.generateRefreshToken(u)
	if err != nil {
		return nil, err
	}

	return &RefreshResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (uc *UseCase) generateAccessToken(u user.User) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(u.UUID)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(15 * time.Minute))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{auth.AccessToken})

	return uc.JWT.Generate(b.Build())
}

func (uc *UseCase) generateRefreshToken(u user.User) (string, error) {
	b := jwt.NewClaimsBuilder()
	b.SetSubject(u.UUID)
	b.SetNotBefore(time.Now())
	b.SetExpirationTime(time.Now().Add(2 * 24 * time.Hour))
	b.SetIssuedAt(time.Now())
	b.SetAudience([]string{auth.RefreshToken})

	return uc.JWT.Generate(b.Build())
}
