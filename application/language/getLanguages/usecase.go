package getlanguages

import (
	"context"

	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain/language"
)

type UseCase struct {
	languageRepository language.Repository
	languageResolver   resolver.Resolver
}

func NewUseCase(languageRepository language.Repository, languageResolver resolver.Resolver) *UseCase {
	return &UseCase{
		languageRepository: languageRepository,
		languageResolver:   languageResolver,
	}
}

func (uc *UseCase) Execute(ctx context.Context) (*Response, error) {
	total, err := uc.languageRepository.Count(ctx)
	if err != nil {
		return nil, err
	}

	languages, err := uc.languageRepository.GetAll(ctx, 0, total)
	if err != nil {
		return nil, err
	}

	defaultCode, err := uc.languageResolver.DefaultCode(ctx)
	if err != nil {
		return nil, err
	}

	defaultLanguage, err := uc.languageResolver.Resolve(ctx, defaultCode)
	if err != nil {
		return nil, err
	}

	return NewResponse(languages, defaultLanguage), nil
}
