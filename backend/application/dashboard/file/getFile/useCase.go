package getfile

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/file"
)

type UseCase struct {
	filesRepository file.Repository
	storage         file.Storage
}

func NewUseCase(filesRepository file.Repository, storage file.Storage) *UseCase {
	return &UseCase{
		filesRepository: filesRepository,
		storage:         storage,
	}
}

func (uc *UseCase) Execute(UUID string) (*Response, error) {
	f, err := uc.filesRepository.GetOne(UUID)
	if err != nil {
		return nil, err
	}

	reader, err := uc.storage.Read(context.Background(), f.Name)
	if err != nil {
		return nil, err
	}

	response := Response{
		Name:      f.Name,
		Size:      f.Size,
		OwnerUUID: f.OwnerUUID,
		MimeType:  f.MimeType,
		CreatedAt: f.CreatedAt,

		Reader: reader,
	}

	return &response, nil
}
