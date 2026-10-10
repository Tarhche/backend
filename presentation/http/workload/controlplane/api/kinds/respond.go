package kinds

import (
	"encoding"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/graph/execute"
	getPlan "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/getPlan"
	planResource "github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/planResource"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/presentation/http/workload/controlplane/api/respond"
)

// MaxWait is the longest a request waits for what came of a command: what
// the slowest command a node carries out may take, a Docker VM's dockerd
// coming up and then an image pulled.
const MaxWait = 15 * time.Minute

// commanded is what asking for a command came to, as the API shows it: the
// resource as it is now, the command its node was sent, and what came of it,
// when it was waited for and came in time.
type commanded struct {
	Resource *kind.Raw `json:"resource,omitempty"`
	Command  *command  `json:"command,omitempty"`
	Result   *result   `json:"result,omitempty"`
}

type command struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	Node    string `json:"node"`
	Attempt int    `json:"attempt"`
}

type result struct {
	ID     string    `json:"id"`
	Action string    `json:"action"`
	OK     bool      `json:"ok"`
	Reason string    `json:"reason,omitempty"`
	Output string    `json:"output,omitempty"`
	At     time.Time `json:"at,omitzero"`
}

// dependent is a root whose graph needs what was asked to go, as the API
// shows it: whether its graph makes it again, or only waits on it.
type dependent struct {
	Kind      string `json:"kind"`
	UUID      string `json:"uuid"`
	Name      string `json:"name,omitempty"`
	Recreates bool   `json:"recreates"`
}

// dependentsOf are dependents as the API shows them.
func dependentsOf(dependents []graph.Dependent) []dependent {
	shown := make([]dependent, len(dependents))
	for i, d := range dependents {
		shown[i] = dependent{Kind: d.Root.Kind, UUID: d.Root.UUID, Name: d.Name, Recreates: d.Recreates}
	}

	return shown
}

// deleted is what asking for what roots need to go came to, as the API shows
// it: what asking came to, the roots that need it, and whether one of them
// makes it again.
type deleted struct {
	commanded

	Dependents []dependent `json:"dependents"`
	Recreated  bool        `json:"recreated"`
}

// previewedAdmission is what a dry run says admitting a resource would do:
// what would be admitted, and, of a root, what carrying out its graph would.
type previewedAdmission struct {
	Resource *kind.Raw        `json:"resource"`
	Plan     *execute.Preview `json:"plan,omitempty"`
}

// previewedDelete is what a dry run says deleting a root would do: what
// removing it would remove, unlink and keep.
type previewedDelete struct {
	Plan *execute.Preview `json:"plan"`
}

// previewedNeeded is what a dry run says deleting what is not a root would
// do: the roots that need it, none among them, each with what removing it
// would do when it would go with it, and whether one of them would make it
// again.
type previewedNeeded struct {
	Dependents []previewedDependent `json:"dependents"`
	Recreated  bool                 `json:"recreated"`
}

// previewedDependent is a root that needs what would be deleted, and what
// removing it would do, when it would go with it.
type previewedDependent struct {
	dependent

	Plan *execute.Preview `json:"plan,omitempty"`
}

// previewedDeleteOf is what a dry run of a delete came to, as the API shows
// it: a root's plan, or the roots that need what is not one.
func previewedDeleteOf(response *planResource.Response) any {
	if response.Plan != nil {
		return previewedDelete{Plan: response.Plan}
	}

	shown := previewedNeeded{Dependents: make([]previewedDependent, len(response.Dependents)), Recreated: response.Recreated}

	for i, d := range response.Dependents {
		shown.Dependents[i] = previewedDependent{dependent: dependentsOf([]graph.Dependent{d.Dependent})[0], Plan: d.Plan}
	}

	return shown
}

// planShown is how a root's plan went, as the API shows it: each node, and
// what its lease keeps of its failures, while it has one.
type planShown struct {
	Root       kind.Reference `json:"root"`
	Generation int64          `json:"generation"`
	Nodes      []nodeShown    `json:"nodes"`
	Lease      *leaseShown    `json:"lease,omitempty"`
}

// nodeShown is how one node of a root's plan went.
type nodeShown struct {
	Node       graph.NodeID    `json:"node"`
	Level      int             `json:"level"`
	Kind       string          `json:"kind"`
	Action     graph.Action    `json:"action"`
	State      graph.State     `json:"state"`
	Reason     string          `json:"reason,omitempty"`
	Ref        *kind.Reference `json:"ref,omitempty"`
	StartedAt  time.Time       `json:"started_at,omitzero"`
	FinishedAt time.Time       `json:"finished_at,omitzero"`
}

