package createfile

import (
	"context"

	"github.com/khanzadimahdi/testproject.git/domain/file"
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

func (uc *UseCase) UploadFile(request Request) (*UploadFileResponse, error) {
	if ok, validation := request.Validate(); !ok {
		return &UploadFileResponse{
			ValidationErrors: validation,
		}, nil
	}

	if err := uc.storage.Store(context.Background(), request.Name, request.FileReader, request.Size); err != nil {
		return nil, err
	}

	return &UploadFileResponse{}, uc.filesRepository.Save(&file.File{
		Name:      request.Name,
		Size:      request.Size,
		OwnerUUID: request.UserUUID,
	})
}
