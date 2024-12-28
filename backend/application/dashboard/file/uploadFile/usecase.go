package createfile

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/file"
)

type UseCase struct {
	filesRepository file.Repository
	storage         file.Storage
	validator       domain.Validator
}

func NewUseCase(
	filesRepository file.Repository,
	storage file.Storage,
	validator domain.Validator,
) *UseCase {
	return &UseCase{
		filesRepository: filesRepository,
		storage:         storage,
		validator:       validator,
	}
}

func (uc *UseCase) Execute(request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{
			ValidationErrors: validationErrors,
		}, nil
	}

	uuid, err := uc.filesRepository.Save(&file.File{
		Name:      request.Name,
		Size:      request.Size,
		OwnerUUID: request.OwnerUUID,
		MimeType:  request.MimeType,
	})
	if err != nil {
		return nil, err
	}

	if err := uc.storage.Store(context.Background(), uuid, request.FileReader, request.Size); err != nil {
		_ = uc.filesRepository.Delete(uuid)

		return nil, err
	}

	return &Response{UUID: uuid}, nil
}
