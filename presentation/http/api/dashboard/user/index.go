package user

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	getusers "github.com/khanzadimahdi/testproject/application/dashboard/user/getUsers"
)

type indexHandler struct {
	useCase *getusers.UseCase
}

func NewIndexHandler(useCase *getusers.UseCase) *indexHandler {
	return &indexHandler{
		useCase: useCase,
	}
}

func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
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
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
