package verify

import (
	"crypto/rand"
	"encoding/base64"
	"errors"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/config"
	"github.com/khanzadimahdi/testproject/domain/password"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
	"github.com/khanzadimahdi/testproject/infrastructure/jwt"
)

type UseCase struct {
	userRepository   user.Repository
	roleRepository   role.Repository
	configRepository config.Repository
	hasher           password.Hasher
	jwt              *jwt.JWT
	translator       translator.Translator
	validator        domain.Validator
}

func NewUseCase(
	userRepository user.Repository,
	roleRepository role.Repository,
	configRepository config.Repository,
	hasher password.Hasher,
	JWT *jwt.JWT,
	translator translator.Translator,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		userRepository:   userRepository,
		roleRepository:   roleRepository,
		configRepository: configRepository,
		hasher:           hasher,
		jwt:              JWT,
		translator:       translator,
		validator:        validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	registrationToken, err := base64.URLEncoding.DecodeString(request.Token)
	if err != nil {
		return nil, err
	}

	claims, err := uc.jwt.Verify(string(registrationToken))
	if err != nil {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"token": err.Error(),
			},
		}, nil
	}

	if audiences, err := claims.GetAudience(); err != nil || len(audiences) == 0 || audiences[0] != auth.RegistrationToken {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"token": uc.translator.Translate("registration token is not valid"),
			},
		}, nil
	}

	identity, err := claims.GetSubject()
	if err != nil {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"token": err.Error(),
			},
		}, nil
	}

	if exists, err := uc.identityExists(identity); err != nil {
		return nil, err
	} else if exists {
		return &Response{
			ValidationErrors: map[string]string{
				"identity": uc.translator.Translate("user already exists"),
			},
		}, nil
	}

	if exists, err := uc.identityExists(request.Username); err != nil {
		return nil, err
	} else if exists {
		return &Response{
			ValidationErrors: map[string]string{
				"username": uc.translator.Translate("user with given username already exists"),
			},
		}, nil
	}

	salt := make([]byte, 64)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	u := user.User{
		Name:     request.Name,
		Username: request.Username,
		Email:    identity,
		PasswordHash: password.Hash{
			Value: uc.hasher.Hash([]byte(request.Password), salt),
			Salt:  salt,
		},
	}

	userUUID, err := uc.userRepository.Save(&u)
	if err != nil {
		return nil, err
	}

	if err := uc.assignDefaultRoles(userUUID); err != nil {
		return nil, err
	}

	return &Response{}, nil
}

func (uc *UseCase) assignDefaultRoles(userUUID string) error {
	c, err := uc.configRepository.GetLatestRevision()
	if errors.Is(err, domain.ErrNotExists) {
		return nil
	} else if err != nil {
		return err
	}

	roles, err := uc.roleRepository.GetByUUIDs(c.UserDefaultRoleUUIDs)
	if err != nil {
		return err
	}

	for i := range roles {
		userUUIDs := make([]string, len(roles[i].UserUUIDs)+1)
		copy(userUUIDs, roles[i].UserUUIDs)
		userUUIDs[len(userUUIDs)-1] = userUUID

		roles[i].UserUUIDs = userUUIDs

		if _, err := uc.roleRepository.Save(&roles[i]); err != nil {
			return err
		}
	}

	return nil
}

func (uc *UseCase) identityExists(identity string) (bool, error) {
	u, err := uc.userRepository.GetOneByIdentity(identity)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return u.UUID == identity || u.Email == identity || u.Username == identity, nil
}
