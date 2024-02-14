package element

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"
	getElement "github.com/khanzadimahdi/testproject/application/dashboard/element/getElement"
	"github.com/khanzadimahdi/testproject/domain"
)

type showHandler struct {
	getElementUseCase *getElement.UseCase
}

func NewShowHandler(getElementUseCase *getElement.UseCase) *showHandler {
	return &showHandler{
		getElementUseCase: getElementUseCase,
	}
}

func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	UUID := httprouter.ParamsFromContext(r.Context()).ByName("uuid")

	response, err := h.getElementUseCase.GetElement(UUID)

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
