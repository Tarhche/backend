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

func (uc *UseCase) Execute(ctx context.Context, request Request) error {
	file, err := uc.filesRepository.GetOneByOwnerUUID(ctx, request.OwnerUUID, request.FileUUID)
	if err != nil {
		return err
	}

	if err := uc.storage.Delete(ctx, file.StoredName); err != nil {
		return err
	}

	return uc.filesRepository.DeleteByOwnerUUID(ctx, request.OwnerUUID, file.UUID)
}
