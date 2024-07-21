package permission

import (
	"encoding/json"
	"github.com/khanzadimahdi/testproject/application/auth"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
	"net/http"

	getpermissions "github.com/khanzadimahdi/testproject/application/dashboard/permission/getPermissions"
)

type indexHandler struct {
	getPermissionsUseCase *getpermissions.UseCase
	authorizer            domain.Authorizer
}

func NewIndexHandler(getPermissionsUseCase *getpermissions.UseCase, a domain.Authorizer) *indexHandler {
	return &indexHandler{
		getPermissionsUseCase: getPermissionsUseCase,
		authorizer:            a,
	}
}

func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	userUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(userUUID, permission.PermissionsIndex); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	response, err := h.getPermissionsUseCase.GetPermissions()
	switch true {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
