package deletelanguage

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/language"
)

type UseCase struct {
	languageRepository language.Repository
}

func NewUseCase(languageRepository language.Repository) *UseCase {
	return &UseCase{
		languageRepository: languageRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) error {
	return uc.languageRepository.Delete(ctx, request.Code)
}
