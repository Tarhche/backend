package login

import (
	"log"
	"time"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type UseCase struct {
	userRepository user.Repository
	JWT            *jwt.JWT
	Hasher         password.Hasher
}

func NewUseCase(userRepository user.Repository, JWT *jwt.JWT, hasher password.Hasher) *UseCase {
	return &UseCase{
		userRepository: userRepository,
		JWT:            JWT,
		Hasher:         hasher,
	}
}

func (uc *UseCase) Login(request Request) (*LoginResponse, error) {
	if ok, validation := request.Validate(); !ok {
		return &LoginResponse{
			ValidationErrors: validation,
		}, nil
	}
	log.Printf("%#v", request)
	u, err := uc.userRepository.GetOneByIdentity(request.Identity)
	if err != nil {
		return nil, err
	}

	if !uc.passwordIsValid(u, []byte(request.Password)) {
		return &LoginResponse{
			ValidationErrors: validationErrors{
				"identity": "your identity or password is wrong",
			},
		}, nil
	}

	accessToken, err := uc.generateAccessToken(u)
	if err != nil {
		return nil, err
	}

	refreshToken, err := uc.generateRefreshToken(u)
	if err != nil {
		return nil, err
	}

	return &LoginResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (uc *UseCase) passwordIsValid(u user.User, password []byte) bool {
	return uc.Hasher.Equal(password, u.PasswordHash.Value, u.PasswordHash.Salt)
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
