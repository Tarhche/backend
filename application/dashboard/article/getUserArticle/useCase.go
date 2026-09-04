package getuserarticle

import (
	"context"
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
)

// UseCase shows one of the articles somebody wrote.
//
// It asks for the article as theirs, so one that is somebody else's is not
// found rather than refused: to whoever may only reach their own, an article
// that is not theirs and one that does not exist are the same thing.
type UseCase struct {
	articleRepository article.Repository
	userRepository    user.Repository
}

func NewUseCase(articleRepository article.Repository, userRepository user.Repository) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
		userRepository:    userRepository,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	a, err := uc.articleRepository.GetByCorrelationUUIDAndLanguageAndAuthor(ctx, request.CorrelationUUID, request.LanguageCode, request.AuthorUUID)
	if err != nil {
		return nil, err
	}

	u, err := uc.userRepository.GetOne(ctx, a.AuthorUUID)
	if err != nil && !errors.Is(err, domain.ErrNotExists) {
		return nil, err
	}

	return NewResponse(a, u), nil
}
