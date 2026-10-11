package kinds

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	actOnResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/actOnResource"
	admitResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	deleteResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	getDependents "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getDependents"
	getKinds "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getKinds"
	getPlan "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getPlan"
	getResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResource"
	getResources "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getResources"
	planResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/planResource"
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

// flag reads a parameter that says yes or no, and writes the refusal when it
// says neither.
func flag(rw http.ResponseWriter, r *http.Request, name string) (bool, bool) {
	yes, valid := flagOf(r, name)
	if !valid {
		respond.Refused(rw, domain.ValidationErrors{name: "invalid_value"})

		return false, false
	}

	return yes, true
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
	plan     *planResource.UseCase
	kindName string
}

// NewCreateHandler admits a kind's resources through useCase, and says what
// admitting one would do through plan, which may be nothing: a dry run is
// refused then.
func NewCreateHandler(useCase *admitResource.UseCase, plan *planResource.UseCase, kindName string) *createHandler {
	return &createHandler{useCase: useCase, plan: plan, kindName: kindName}
}

// @Summary		Admit a resource
// @Description	admit a resource of a kind for its owner: its kind sets its defaults, checks it and places it, and it is sent the first command its kind asks for; with wait, the answer waits that long for what came of that command. With dry_run, nothing is kept or sent: the answer is what would be admitted, and, of a root of a graph, what carrying out its graph would do, level by level
// @Tags			workload resources
// @Accept			json
// @Produce		json
// @Param			plural	path		string		true	"The kind's plural"
// @Param			owner	query		string		true	"Whom the resource is for"
// @Param			parent	query		string		false	"The resource it is to live in"
// @Param			wait	query		string		false	"How long to wait for its first command's result: seconds, or a duration such as 90s"
// @Param			dry_run	query		bool		false	"Say what would be admitted, and keep nothing"
// @Param			body	body		kind.Raw	true	"The manifest: its metadata's name, labels, owners and lifetime, and its spec"
// @Success		200		{object}	previewedAdmission
// @Success		201		{object}	commanded
// @Failure		400		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/{plural} [post]
func (h *createHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	duration, ok := wait(rw, r)
	if !ok {
		return
	}

	dryRun, ok := flag(rw, r, "dry_run")
	if !ok {
		return
	}

	var manifest kind.Raw
	if !respond.Decode(rw, r, &manifest) {
		return
	}

	if dryRun {
		h.dryRun(rw, r, manifest)

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

// dryRun says what admitting manifest would do, and does nothing: a route
// that serves no dry run refuses one.
func (h *createHandler) dryRun(rw http.ResponseWriter, r *http.Request, manifest kind.Raw) {
	if h.plan == nil {
		respond.Refused(rw, domain.ValidationErrors{"dry_run": "invalid_value"})

		return
	}

	response, err := h.plan.Execute(r.Context(), &planResource.Request{
		Kind:      h.kindName,
		OwnerUUID: respond.Owner(r),
		Parent:    r.URL.Query().Get("parent"),
		Manifest:  manifest,
	})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0:
		respond.Refused(rw, response.ValidationErrors)
	default:
		respond.JSON(rw, http.StatusOK, previewedAdmission{Resource: &response.Resource, Plan: response.Plan})
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
// @Param			uuid	path		string	true	"Resource UUID, or what else names it inside its parent"
// @Param			owner	query		string	false	"Only this owner's"
// @Param			parent	query		string	false	"The resource it lives in: inside it, a kind may name it by more than its uuid"
// @Success		200		{object}	kind.Raw
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/{plural}/{uuid} [get]
func (h *showHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getResource.Request{
		Kind:      h.kindName,
		OwnerUUID: respond.Owner(r),
		UUID:      r.PathValue("uuid"),
		Parent:    r.URL.Query().Get("parent"),
	})
	if err != nil {
		failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, response.Resource)
}

type deleteHandler struct {
	useCase  *deleteResource.UseCase
	plan     *planResource.UseCase
	kindName string
}

// NewDeleteHandler deletes a kind's resources through useCase, and says what
// deleting one would do through plan, which may be nothing: a dry run is
// refused then.
func NewDeleteHandler(useCase *deleteResource.UseCase, plan *planResource.UseCase, kindName string) *deleteHandler {
	return &deleteHandler{useCase: useCase, plan: plan, kindName: kindName}
}

// @Summary		Delete a resource
// @Description	desire a resource deleted: its delete is asked for at once when its state allows it, and once it can be otherwise; its record goes once its node says it is gone. What the graphs of roots need is answered with those roots, and whether one of them makes it again, always as 202; with cascade, they are marked to go first, and those that do not live in it are waited on to go before it is asked
// @Tags			workload resources
// @Produce		json
// @Param			plural	path		string	true	"The kind's plural"
// @Param			uuid	path		string	true	"Resource UUID, or what else names it inside its parent"
// @Param			owner	query		string	false	"Only this owner's"
// @Param			parent	query		string	false	"The resource it lives in: inside it, a kind may name it by more than its uuid"
// @Param			wait	query		string	false	"How long to wait for its node to say it is gone"
// @Param			cascade	query		bool	false	"Delete the roots whose graphs need it with it"
// @Param			dry_run	query		bool	false	"Say what deleting it would do, and do nothing: of a root, what removing it would remove, unlink and keep; of anything else, the roots that need it, and, with cascade, what removing each would do"
// @Success		200		{object}	previewedNeeded
// @Success		202		{object}	deleted
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

	cascade, ok := flag(rw, r, "cascade")
	if !ok {
		return
	}

	dryRun, ok := flag(rw, r, "dry_run")
	if !ok {
		return
	}

	if dryRun {
		h.dryRun(rw, r, cascade)

		return
	}

	response, err := h.useCase.Execute(r.Context(), &deleteResource.Request{
		Kind:      h.kindName,
		OwnerUUID: respond.Owner(r),
		UUID:      r.PathValue("uuid"),
		Parent:    r.URL.Query().Get("parent"),
		Wait:      duration,
		Cascade:   cascade,
	})
	if err != nil {
		failed(rw, r, err)

		return
	}

	if len(response.ValidationErrors) > 0 {
		refusedWith(rw, response.ValidationErrors, nil, response.Dependents, response.Recreated)

		return
	}

	if len(response.Dependents) > 0 {
		respondDeleted(rw, response.Resource, response.Gone, response.Command, response.Result, response.Dependents, response.Recreated)

		return
	}

	if response.Command == nil && !response.Gone {
		// expected deleted, and asked for once it can be.
		respond.JSON(rw, http.StatusAccepted, commanded{Resource: &response.Resource})

		return
	}

	respondCommanded(rw, response.Resource, response.Gone, response.Command, response.Result, false)
}

// dryRun says what deleting the resource the request names would do, with
// the roots that need it when cascade says so, and does nothing: a route that
// serves no dry run refuses one.
func (h *deleteHandler) dryRun(rw http.ResponseWriter, r *http.Request, cascade bool) {
	if h.plan == nil {
		respond.Refused(rw, domain.ValidationErrors{"dry_run": "invalid_value"})

		return
	}

	response, err := h.plan.Execute(r.Context(), &planResource.Request{
		Kind:      h.kindName,
		OwnerUUID: respond.Owner(r),
		Parent:    r.URL.Query().Get("parent"),
		Delete:    true,
		UUID:      r.PathValue("uuid"),
		Cascade:   cascade,
	})
	if err != nil {
		failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, previewedDeleteOf(response))
}

type actionHandler struct {
	useCase  *actOnResource.UseCase
	kindName string
}

func NewActionHandler(useCase *actOnResource.UseCase, kindName string) *actionHandler {
	return &actionHandler{useCase: useCase, kindName: kindName}
}

// @Summary		Ask a resource for a command
// @Description	ask a resource for one of its kind's commands, with its payload as the body: carried out in the control plane, or sent to the node holding it; with wait, the answer waits that long for what came of it. A command that deletes what the graphs of roots need is answered as a delete is
// @Tags			workload resources
// @Accept			json
// @Produce		json
// @Param			plural	path		string	true	"The kind's plural"
// @Param			uuid	path		string	true	"Resource UUID, or what else names it inside its parent"
// @Param			action	path		string	true	"The command, such as start or stop"
// @Param			owner	query		string	false	"Only this owner's"
// @Param			parent	query		string	false	"The resource it lives in: inside it, a kind may name it by more than its uuid"
// @Param			wait	query		string	false	"How long to wait for what came of it"
// @Param			cascade	query		bool	false	"For a command that deletes it, delete the roots whose graphs need it with it"
// @Success		200		{object}	commanded
// @Success		202		{object}	deleted
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

	cascade, ok := flag(rw, r, "cascade")
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
		Parent:    r.URL.Query().Get("parent"),
		Action:    r.PathValue("action"),
		Payload:   payload,
		Wait:      duration,
		Cascade:   cascade,
	})

	switch {
	case err != nil:
		failed(rw, r, err)
	case len(response.ValidationErrors) > 0, response.NodeError != nil:
		refusedWith(rw, response.ValidationErrors, response.NodeError, response.Dependents, response.Recreated)
	case len(response.Dependents) > 0:
		respondDeleted(rw, response.Resource, response.Gone, response.Command, response.Result, response.Dependents, response.Recreated)
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
// @Param			uuid	path		string	true	"Resource UUID, or what else names it inside its parent"
// @Param			query	path		string	true	"The query, such as state or logs"
// @Param			owner	query		string	false	"Only this owner's"
// @Param			parent	query		string	false	"The resource it lives in: inside it, a kind may name it by more than its uuid"
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
		Parent:    r.URL.Query().Get("parent"),
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

type planHandler struct {
	useCase  *getPlan.UseCase
	kindName string
}

func NewPlanHandler(useCase *getPlan.UseCase, kindName string) *planHandler {
	return &planHandler{useCase: useCase, kindName: kindName}
}

// @Summary		How a root's plan went
// @Description	how the plan of a root of a graph went, the last time it was carried out: each node's level, kind, action, state, reason, record and times, and its lease's failures, next try and reason. What is no root is not there
// @Tags			workload resources
// @Produce		json
// @Param			plural	path		string	true	"The kind's plural"
// @Param			uuid	path		string	true	"Resource UUID, or what else names it inside its parent"
// @Param			owner	query		string	false	"Only this owner's"
// @Param			parent	query		string	false	"The resource it lives in: inside it, a kind may name it by more than its uuid"
// @Success		200		{object}	planShown
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/{plural}/{uuid}/plan [get]
func (h *planHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getPlan.Request{
		Kind:      h.kindName,
		OwnerUUID: respond.Owner(r),
		UUID:      r.PathValue("uuid"),
		Parent:    r.URL.Query().Get("parent"),
	})
	if err != nil {
		failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, planShownOf(response))
}

