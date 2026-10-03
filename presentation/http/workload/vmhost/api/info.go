package api

import (
	"net/http"

	"github.com/khanzadimahdi/testproject/application/workload/vmhost/getInfo"
)

type infoHandler struct {
	useCase *getInfo.UseCase
}

func NewInfoHandler(useCase *getInfo.UseCase) *infoHandler {
	return &infoHandler{useCase: useCase}
}

// ServeHTTP says what vmhost is: vm.Info. It is asked on every node
// heartbeat, and by vmhost's container's healthcheck.
func (h *infoHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context())
	if err != nil {
		failed(rw, r, err)

		return
	}

	reply(rw, http.StatusOK, response.Info)
}
