package getfiles

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

func (uc *UseCase) GetFiles(request *Request) (*GetFilesReponse, error) {
	totalFiles, err := uc.fileRepository.Count()
	if err != nil {
		return nil, err
	}

	currentPage := request.Page
	var offset uint = 0
	if currentPage > 0 {
		offset = (currentPage - 1) * limit
	}

	totalPages := totalFiles / limit

	if (totalPages * limit) != totalFiles {
		totalPages++
	}

	a, err := uc.fileRepository.GetAll(offset, limit)
	if err != nil {
		return nil, err
	}

	return NewGetFilesReponse(a, totalPages, currentPage), nil
}
