package getusers

import (
	"github.com/khanzadimahdi/testproject/domain/user"
)

const limit = 10

type UseCase struct {
	userRepository user.Repository
}

func NewUseCase(articleRepository user.Repository) *UseCase {
	return &UseCase{
		userRepository: articleRepository,
	}
}

func (uc *UseCase) GetArticles(request *Request) (*Response, error) {
	totalArticles, err := uc.userRepository.Count()
	if err != nil {
		return nil, err
	}

	currentPage := request.Page
	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := totalArticles / limit

	if (totalPages * limit) != totalArticles {
		totalPages++
	}

	a, err := uc.userRepository.GetAll(offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(a, totalPages, currentPage), nil
}
