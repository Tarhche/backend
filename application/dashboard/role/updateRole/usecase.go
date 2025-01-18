package updaterole

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"github.com/khanzadimahdi/testproject/domain/role"
	"github.com/khanzadimahdi/testproject/domain/translator"
)

type UseCase struct {
	roleRepository       role.Repository
	permissionRepository permission.Repository
	validator            domain.Validator
	translator           translator.Translator
}

func NewUseCase(
	roleRepository role.Repository,
	permissionRepository permission.Repository,
	validator domain.Validator,
	translator translator.Translator,
) *UseCase {
	return &UseCase{
		roleRepository:       roleRepository,
		permissionRepository: permissionRepository,
		validator:            validator,
		translator:           translator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	if permissions, err := uc.permissionRepository.Get(request.Permissions); err != nil {
		return nil, err
	} else if len(permissions) < len(request.Permissions) {
		return &Response{
			ValidationErrors: domain.ValidationErrors{
				"permissions": uc.translator.Translate("one_or_more_permissions_not_exist"),
			},
		}, nil
	}

	r := role.Role{
		UUID:        request.UUID,
		Name:        request.Name,
		Description: request.Description,
		Permissions: request.Permissions,
		UserUUIDs:   request.UserUUIDs,
	}

	_, err := uc.roleRepository.Save(&r)

	return nil, err
}