// leaseShown is what a root's lease keeps of how its plan went lately: its
// failures in a row, when it is carried out again, and why it failed.
type leaseShown struct {
	Failures int       `json:"failures"`
	NextAt   time.Time `json:"next_at,omitzero"`
	Reason   string    `json:"reason,omitempty"`
}

// planShownOf is how a root's plan went, as the API shows it.
func planShownOf(response *getPlan.Response) planShown {
	shown := planShown{Root: response.Root, Generation: response.Generation, Nodes: make([]nodeShown, len(response.Executions))}

	for i, e := range response.Executions {
		shown.Nodes[i] = nodeShown{
			Node:       e.Node,
			Level:      e.Level,
			Kind:       e.Kind,
			Action:     e.Action,
			State:      e.State,
			Reason:     e.Reason,
			StartedAt:  e.StartedAt,
			FinishedAt: e.FinishedAt,
		}

		if e.Ref != (graph.Ref{}) {
			ref := e.Ref
			shown.Nodes[i].Ref = &ref
		}
	}

	if lease := response.Lease; lease != nil {
		shown.Lease = &leaseShown{Failures: lease.Failures, NextAt: lease.NextAt, Reason: lease.Reason}
	}

	return shown
}

// respondDeleted writes what asking for what roots need to go came to, with
// the roots: always 202, whether it is gone, its delete on its way, or what
// came of it was heard, since a 204 says nothing, and these are what is to be
// said of it besides.
func respondDeleted(rw http.ResponseWriter, resource kind.Raw, gone bool, sent *kind.ActOnResource, answered *kind.ResourceActedOn, dependents []graph.Dependent, recreated bool) {
	body := deleted{commanded: commandedOf(resource, gone, sent, answered), Dependents: dependentsOf(dependents), Recreated: recreated}

	respond.JSON(rw, http.StatusAccepted, body)
}

// respondCommanded writes what asking for a command came to, with the status
// it stands for: a resource gone is 204, one whose command was carried out or
// answered is 200 (201 when it was made), and one whose command is on its way
// is 202.
func respondCommanded(rw http.ResponseWriter, resource kind.Raw, gone bool, sent *kind.ActOnResource, answered *kind.ResourceActedOn, made bool) {
	if gone {
		rw.WriteHeader(http.StatusNoContent)

		return
	}

	body := commandedOf(resource, gone, sent, answered)

	status := http.StatusOK

	switch {
	case made:
		status = http.StatusCreated
	case sent != nil && answered == nil:
		status = http.StatusAccepted
	}

	respond.JSON(rw, status, body)
}

// commandedOf is what asking for a command came to, as the API shows it: the
// resource as it is now, unless it is gone, the command its node was sent,
// and what came of it.
func commandedOf(resource kind.Raw, gone bool, sent *kind.ActOnResource, answered *kind.ResourceActedOn) commanded {
	var body commanded

	if !gone {
		body.Resource = &resource
	}

	if sent != nil {
		body.Command = &command{ID: sent.ID, Action: sent.Action, Node: sent.Node, Attempt: sent.Attempt}
	}

	if answered != nil {
		body.Result = &result{ID: answered.ID, Action: answered.Action, OK: answered.OK, Reason: answered.Reason, Output: answered.Output, At: answered.At}
	}

	return body
}

// refusedWith writes a refusal, a request refused field by field or what a
// node refused, with the roots that need what was asked to go, when any do:
// what refused it may be that they still use it.
func refusedWith(rw http.ResponseWriter, invalid domain.ValidationErrors, node *noderequest.Error, dependents []graph.Dependent, recreated bool) {
	if len(dependents) == 0 {
		if node != nil {
			respond.NodeRefused(rw, node)
		} else {
			respond.Refused(rw, invalid)
		}

		return
	}

	body := struct {
		Errors     domain.ValidationErrors `json:"errors,omitempty"`
		Error      *noderequest.Error      `json:"error,omitempty"`
		Dependents []dependent             `json:"dependents"`
		Recreated  bool                    `json:"recreated"`
	}{Dependents: dependentsOf(dependents), Recreated: recreated}

	status := http.StatusBadRequest

	if node != nil {
		body.Error = node
		status = respond.StatusOf(node.Code)
	} else {
		body.Errors = invalid
	}

	respond.JSON(rw, status, body)
}

