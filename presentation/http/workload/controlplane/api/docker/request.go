// Package docker is the control plane's passthrough to a Docker VM's dockerd.
package docker

import (
	"encoding/json"
	"io"
	"net/http"

	requestdocker "github.com/khanzadimahdi/testproject/application/workload/controlplane/docker/requestDocker"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

// maxPayload is the most a request for dockerd may carry: a node request is
// held under NATS's own limit.
const maxPayload = noderequest.MaxReplyBytes

type requestHandler struct {
	useCase *requestdocker.UseCase
}

func NewRequestHandler(useCase *requestdocker.UseCase) *requestHandler {
	return &requestHandler{useCase: useCase}
}

// result is dockerd's answer, as the node gave it.
type result struct {
	Result    json.RawMessage `json:"result,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
}

// @Summary		Ask a docker vm's dockerd
// @Description	pass a question for a docker vm's dockerd on to the node holding it; op is the operation without its docker. prefix, such as containers.list
// @Tags			workload docker
// @Accept			json
// @Produce		json
// @Param			uuid	path		string	true	"VM UUID"
// @Param			op		path		string	true	"Operation, such as containers.list or images.pull"
// @Param			owner	query		string	false	"Only this owner's vm"
// @Success		200		{object}	map[string]interface{}
// @Failure		400		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Failure		409		{object}	map[string]interface{}
// @Failure		422		{object}	map[string]interface{}
// @Failure		502		{object}	map[string]interface{}
// @Failure		504		{object}	map[string]interface{}
// @Router			/vms/{uuid}/docker/{op} [post]
func (h *requestHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, maxPayload+1))
	if err != nil || len(payload) > maxPayload {
		respond.Refused(rw, domain.ValidationErrors{"payload": "too_large"})

		return
	}

	response, err := h.useCase.Execute(r.Context(), &requestdocker.Request{
		OwnerUUID: respond.Owner(r),
		VMUUID:    r.PathValue("uuid"),
		Op:        noderequest.Op("docker." + r.PathValue("op")),
		Payload:   payload,
	})

	switch {
	case err != nil:
		respond.Failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	case response.NodeError != nil:
		respond.NodeRefused(rw, response.NodeError)
	default:
		respond.JSON(rw, http.StatusOK, result{Result: response.Result, Truncated: response.Truncated})
	}
}
