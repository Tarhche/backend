package user

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	"github.com/khanzadimahdi/testproject/application/auth"
	getusers "github.com/khanzadimahdi/testproject/application/dashboard/user/getUsers"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/permission"
)

type indexHandler struct {
	useCase    *getusers.UseCase
	authorizer domain.Authorizer
}

func NewIndexHandler(useCase *getusers.UseCase, a domain.Authorizer) *indexHandler {
	return &indexHandler{
		useCase:    useCase,
		authorizer: a,
	}
}

func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	currentUserUUID := auth.FromContext(r.Context()).UUID
	if ok, err := h.authorizer.Authorize(currentUserUUID, permission.UsersIndex); err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	} else if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	var page uint = 1
	if r.URL.Query().Has("page") {
		parsedPage, err := strconv.ParseUint(r.URL.Query().Get("page"), 10, int(unsafe.Sizeof(page)))
		if err == nil {
			page = uint(parsedPage)
		}
	}

	request := &getusers.Request{
		Page: page,
	}

	response, err := h.useCase.Execute(request)
	switch true {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
