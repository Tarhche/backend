package getlanguages

import "github.com/khanzadimahdi/testproject/domain/language"

const limit = 10

type UseCase struct {
	languageRepository language.Repository
}

func NewUseCase(languageRepository language.Repository) *UseCase {
	return &UseCase{
		languageRepository: languageRepository,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	totalLanguages, err := uc.languageRepository.Count()
	if err != nil {
		return nil, err
	}

	currentPage := request.Page
	if currentPage == 0 {
		currentPage = 1
	}

	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := totalLanguages / limit

	if (totalPages * limit) != totalLanguages {
		totalPages++
	}

	languages, err := uc.languageRepository.GetAll(offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(languages, totalPages, currentPage), nil
}
