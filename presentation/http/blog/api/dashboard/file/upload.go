package file

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	uploadfile "github.com/khanzadimahdi/testproject/application/dashboard/file/uploadFile"
)

type createHandler struct {
	useCase *uploadfile.UseCase
}

func NewUploadHandler(useCase *uploadfile.UseCase) *createHandler {
	return &createHandler{
		useCase: useCase,
	}
}

// @Summary		Upload file
// @Description	upload a file via multipart form
// @Tags			dashboard files
// @Accept			multipart/form-data
// @Produce		json
// @Param			file	formData	file	true	"File to upload"
// @Success		201		{object}	uploadfile.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/dashboard/files [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(uploadfile.MaxFileSize); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return
	}
	defer file.Close()

	mimetype, err := detectMimeType(file)
	if err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.useCase.Execute(&uploadfile.Request{
		Name:       header.Filename,
		OwnerUUID:  auth.FromContext(r.Context()).UUID,
		Size:       header.Size,
		FileReader: file,
		MimeType:   mimetype,
	})

	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		json.NewEncoder(rw).Encode(response)
	}
}

func detectMimeType(file io.ReadSeeker) (string, error) {
	buffer := make([]byte, 512)

	if _, err := file.Read(buffer); err != nil {
		return "", err
	}

	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}

	return http.DetectContentType(buffer), nil
}
