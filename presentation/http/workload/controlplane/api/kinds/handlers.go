package kinds

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	actOnResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/actOnResource"
	admitResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	deleteResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	getKinds "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getKinds"
	getResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResource"
	getResources "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResources"
	queryResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/queryResource"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

// maxPayload is the most a request may carry: what it asks is sent on to a
// node, in a message held under NATS's own limit.
const maxPayload = noderequest.MaxReplyBytes

// body reads a request's body, no more of it than a node can be sent, and
// writes the refusal when there is more.
func body(rw http.ResponseWriter, r *http.Request) (json.RawMessage, bool) {
	payload, err := io.ReadAll(io.LimitReader(r.Body, maxPayload+1))
	if err != nil || len(payload) > maxPayload {
		respond.Refused(rw, domain.ValidationErrors{"payload": "too_large"})

		return nil, false
	}

	return payload, true
}

// wait reads how long a request asks to wait, and writes the refusal when it
// is not a wait.
func wait(rw http.ResponseWriter, r *http.Request) (time.Duration, bool) {
	asked, valid := waitOf(r)
	if !valid {
		respond.Refused(rw, domain.ValidationErrors{"wait": "invalid_value"})

		return 0, false
	}

	return asked, true
}

type indexHandler struct {
	useCase  *getResources.UseCase
	kindName string
}

func NewIndexHandler(useCase *getResources.UseCase, kindName string) *indexHandler {
	return &indexHandler{useCase: useCase, kindName: kindName}
}

// @Summary		List a kind's resources
// @Description	a page of a kind's manifests, newest first
// @Tags			workload resources
// @Produce		json
// @Param			plural	path		string	true	"The kind's plural"
// @Param			owner	query		string	false	"Only this person's own"
// @Param			parent	query		string	false	"Only those living in this resource"
// @Param			label	query		[]string	false	"Only those labelled so, as key=value; given more than once, every one of them"	collectionFormat(multi)
// @Param			page	query		int		false	"Page number"	default(1)
// @Success		200		{object}	getResources.Response
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/{plural} [get]
func (h *indexHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	labels, ok := labelsOf(r.URL.Query()["label"])
	if !ok {
		respond.Refused(rw, domain.ValidationErrors{"label": "invalid_value"})

		return
	}

	response, err := h.useCase.Execute(r.Context(), &getResources.Request{
		Kind:      h.kindName,
		OwnerUUID: respond.Owner(r),
		Parent:    r.URL.Query().Get("parent"),
		Labels:    labels,
		Page:      respond.Page(r),
	})
	if err != nil {
		failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, response)
}

type createHandler struct {
	useCase  *admitResource.UseCase
	kindName string
}

func NewCreateHandler(useCase *admitResource.UseCase, kindName string) *createHandler {
	return &createHandler{useCase: useCase, kindName: kindName}
}

// @Summary		Admit a resource
// @Description	admit a resource of a kind for its owner: its kind sets its defaults, checks it and places it, and it is sent the first command its kind asks for; with wait, the answer waits that long for what came of that command
// @Tags			workload resources
// @Accept			json
// @Produce		json
// @Param			plural	path		string		true	"The kind's plural"
// @Param			owner	query		string		true	"Whom the resource is for"
// @Param			parent	query		string		false	"The resource it is to live in"
// @Param			wait	query		string		false	"How long to wait for its first command's result: seconds, or a duration such as 90s"
// @Param			body	body		kind.Raw	true	"The manifest: its metadata's name, labels, owners and lifetime, and its spec"
// @Success		201		{object}	commanded
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/{plural} [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	duration, ok := wait(rw, r)
	if !ok {
		return
	}

	var manifest kind.Raw
	if !respond.Decode(rw, r, &manifest) {
		return
	}

	response, err := h.useCase.Execute(r.Context(), &admitResource.Request{
		Kind:      h.kindName,
		OwnerUUID: respond.Owner(r),
		Parent:    r.URL.Query().Get("parent"),
		Manifest:  manifest,
		Wait:      duration,
	})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	default:
		respondCommanded(rw, response.Resource, false, response.Command, response.Result, true)
	}
}

type showHandler struct {
	useCase  *getResource.UseCase
	kindName string
}

func NewShowHandler(useCase *getResource.UseCase, kindName string) *showHandler {
	return &showHandler{useCase: useCase, kindName: kindName}
}

// @Summary		Get a resource
// @Tags			workload resources
// @Produce		json
// @Param			plural	path		string	true	"The kind's plural"
// @Param			uuid	path		string	true	"Resource UUID"
// @Param			owner	query		string	false	"Only this owner's"
// @Success		200		{object}	kind.Raw
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/{plural}/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getResource.Request{Kind: h.kindName, OwnerUUID: respond.Owner(r), UUID: r.PathValue("uuid")})
	if err != nil {
		failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, response.Resource)
}

type deleteHandler struct {
	useCase  *deleteResource.UseCase
	kindName string
}

func NewDeleteHandler(useCase *deleteResource.UseCase, kindName string) *deleteHandler {
	return &deleteHandler{useCase: useCase, kindName: kindName}
}

