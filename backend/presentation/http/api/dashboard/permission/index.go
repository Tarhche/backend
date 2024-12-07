package permission

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"

	getpermissions "github.com/khanzadimahdi/testproject/application/dashboard/permission/getPermissions"
)

type indexHandler struct {
	useCase    *getpermissions.UseCase
	authorizer domain.Authorizer
}

func NewIndexHandler(useCase *getpermissions.UseCase, a domain.Authorizer) *indexHandler {
	return &indexHandler{
		useCase:    useCase,
		authorizer: a,
	}
}

func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(userUUID, permission.PermissionsIndex); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusForbidden)
		return
	}

	response, err := h.useCase.Execute()
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
