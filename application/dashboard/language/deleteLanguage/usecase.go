package deletelanguage

import "github.com/khanzadimahdi/testproject/domain/language"

type UseCase struct {
	languageRepository language.Repository
}

func NewUseCase(languageRepository language.Repository) *UseCase {
	return &UseCase{
		languageRepository: languageRepository,
	}
}

func (uc *UseCase) Execute(request *Request) error {
	return uc.languageRepository.Delete(request.Code)
}
