package getNotesByAuthor

import (
	"context"
	"fmt"

	"github.com/khanzadimahdi/testproject/application/element"
	"github.com/khanzadimahdi/testproject/application/language/resolver"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
	"github.com/khanzadimahdi/testproject/domain/note"
	"github.com/khanzadimahdi/testproject/domain/user"
)

const limit = 10

type UseCase struct {
	noteRepository     note.Repository
	articleRepository  article.Repository
	userRepository     user.Repository
	languageRepository language.Repository
	languageResolver   resolver.Resolver
	elementRetriever   *element.Retriever
	validator          domain.Validator
}

func NewUseCase(
	noteRepository note.Repository,
	articleRepository article.Repository,
	userRepository user.Repository,
	languageRepository language.Repository,
	languageResolver resolver.Resolver,
	elementRetriever *element.Retriever,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		noteRepository:     noteRepository,
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

	author, err := uc.resolveAuthor(ctx, request)
	if err != nil {
		return nil, err
	}

	totalNotes, err := uc.noteRepository.CountPublishedByAuthor(ctx, author.UUID, languageCode)
	if err != nil {
		return nil, err
	}

	// Only for the articles tab's label.
	totalArticles, err := uc.articleRepository.CountPublishedByAuthor(ctx, author.UUID, languageCode)
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

	totalPages := totalNotes / limit

	if (totalPages * limit) != totalNotes {
		totalPages++
	}

	n, err := uc.noteRepository.GetPublishedByAuthor(ctx, author.UUID, languageCode, offset, limit)
	if err != nil {
		return nil, err
	}

	elementsResponse, err := uc.elementRetriever.RetrieveByVenues(
		ctx,
		[]string{fmt.Sprintf("/%s/authors/%s/notes", languageCode, author.UUID)},
		languageCode,
	)
	if err != nil {
		return nil, err
	}

	publishedLanguages := make(map[string][]language.Language, len(n))
	for i := range n {
		codes, err := uc.noteRepository.GetPublishedLanguageCodes(ctx, n[i].CorrelationUUID)
		if err != nil {
			return nil, err
		}

		nl, err := uc.languageRepository.GetByCodes(ctx, codes)
		if err != nil {
			return nil, err
		}
		publishedLanguages[n[i].UUID] = nl
	}

	return NewResponse(author, n, publishedLanguages, l, elementsResponse, totalArticles, totalNotes, totalPages, currentPage), nil
}

func (uc *UseCase) resolveAuthor(ctx context.Context, request *Request) (user.User, error) {
	if len(request.AuthorUUID) > 0 {
		return uc.userRepository.GetOne(ctx, request.AuthorUUID)
	}

	return uc.userRepository.GetOneByIdentity(ctx, request.Username)
}
