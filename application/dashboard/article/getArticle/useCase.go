package getarticle

import (
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
)

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

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	a, err := uc.articleRepository.GetByCorrelationUUIDAndLanguage(request.CorrelationUUID, request.LanguageCode)
	if err != nil {
		return nil, err
	}

	u, err := uc.userRepository.GetOne(a.AuthorUUID)
	if err != nil && !errors.Is(err, domain.ErrNotExists) {
		return nil, err
	}

	return NewResponse(a, u), nil
}
