package getContentsByHashtag

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
	articleRepository  article.Repository
	noteRepository     note.Repository
	userRepository     user.Repository
	languageRepository language.Repository
	languageResolver   resolver.Resolver
	elementRetriever   *element.Retriever
	validator          domain.Validator
}

func NewUseCase(
	articleRepository article.Repository,
	noteRepository note.Repository,
	userRepository user.Repository,
	languageRepository language.Repository,
	languageResolver resolver.Resolver,
	elementRetriever *element.Retriever,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		articleRepository:  articleRepository,
		noteRepository:     noteRepository,
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

	hashtags := []string{request.Hashtag}

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

	totalArticles, err := uc.articleRepository.CountPublishedByHashtags(ctx, hashtags, languageCode)
	if err != nil {
		return nil, err
	}

	totalNotes, err := uc.noteRepository.CountPublishedByHashtags(ctx, hashtags, languageCode)
	if err != nil {
		return nil, err
	}

	// Each tab paginates on its own count. With no tab asked for, prefer
	// articles, falling back to notes when the hashtag has none — so a hashtag
	// used only on notes still opens on something.
	selectedType := request.Type
	if len(selectedType) == 0 {
		if totalArticles == 0 && totalNotes > 0 {
			selectedType = TypeNote
		} else {
			selectedType = TypeArticle
		}
	}

	total := totalArticles
	if selectedType == TypeNote {
		total = totalNotes
	}

	currentPage := request.Page
	if currentPage == 0 {
		currentPage = 1
	}

	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := total / limit

	if (totalPages * limit) != total {
		totalPages++
	}

	var contents []Content
	if selectedType == TypeNote {
		n, err := uc.noteRepository.GetPublishedByHashtags(ctx, hashtags, languageCode, offset, limit)
		if err != nil {
			return nil, err
		}

		contents = make([]Content, len(n))
		for i := range n {
			contents[i] = NewNoteContent(n[i])
		}
	} else {
		a, err := uc.articleRepository.GetPublishedByHashtags(ctx, hashtags, languageCode, offset, limit)
		if err != nil {
			return nil, err
		}

		contents = make([]Content, len(a))
		for i := range a {
			contents[i] = NewArticleContent(a[i])
		}
	}

	userUUIDs := make([]string, len(contents))
	for i := range contents {
		userUUIDs[i] = contents[i].AuthorUUID
	}

	authors, err := uc.userRepository.GetByUUIDs(ctx, userUUIDs)
	if err != nil {
		return nil, err
	}

	elementsResponse, err := uc.elementRetriever.RetrieveByVenues(
		ctx,
		[]string{fmt.Sprintf("/%s/hashtags/%s", languageCode, request.Hashtag)},
		languageCode,
	)
	if err != nil {
		return nil, err
	}

	publishedLanguages := make(map[string][]language.Language, len(contents))
	for i := range contents {
		var codes []string
		var err error

		if contents[i].IsNote() {
			codes, err = uc.noteRepository.GetPublishedLanguageCodes(ctx, contents[i].CorrelationUUID)
		} else {
			codes, err = uc.articleRepository.GetPublishedLanguageCodes(ctx, contents[i].CorrelationUUID)
		}
		if err != nil {
			return nil, err
		}

		cl, err := uc.languageRepository.GetByCodes(ctx, codes)
		if err != nil {
			return nil, err
		}
		publishedLanguages[contents[i].UUID] = cl
	}

	return NewResponse(selectedType, contents, authors, publishedLanguages, l, elementsResponse, totalArticles, totalNotes, totalPages, currentPage), nil
}
