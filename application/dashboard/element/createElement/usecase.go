package createelement

import (
	"github.com/khanzadimahdi/testproject/domain/element"
)

type UseCase struct {
	elementRepository element.Repository
}

func NewUseCase(elementRepository element.Repository) *UseCase {
	return &UseCase{
		elementRepository: elementRepository,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if ok, validation := request.Validate(); !ok {
		return &Response{
			ValidationErrors: validation,
		}, nil
	}

	elem := element.Element{
		Type:   request.Type,
		Body:   request.Body,
		Venues: request.Venues,
	}

	uuid, err := uc.elementRepository.Save(&elem)
	if err != nil {
		return nil, err
	}

	return &Response{UUID: uuid}, err
}