type dependentsHandler struct {
	useCase  *getDependents.UseCase
	kindName string
}

func NewDependentsHandler(useCase *getDependents.UseCase, kindName string) *dependentsHandler {
	return &dependentsHandler{useCase: useCase, kindName: kindName}
}

// @Summary		What needs a resource
// @Description	the roots whose graphs need a resource, as its delete would be answered with them now, each with whether it makes it again once it is gone, and whether one of them does: none for a root, and none for what nobody keeps a record of. Nothing is changed, marked or asked of any node: it is a question, which a delete's answer would also say
// @Tags			workload resources
// @Produce		json
// @Param			plural	path		string	true	"The kind's plural"
// @Param			uuid	path		string	true	"Resource UUID, or what else names it inside its parent"
// @Param			owner	query		string	false	"Only this owner's"
// @Param			parent	query		string	false	"The resource it lives in: inside it, a kind may name it by more than its uuid"
// @Success		200		{object}	dependentsShown
// @Failure		404		{object}	map[string]interface{}
// @Failure		500		{object}	map[string]interface{}
// @Router			/{plural}/{uuid}/dependents [get]
func (h *dependentsHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	response, err := h.useCase.Execute(r.Context(), &getDependents.Request{
		Kind:      h.kindName,
		OwnerUUID: respond.Owner(r),
		UUID:      r.PathValue("uuid"),
		Parent:    r.URL.Query().Get("parent"),
	})
	if err != nil {
		failed(rw, r, err)

		return
	}

	respond.JSON(rw, http.StatusOK, dependentsShown{Dependents: dependentsOf(response.Dependents), Recreated: response.Recreated})
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
