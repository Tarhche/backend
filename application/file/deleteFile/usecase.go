package deletefile

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

func (uc *UseCase) DeleteFile(request Request) error {
	file, err := uc.filesRepository.GetOne(request.FileUUID)
	if err != nil {
		return err
	}

	if err := uc.storage.Delete(context.Background(), file.Name); err != nil {
		return err
	}

	return uc.filesRepository.Delete(file.UUID)
}
