package getarticle

import (
	"context"
	"errors"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/user"
)

type UseCase struct {
	articleRepository  article.Repository
	userRepository     user.Repository
	languageRepository language.Repository
	languageResolver   resolver.Resolver
	elementRetriever   *element.Retriever
	validator          domain.Validator
}

func NewUseCase(
	articleRepository article.Repository,
	userRepository user.Repository,
	languageRepository language.Repository,
	languageResolver resolver.Resolver,
	elementRetriever *element.Retriever,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		articleRepository:  articleRepository,
		userRepository:     userRepository,
		languageRepository: languageRepository,
		languageResolver:   languageResolver,
		elementRetriever:   elementRetriever,
		validator:          validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	languageCode := request.LanguageCode
	if len(languageCode) == 0 {
		code, err := uc.languageResolver.DefaultCode(ctx)
		if err != nil {
			return nil, err
		}

		languageCode = code
	}

	l, err := uc.languageResolver.Resolve(ctx, languageCode)
	if err != nil {
		return nil, err
	}

	a, err := uc.articleRepository.GetOnePublished(ctx, request.CorrelationUUID, languageCode)
	if err != nil {
		return nil, err
	}

	author, err := uc.userRepository.GetOne(ctx, a.AuthorUUID)
	if err != nil && !errors.Is(err, domain.ErrNotExists) {
		return nil, err
	}

	availableLanguageCodes, err := uc.articleRepository.GetPublishedLanguageCodes(ctx, a.CorrelationUUID)
	if err != nil {
		return nil, err
	}

	availableLanguages, err := uc.languageRepository.GetByCodes(ctx, availableLanguageCodes)
	if err != nil {
		return nil, err
	}

	elementsResponse, err := uc.elementRetriever.RetrieveByVenues(
		ctx,
		[]string{fmt.Sprintf("/%s/articles/%s", languageCode, a.CorrelationUUID)},
		languageCode,
	)
	if err != nil {
		return nil, err
	}

	defer uc.articleRepository.IncreaseView(ctx, a.UUID, 1)

	return NewResponse(a, l, author, availableLanguages, elementsResponse), nil
}
