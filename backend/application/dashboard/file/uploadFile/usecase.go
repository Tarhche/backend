package createfile

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

func (uc *UseCase) Execute(request Request) (*Response, error) {
	if ok, validation := request.Validate(); !ok {
		return &Response{
			ValidationErrors: validation,
		}, nil
	}

	if err := uc.storage.Store(context.Background(), request.Name, request.FileReader, request.Size); err != nil {
		return nil, err
	}

	uuid, err := uc.filesRepository.Save(&file.File{
		Name:      request.Name,
		Size:      request.Size,
		OwnerUUID: request.OwnerUUID,
	})
	if err != nil {
		return nil, err
	}

	return &Response{UUID: uuid}, nil
}
