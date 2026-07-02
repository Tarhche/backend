package getlanguage

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

func (uc *UseCase) Execute(ctx context.Context, code string) (*Response, error) {
	l, err := uc.languageRepository.GetOne(ctx, code)
	if err != nil {
		return nil, err
	}

	return NewResponse(l), nil
}
