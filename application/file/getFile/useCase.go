package getfile

import (
	"context"
	"io"

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

func (uc *UseCase) GetFile(UUID string, writer io.Writer) error {
	file, err := uc.filesRepository.GetOne(UUID)
	if err != nil {
		return err
	}

	reader, err := uc.storage.Read(context.Background(), file.Name)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, reader)

	return err
}
