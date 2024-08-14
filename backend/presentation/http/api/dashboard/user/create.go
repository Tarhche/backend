package user

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	createuser "github.com/khanzadimahdi/testproject/application/dashboard/user/createUser"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type createHandler struct {
	createArticleUseCase *createuser.UseCase
	authorizer           domain.Authorizer
}

func NewCreateHandler(createArticleUseCase *createuser.UseCase, a domain.Authorizer) *createHandler {
	return &createHandler{
		createArticleUseCase: createArticleUseCase,
		authorizer:           a,
	}
}

func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	currentUserUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(currentUserUUID, permission.UsersCreate); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	var request createuser.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.createArticleUseCase.Execute(request)

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
