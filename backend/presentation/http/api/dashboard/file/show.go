package file

import (
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	getfile "github.com/khanzadimahdi/testproject/application/dashboard/file/getFile"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type showHandler struct {
	showFileUseCase *getfile.UseCase
	authorizer      domain.Authorizer
}

func NewShowHandler(showFileUseCase *getfile.UseCase, a domain.Authorizer) *showHandler {
	return &showHandler{
		showFileUseCase: showFileUseCase,
		authorizer:      a,
	}
}

func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(userUUID, permission.FilesShow); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	UUID := r.PathValue("uuid")

	err := h.showFileUseCase.Execute(UUID, rw)

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
