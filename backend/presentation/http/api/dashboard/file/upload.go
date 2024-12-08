package file

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	uploadfile "github.com/khanzadimahdi/testproject/application/dashboard/file/uploadFile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type createHandler struct {
	useCase    *uploadfile.UseCase
	authorizer domain.Authorizer
}

func NewUploadHandler(useCase *uploadfile.UseCase, a domain.Authorizer) *createHandler {
	return &createHandler{
		useCase:    useCase,
		authorizer: a,
	}
}

func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(userUUID, permission.FilesCreate); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusForbidden)
		return
	}

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
