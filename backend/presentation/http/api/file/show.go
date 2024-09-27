package file

import (
	"bytes"
	"errors"
	"net/http"

	getfile "github.com/khanzadimahdi/testproject/application/file/getFile"
	"github.com/khanzadimahdi/testproject/domain"
)

type showHandler struct {
	showFileUseCase *getfile.UseCase
}

func NewShowHandler(showFileUseCase *getfile.UseCase) *showHandler {
	return &showHandler{
		showFileUseCase: showFileUseCase,
	}
}

func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	var buffer bytes.Buffer
	err := h.showFileUseCase.Execute(UUID, &buffer)

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusOK)
		buffer.WriteTo(rw)
	}
}
