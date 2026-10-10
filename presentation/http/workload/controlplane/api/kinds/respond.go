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

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
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

// respondCommanded writes what asking for a command came to, with the status
// it stands for: a resource gone is 204, one whose command was carried out or
// answered is 200 (201 when it was made), and one whose command is on its way
// is 202.
func respondCommanded(rw http.ResponseWriter, resource kind.Raw, gone bool, sent *kind.ActOnResource, answered *kind.ResourceActedOn, made bool) {
	if gone {
		rw.WriteHeader(http.StatusNoContent)

		return
	}

	body := commanded{Resource: &resource}

	if sent != nil {
		body.Command = &command{ID: sent.ID, Action: sent.Action, Node: sent.Node, Attempt: sent.Attempt}
	}

	if answered != nil {
		body.Result = &result{ID: answered.ID, Action: answered.Action, OK: answered.OK, Reason: answered.Reason, Output: answered.Output, At: answered.At}
	}

	status := http.StatusOK

	switch {
	case made:
		status = http.StatusCreated
	case sent != nil && answered == nil:
		status = http.StatusAccepted
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
