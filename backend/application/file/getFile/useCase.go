package getfile

import (
	"context"
	"io"

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

func (uc *UseCase) Execute(UUID string, writer io.Writer) error {
	f, err := uc.filesRepository.GetOne(UUID)
	if err != nil {
		return err
	}

	reader, err := uc.storage.Read(context.Background(), f.Name)
	if err != nil {
		return err
	}
	defer reader.Close()

	_, err = io.Copy(writer, reader)

	return err
}
