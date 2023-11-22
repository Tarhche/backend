package file

import (
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"
	deletefile "github.com/khanzadimahdi/testproject.git/application/file/deleteFile"
	"github.com/khanzadimahdi/testproject.git/domain"
)

type deleteHandler struct {
	deleteFileUseCase *deletefile.UseCase
}

func NewDeleteHandler(deleteFileUseCase *deletefile.UseCase) *deleteHandler {
	return &deleteHandler{
		deleteFileUseCase: deleteFileUseCase,
	}
}

func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := httprouter.ParamsFromContext(r.Context()).ByName("uuid")

	err := h.deleteFileUseCase.DeleteFile(deletefile.Request{
		FileUUID: UUID,
	})

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
