package file

import (
	"encoding/json"
	"net/http"
	"strconv"
	"unsafe"

	getfiles "github.com/khanzadimahdi/testproject/application/dashboard/file/getFiles"
)

type indexHandler struct {
	getFilesUseCase *getfiles.UseCase
}

func NewIndexHandler(getFilesUseCase *getfiles.UseCase) *indexHandler {
	return &indexHandler{
		getFilesUseCase: getFilesUseCase,
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

	request := &getfiles.Request{
		Page: page,
	}

	response, err := h.getFilesUseCase.GetFiles(request)
	switch true {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	default:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		json.NewEncoder(rw).Encode(response)
	}
}
