package comment

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	"github.com/khanzadimahdi/testproject/application/comment/getComments"
)

type indexHandler struct {
	getCommentsUseCase *getComments.UseCase
}

func NewIndexHandler(getCommentsUseCase *getComments.UseCase) *indexHandler {
	return &indexHandler{
		getCommentsUseCase: getCommentsUseCase,
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

	var objectUUID string
	if r.URL.Query().Has("object_uuid") {
		objectUUID = r.URL.Query().Get("object_uuid")
	}

	var objectType string
	if r.URL.Query().Has("object_type") {
		objectType = r.URL.Query().Get("object_type")
	}

	request := &getComments.Request{
		Page:       page,
		ObjectUUID: objectUUID,
		ObjectType: objectType,
	}

	response, err := h.getCommentsUseCase.Execute(request)
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
