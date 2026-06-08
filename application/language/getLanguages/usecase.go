package getlanguages

import "github.com/khanzadimahdi/testproject/domain/language"

type UseCase struct {
	languageRepository language.Repository
}

func NewUseCase(languageRepository language.Repository) *UseCase {
	return &UseCase{
		languageRepository: languageRepository,
	}
}

func (uc *UseCase) Execute() (*Response, error) {
	total, err := uc.languageRepository.Count()
	if err != nil {
		return nil, err
	}

	languages, err := uc.languageRepository.GetAll(0, total)
	if err != nil {
		return nil, err
	}

	return NewResponse(languages), nil
}
