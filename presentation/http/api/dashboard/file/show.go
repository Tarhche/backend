package file

import (
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"
	getfile "github.com/khanzadimahdi/testproject/application/dashboard/file/getFile"
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
	UUID := httprouter.ParamsFromContext(r.Context()).ByName("uuid")

	err := h.showFileUseCase.GetFile(UUID, rw)

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
