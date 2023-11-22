package file

import (
	"encoding/json"
	"log"
	"net/http"

	uploadfile "github.com/khanzadimahdi/testproject.git/application/file/uploadFile"
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

	validationErrors, err := h.uploadFileUseCase.UploadFile(uploadfile.Request{
		Name:       header.Filename,
		UserUUID:   "018bd8ce-d886-777d-822d-e8ceeab26aff",
		Size:       header.Size,
		FileReader: file,
	})

	log.Println(validationErrors)

	switch true {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(validationErrors.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(validationErrors)
	default:
		rw.WriteHeader(http.StatusCreated)
	}
}
