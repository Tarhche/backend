package config

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/config/getConfig"
)

type showHandler struct {
	useCase *getConfig.UseCase
}

func NewShowHandler(useCase *getConfig.UseCase) *showHandler {
	return &showHandler{
		useCase: useCase,
	}
}

// @Summary		Get config
// @Description	fetch dashboard configuration
// @Tags			dashboard config
// @Accept			json
// @Produce		json
// @Success		200	{object}	getConfig.Response
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/config [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
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
