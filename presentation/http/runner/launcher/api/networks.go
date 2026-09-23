package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/khanzadimahdi/testproject/application/runner/launcher/ensureNetwork"
	"github.com/khanzadimahdi/testproject/application/runner/launcher/removeNetwork"
	"github.com/khanzadimahdi/testproject/domain/runner/machine"
)

type ensureNetworkHandler struct {
	useCase *ensureNetwork.UseCase
}

func NewEnsureNetworkHandler(useCase *ensureNetwork.UseCase) *ensureNetworkHandler {
	return &ensureNetworkHandler{useCase: useCase}
}

// ensureNetworkBody is what a network is asked for with, beside its names.
type ensureNetworkBody struct {
	Masquerade bool `json:"masquerade"`
}

// @Summary		Ensure a network
// @Description	makes one of an orchestrator's networks, if it is not there already, and says what it is
// @Tags			runner launcher
// @Accept			json
// @Produce		json
// @Param			owner	path		string				true	"Orchestrator"
// @Param			name	path		string				true	"Network"
// @Param			body	body		ensureNetworkBody	true	"Whether it routes out"
// @Success		200		{object}	ensureNetwork.Response
// @Failure		400		{object}	ensureNetwork.Response
// @Failure		500		{string}	string	"why it could not be made"
// @Router			/networks/{owner}/{name} [put]
func (h *ensureNetworkHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	var body ensureNetworkBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		rw.WriteHeader(http.StatusBadRequest)

		return
	}

	request := ensureNetwork.Request{
		Owner:      r.PathValue("owner"),
		Name:       r.PathValue("name"),
		Masquerade: body.Masquerade,
	}

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		reply(rw, http.StatusBadRequest, response)
	default:
		reply(rw, http.StatusOK, response)
	}
}

type removeNetworkHandler struct {
	useCase *removeNetwork.UseCase
}

func NewRemoveNetworkHandler(useCase *removeNetwork.UseCase) *removeNetworkHandler {
	return &removeNetworkHandler{useCase: useCase}
}

// @Summary		Remove a network
// @Description	takes one of an orchestrator's networks away, once nothing is plugged into it
// @Tags			runner launcher
// @Param			owner	path	string	true	"Orchestrator"
// @Param			name	path	string	true	"Network"
// @Success		204
// @Failure		400	{object}	removeNetwork.Response
// @Failure		409	{string}	string	"something is still plugged into it"
// @Failure		500	{string}	string	"why it could not be removed"
// @Router			/networks/{owner}/{name} [delete]
func (h *removeNetworkHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	request := removeNetwork.Request{
		Owner: r.PathValue("owner"),
		Name:  r.PathValue("name"),
	}

	response, err := h.useCase.Execute(r.Context(), &request)

	switch {
	case errors.Is(err, machine.ErrNetworkInUse):
		http.Error(rw, err.Error(), http.StatusConflict)
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		reply(rw, http.StatusBadRequest, response)
	default:
		rw.WriteHeader(http.StatusNoContent)
	}
}
