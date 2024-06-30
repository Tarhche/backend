package user

import (
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"
	deleteuser "github.com/khanzadimahdi/testproject/application/dashboard/user/deleteUser"
	"github.com/khanzadimahdi/testproject/domain"
)

type deleteHandler struct {
	deleteArticleUseCase *deleteuser.UseCase
}

func NewDeleteHandler(deleteArticleUseCase *deleteuser.UseCase) *deleteHandler {
	return &deleteHandler{
		deleteArticleUseCase: deleteArticleUseCase,
	}
}

func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := httprouter.ParamsFromContext(r.Context()).ByName("uuid")

	request := deleteuser.Request{
		UserUUID: UUID,
	}

	err := h.deleteArticleUseCase.DeleteUser(request)
	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
