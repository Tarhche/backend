package element

import (
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"
	deleteElement "github.com/khanzadimahdi/testproject/application/dashboard/element/deleteElement"
	"github.com/khanzadimahdi/testproject/domain"
)

type deleteHandler struct {
	deleteElementUseCase *deleteElement.UseCase
}

func NewDeleteHandler(deleteElementUseCase *deleteElement.UseCase) *deleteHandler {
	return &deleteHandler{
		deleteElementUseCase: deleteElementUseCase,
	}
}

func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := httprouter.ParamsFromContext(r.Context()).ByName("uuid")

	request := deleteElement.Request{
		ElementUUID: UUID,
	}

	err := h.deleteElementUseCase.DeleteElement(request)
	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
