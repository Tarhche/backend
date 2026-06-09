package getlanguage

import "github.com/khanzadimahdi/testproject/domain/language"

type UseCase struct {
	languageRepository language.Repository
}

func NewUseCase(languageRepository language.Repository) *UseCase {
	return &UseCase{
		languageRepository: languageRepository,
	}
}

func (uc *UseCase) Execute(code string) (*Response, error) {
	l, err := uc.languageRepository.GetOne(code)
	if err != nil {
		return nil, err
	}

	return NewResponse(l), nil
}
