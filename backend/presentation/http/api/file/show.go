package file

import (
	"errors"
	"net/http"

	getfile "github.com/khanzadimahdi/testproject/application/file/getFile"
	"github.com/khanzadimahdi/testproject/domain"
)

type showHandler struct {
	useCase *getfile.UseCase
}

func NewShowHandler(useCase *getfile.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := r.PathValue("uuid")

	response, err := h.useCase.Execute(UUID)

	switch {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		http.ServeContent(rw, r, response.Name, response.CreatedAt, response.Reader)
	}
}
