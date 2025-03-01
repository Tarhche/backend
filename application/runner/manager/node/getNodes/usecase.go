package getNodes

import (
	"github.com/khanzadimahdi/testproject/domain/runner/node"
)

const limit = 10

type UseCase struct {
	nodeRepository node.Repository
}

func NewUseCase(nodeRepository node.Repository) *UseCase {
	return &UseCase{
		nodeRepository: nodeRepository,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	totalNodes, err := uc.nodeRepository.Count()
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

	totalPages := totalNodes / limit

	if (totalPages * limit) != totalNodes {
		totalPages++
	}

	nodes, err := uc.nodeRepository.GetAll(offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(nodes, totalPages, currentPage), nil
}