// failed writes an error a use case returned: a kind, an action or a
// resource that is not there is 404, and anything else 500.
func failed(rw http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, kind.ErrUnknownKind) || errors.Is(err, kind.ErrUnknownAction) {
		rw.WriteHeader(http.StatusNotFound)

		return
	}

	respond.Failed(rw, r, err)
}

// flagOf is what a request's parameter that says yes or no says, and whether
// it says either: one not given says no.
func flagOf(r *http.Request, name string) (bool, bool) {
	given := r.URL.Query().Get(name)
	if len(given) == 0 {
		return false, true
	}

	yes, err := strconv.ParseBool(given)

	return yes, err == nil
}

// waitOf is how long a request asks to wait for what came of its command:
// seconds, or a duration such as 90s, and never longer than MaxWait. It
// reports whether what was asked is a wait at all.
func waitOf(r *http.Request) (time.Duration, bool) {
	asked := r.URL.Query().Get("wait")
	if len(asked) == 0 {
		return 0, true
	}

	if seconds, err := strconv.ParseUint(asked, 10, 32); err == nil {
		return min(time.Duration(seconds)*time.Second, MaxWait), true
	}

	wait, err := time.ParseDuration(asked)
	if err != nil || wait < 0 {
		return 0, false
	}

	return min(wait, MaxWait), true
}

// textUnmarshaler is a type whose JSON is text, such as a time.
var textUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()

// payloadOf is a query's payload, read from the request's parameters: each
// is the field of the payload type its name names, as its JSON is written. A
// field that is text takes what was given as text, and any other as JSON, so
// that tail=10 is a number; one given more than once is a list. A parameter
// that names no field is the request's, owner say, and not the payload's.
func payloadOf(values url.Values, payloadType reflect.Type) (json.RawMessage, error) {
	if payloadType == nil {
		return nil, nil
	}

	for payloadType.Kind() == reflect.Pointer {
		payloadType = payloadType.Elem()
	}

	if payloadType.Kind() != reflect.Struct {
		return nil, nil
	}

	fields := fieldsOf(payloadType)
	object := make(map[string]json.RawMessage)

	for name, given := range values {
		field, named := fields[name]
		if !named || len(given) == 0 {
			continue
		}

		if field.Kind() == reflect.Slice && field.Elem().Kind() != reflect.Uint8 {
			list := make([]json.RawMessage, len(given))
			for i, value := range given {
				list[i] = valueOf(value, field.Elem())
			}

			encoded, err := json.Marshal(list)
			if err != nil {
				return nil, err
			}

			object[name] = encoded

			continue
		}

		object[name] = valueOf(given[len(given)-1], field)
	}

	if len(object) == 0 {
		return nil, nil
	}

	return json.Marshal(object)
}

// valueOf is a parameter's value as the JSON of a field of type t.
func valueOf(value string, t reflect.Type) json.RawMessage {
	textual := t.Kind() == reflect.String || t.Implements(textUnmarshaler) || reflect.PointerTo(t).Implements(textUnmarshaler)

	if !textual && json.Valid([]byte(value)) {
		return json.RawMessage(value)
	}

	encoded, _ := json.Marshal(value)

	return encoded
}

// fieldsOf is the fields of a struct by the names its JSON gives them, with
// those of the structs it embeds among them.
func fieldsOf(t reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)

	for i := range t.NumField() {
		field := t.Field(i)

		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}

		if field.Anonymous && len(name) == 0 {
			embedded := field.Type
			for embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}

			if embedded.Kind() == reflect.Struct {
				for inner, innerType := range fieldsOf(embedded) {
					if _, taken := fields[inner]; !taken {
						fields[inner] = innerType
					}
				}

				continue
			}
		}

		if !field.IsExported() {
			continue
		}

		if len(name) == 0 {
			name = field.Name
		}

		fields[name] = field.Type
	}

	return fields
}

// labelsOf are the labels a listing is narrowed to, each given as key=value,
// and whether every one of them was: a key is never empty, while a value may
// be.
func labelsOf(given []string) (map[string]string, bool) {
	if len(given) == 0 {
		return nil, true
	}

	labels := make(map[string]string, len(given))

	for _, label := range given {
		key, value, found := strings.Cut(label, "=")
		if !found || len(key) == 0 {
			return nil, false
		}

		labels[key] = value
	}

	return labels, true
}
