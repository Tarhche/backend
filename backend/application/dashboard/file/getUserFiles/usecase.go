package getuserfiles

import (
	"github.com/khanzadimahdi/testproject/domain/file"
)

const limit = 10

type UseCase struct {
	fileRepository file.Repository
}

func NewUseCase(fileRepository file.Repository) *UseCase {
	return &UseCase{
		fileRepository: fileRepository,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	totalFiles, err := uc.fileRepository.CountByOwnerUUID(request.OwnerUUID)
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

	totalPages := totalFiles / limit

	if (totalPages * limit) != totalFiles {
		totalPages++
	}

	a, err := uc.fileRepository.GetAllByOwnerUUID(request.OwnerUUID, offset, limit)
	if err != nil {
		return nil, err
	}

	return NewResponse(a, totalPages, currentPage), nil
}
