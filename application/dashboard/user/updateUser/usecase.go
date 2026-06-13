package updateuser

import (
	"errors"

	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	userRepository   user.Repository
	languageResolver resolver.Resolver
	validator        domain.Validator
	translator       translator.Translator
}

func NewUseCase(
	userRepository user.Repository,
	languageResolver resolver.Resolver,
	validator domain.Validator,
	translator translator.Translator,
) *UseCase {
	return &UseCase{
		userRepository:   userRepository,
		languageResolver: languageResolver,
		validator:        validator,
		translator:       translator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	// make sure email is unique
	exists, err := uc.anotherUserExists(request.Email, request.UserUUID)
	if err != nil {
		return nil, err
	} else if exists {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"email": uc.translator.Translate("email_already_exists"),
			},
		}, nil
	}

	// make sure username is unique
	exists, err = uc.anotherUserExists(request.Username, request.UserUUID)
	if err != nil {
		return nil, err
	} else if exists {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"username": uc.translator.Translate("username_already_exists"),
			},
		}, nil
	}

	if !uc.languageResolver.Verify(request.LanguageCode) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"language_code": uc.translator.Translate("invalid_value"),
			},
		}, nil
	}

	u, err := uc.userRepository.GetOne(request.UserUUID)
	if err != nil {
		return nil, err
	}

	u.Name = request.Name
	u.Avatar = request.Avatar
	u.Email = request.Email
	u.Username = request.Username
	u.LanguageCode = request.LanguageCode

	_, err = uc.userRepository.Save(&u)

	return nil, err
}

func (uc *UseCase) anotherUserExists(identity string, userUUID string) (bool, error) {
	u, err := uc.userRepository.GetOneByIdentity(identity)
	if errors.Is(err, domain.ErrNotExists) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	return u.UUID != userUUID, nil
}
