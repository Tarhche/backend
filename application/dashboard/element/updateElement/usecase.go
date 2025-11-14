package updateelement

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/element"
)

type UseCase struct {
	elementRepository element.Repository
	validator         domain.Validator
}

func NewUseCase(elementRepository element.Repository, validator domain.Validator) *UseCase {
	return &UseCase{
		elementRepository: elementRepository,
		validator:         validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	if _, err := uc.elementRepository.Save(request.ToElement()); err != nil {
		return nil, err
	}

	return nil, nil
}
