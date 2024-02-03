package element

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	getElements "github.com/khanzadimahdi/testproject/application/dashboard/element/getElements"
)

type indexHandler struct {
	getElementsUseCase *getElements.UseCase
}

func NewIndexHandler(getElementsUseCase *getElements.UseCase) *indexHandler {
	return &indexHandler{
		getElementsUseCase: getElementsUseCase,
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

	request := &getElements.Request{
		Page: page,
	}

	response, err := h.getElementsUseCase.GetElements(request)
	switch true {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
