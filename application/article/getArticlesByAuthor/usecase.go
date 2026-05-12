package getArticlesByAuthor

import (
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/user"
)

const limit = 10

type UseCase struct {
	articleRepository article.Repository
	userRepository    user.Repository
	validator         domain.Validator
}

func NewUseCase(
	articleRepository article.Repository,
	userRepository user.Repository,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		articleRepository: articleRepository,
		userRepository:    userRepository,
		validator:         validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	author, err := uc.resolveAuthor(request)
	if err != nil {
		return nil, err
	}

	totalArticles, err := uc.articleRepository.CountPublishedByAuthor(author.UUID)
	if err != nil {
		return nil, err
	}

	currentPage := currentPageOf(request)

	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := totalArticles / limit

	if (totalPages * limit) != totalArticles {
		totalPages++
	}

	a, err := uc.articleRepository.GetPublishedByAuthor(author.UUID, offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(author, a, totalPages, currentPage), nil
}

func (uc *UseCase) resolveAuthor(request *Request) (user.User, error) {
	if len(request.AuthorUUID) > 0 {
		return uc.userRepository.GetOne(request.AuthorUUID)
	}

	return uc.userRepository.GetOneByIdentity(request.Username)
}

func currentPageOf(request *Request) uint {
	if request.Page == 0 {
		return 1
	}
	return request.Page
}
