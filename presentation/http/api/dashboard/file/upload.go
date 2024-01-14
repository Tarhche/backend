package file

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	uploadfile "github.com/khanzadimahdi/testproject/application/dashboard/file/uploadFile"
)

type createHandler struct {
	uploadFileUseCase *uploadfile.UseCase
}

func NewUploadHandler(uploadFileUseCase *uploadfile.UseCase) *createHandler {
	return &createHandler{
		uploadFileUseCase: uploadFileUseCase,
	}
}

func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var max int64 = 5 << 20 // 5MB

	if err := r.ParseMultipartForm(max); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return
	}
	defer file.Close()

	response, err := h.uploadFileUseCase.UploadFile(uploadfile.Request{
		Name:       header.Filename,
		OwnerUUID:  auth.FromContext(r.Context()).UUID,
		Size:       header.Size,
		FileReader: file,
	})

	switch true {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusCreated)
		json.NewEncoder(rw).Encode(response)
	}
}
