package deleteelements

import "github.com/khanzadimahdi/testproject/domain/element"

const limit = 10

type UseCase struct {
	elementRepository element.Repository
}

func NewUseCase(elementRepository element.Repository) *UseCase {
	return &UseCase{
		elementRepository: elementRepository,
	}
}

func (uc *UseCase) GetElements(request *Request) (*GetElementsResponse, error) {
	totalElements, err := uc.elementRepository.Count()
	if err != nil {
		return nil, err
	}

	currentPage := request.Page
	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := totalElements / limit

	if (totalPages * limit) != totalElements {
		totalPages++
	}

	a, err := uc.elementRepository.GetAll(offset, limit)
	if err != nil {
		return nil, err
	}

	return NewGetElementsReponse(a, totalPages, currentPage), nil
}
