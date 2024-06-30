package user

import (
	"encoding/json"
	"errors"
	"net/http"

	createuser "github.com/khanzadimahdi/testproject/application/dashboard/user/createUser"
	"github.com/khanzadimahdi/testproject/domain"
)

type createHandler struct {
	createArticleUseCase *createuser.UseCase
}

func NewCreateHandler(createArticleUseCase *createuser.UseCase) *createHandler {
	return &createHandler{
		createArticleUseCase: createArticleUseCase,
	}
}

func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request createuser.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.createArticleUseCase.CreateUser(request)

	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusCreated)
		json.NewEncoder(rw).Encode(response)
	}
}