// @Summary		Delete a resource
// @Description	desire a resource deleted: its delete is asked for at once when its state allows it, and once it can be otherwise; its record goes once its node says it is gone
// @Tags			workload resources
// @Produce		json
// @Param			plural	path		string	true	"The kind's plural"
// @Param			uuid	path		string	true	"Resource UUID"
// @Param			owner	query		string	false	"Only this owner's"
// @Param			wait	query		string	false	"How long to wait for its node to say it is gone"
// @Success		200		{object}	commanded
// @Success		202		{object}	commanded
// @Success		204
// @Failure		400		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/{plural}/{uuid} [delete]
func (h *deleteHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	duration, ok := wait(rw, r)
	if !ok {
		return
	}

	response, err := h.useCase.Execute(r.Context(), &deleteResource.Request{
		Kind:      h.kindName,
		OwnerUUID: respond.Owner(r),
		UUID:      r.PathValue("uuid"),
		Wait:      duration,
	})
	if err != nil {
		failed(rw, r, err)

		return
	}

	if len(response.ValidationErrors) > 0 {
		respond.Refused(rw, response.ValidationErrors)

		return
	}

	if response.Command == nil && !response.Gone {
		// expected deleted, and asked for once it can be.
		respond.JSON(rw, http.StatusAccepted, commanded{Resource: &response.Resource})

		return
	}

	respondCommanded(rw, response.Resource, response.Gone, response.Command, response.Result, false)
}

type actionHandler struct {
	useCase  *actOnResource.UseCase
	kindName string
}

func NewActionHandler(useCase *actOnResource.UseCase, kindName string) *actionHandler {
	return &actionHandler{useCase: useCase, kindName: kindName}
}

// @Summary		Ask a resource for a command
// @Description	ask a resource for one of its kind's commands, with its payload as the body: carried out in the control plane, or sent to the node holding it; with wait, the answer waits that long for what came of it
// @Tags			workload resources
// @Accept			json
// @Produce		json
// @Param			plural	path		string	true	"The kind's plural"
// @Param			uuid	path		string	true	"Resource UUID"
// @Param			action	path		string	true	"The command, such as start or stop"
// @Param			owner	query		string	false	"Only this owner's"
// @Param			wait	query		string	false	"How long to wait for what came of it"
// @Success		200		{object}	commanded
// @Success		202		{object}	commanded
// @Success		204
// @Failure		400		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Failure		409		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/{plural}/{uuid}/actions/{action} [post]
func (h *actionHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	duration, ok := wait(rw, r)
	if !ok {
		return
	}

	payload, ok := body(rw, r)
	if !ok {
		return
	}

	response, err := h.useCase.Execute(r.Context(), &actOnResource.Request{
		Kind:      h.kindName,
		OwnerUUID: respond.Owner(r),
		UUID:      r.PathValue("uuid"),
		Action:    r.PathValue("action"),
		Payload:   payload,
		Wait:      duration,
	})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	case response.NodeError != nil:
		respond.NodeRefused(rw, response.NodeError)
	default:
		respondCommanded(rw, response.Resource, response.Gone, response.Command, response.Result, false)
	}
}

type queryHandler struct {
	useCase    *queryResource.UseCase
	descriptor kind.Descriptor
}

func NewQueryHandler(useCase *queryResource.UseCase, descriptor kind.Descriptor) *queryHandler {
	return &queryHandler{useCase: useCase, descriptor: descriptor}
}

// answer is a query's answer, as the API shows it.
type answer struct {
	Result    json.RawMessage `json:"result,omitempty"`
	Truncated bool            `json:"truncated,omitempty"`
}

// @Summary		Ask a resource a query
// @Description	ask a resource one of its kind's queries, such as state or logs, its payload as parameters named as its fields are
// @Tags			workload resources
// @Produce		json
// @Param			plural	path		string	true	"The kind's plural"
// @Param			uuid	path		string	true	"Resource UUID"
// @Param			query	path		string	true	"The query, such as state or logs"
// @Param			owner	query		string	false	"Only this owner's"
// @Success		200		{object}	answer
// @Failure		400		{object}	map[string]interface{}
// @Failure		404		{object}	map[string]interface{}
// @Failure		409		{object}	map[string]interface{}
// @Failure		502		{object}	map[string]interface{}
// @Failure		504		{object}	map[string]interface{}
// @Router			/{plural}/{uuid}/{query} [get]
func (h *queryHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	name := r.PathValue("query")

	action, found := h.descriptor.Action(name)
	if !found || action.Mode != kind.ModeQuery {
		rw.WriteHeader(http.StatusNotFound)

		return
	}

	payload, err := payloadOf(r.URL.Query(), action.Payload.Type())
	if err != nil {
		respond.Refused(rw, domain.ValidationErrors{"payload": "invalid_value"})

		return
	}

	response, err := h.useCase.Execute(r.Context(), &queryResource.Request{
		Kind:      h.descriptor.Name,
		OwnerUUID: respond.Owner(r),
		UUID:      r.PathValue("uuid"),
		Action:    name,
		Payload:   payload,
	})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	case response.NodeError != nil:
		respond.NodeRefused(rw, response.NodeError)
	default:
		respond.JSON(rw, http.StatusOK, answer{Result: response.Result, Truncated: response.Truncated})
	}
}

type kindsHandler struct {
	useCase *getKinds.UseCase
}

func NewKindsHandler(useCase *getKinds.UseCase) *kindsHandler {
	return &kindsHandler{useCase: useCase}
}

// @Summary		List the kinds
// @Description	every kind the control plane runs, as it describes itself: its states, its actions, and the states each is allowed in
// @Tags			workload resources
// @Produce		json
// @Success		200	{object}	getKinds.Response
// @Router			/kinds [get]
func (h *kindsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context())
	if err != nil {
		failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, response)
}
