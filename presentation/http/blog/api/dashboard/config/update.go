package config

import (
	"encoding/json"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/dashboard/config/updateConfig"
)

type updateHandler struct {
	useCase *updateConfig.UseCase
}

func NewUpdateHandler(useCase *updateConfig.UseCase) *updateHandler {
	return &updateHandler{
		useCase: useCase,
	}
}

// @Summary		Update config
// @Description	modify dashboard configuration values
// @Tags			dashboard config
// @Accept			json
// @Produce		json
// @Param			body	body	updateConfig.Request	true	"Config update"
// @Success		204
// @Failure		400	{object}	map[string]interface{}
// @Failure		500	{object}	map[string]interface{}
// @Router			/dashboard/config [put]
func (h *updateHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var request updateConfig.Request
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rw.WriteHeader(http.StatusBadRequest)
		return
	}

	response, err := h.useCase.Execute(&request)
	switch {
	case err != nil:
		rw.WriteHeader(http.StatusInternalServerError)
	case response != nil && len(response.ValidationErrors) > 0:
		rw.Header().Add("Content-Type", "application/json")
		rw.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(rw).Encode(response)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
