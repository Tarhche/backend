package getNode

import (
	"github.com/khanzadimahdi/testproject/domain/runner/node"
)

type UseCase struct {
	nodeRepository node.Repository
}

func NewUseCase(nodeRepository node.Repository) *UseCase {
	return &UseCase{
		nodeRepository: nodeRepository,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	n, err := uc.nodeRepository.GetOne(request.Name)
	if err != nil {
		return nil, err
	}

	return NewResponse(&n), nil
}
