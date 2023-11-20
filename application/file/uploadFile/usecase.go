package createfile

import (
	"github.com/khanzadimahdi/testproject.git/domain/file"
)

type UseCase struct {
	filesRepository file.Repository
}

func NewUseCase(filesRepository file.Repository) *UseCase {
	return &UseCase{
		filesRepository: filesRepository,
	}
}

func (uc *UseCase) UploadFile(request Request) (*UploadFileResponse, error) {
	if ok, validation := request.Validate(); !ok {
		return &UploadFileResponse{
			ValidationErrors: validation,
		}, nil
	}

	file := file.File{
		UUID:      "",
		Name:      request.Name,
		Size:      100,
		OwnerUUID: request.UserUUID,
	}

	return &UploadFileResponse{}, uc.filesRepository.Save(&file)
}
