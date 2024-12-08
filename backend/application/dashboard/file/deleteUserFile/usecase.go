package deleteuserfile

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

func (uc *UseCase) Execute(request Request) error {
	file, err := uc.filesRepository.GetOneByOwnerUUID(request.OwnerUUID, request.FileUUID)
	if err != nil {
		return err
	}

	if err := uc.storage.Delete(context.Background(), file.Name); err != nil {
		return err
	}

	return uc.filesRepository.DeleteByOwnerUUID(request.OwnerUUID, file.UUID)
}
