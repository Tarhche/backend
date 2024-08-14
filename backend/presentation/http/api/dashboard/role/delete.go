package role

import (
	"errors"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/khanzadimahdi/testproject/application/auth"
	deleterole "github.com/khanzadimahdi/testproject/application/dashboard/role/deleteRole"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type deleteHandler struct {
	deleteRoleUseCase *deleterole.UseCase
	authorizer        domain.Authorizer
}

func NewDeleteHandler(deleteRoleUseCase *deleterole.UseCase, a domain.Authorizer) *deleteHandler {
	return &deleteHandler{
		deleteRoleUseCase: deleteRoleUseCase,
		authorizer:        a,
	}
}

func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(userUUID, permission.RolesDelete); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	UUID := httprouter.ParamsFromContext(r.Context()).ByName("uuid")

	request := deleterole.Request{
		RoleUUID: UUID,
	}

	err := h.deleteRoleUseCase.Execute(request)
	switch true {
	case errors.Is(err, domain.ErrNotExists):
		rw.WriteHeader(http.StatusNotFound)
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.WriteHeader(http.StatusOK)
	}
}
