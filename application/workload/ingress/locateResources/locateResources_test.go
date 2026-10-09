package locateResources_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/application/workload/ingress/locateResources"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/ingress"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	ingressMemory "github.com/khanzadimahdi/testproject/infrastructure/workload/ingress/memory"
)

const nodeName = "workload-orchestrator-01"

// A lamp is reached while it is lit: lit it is what lighting it desires,
// unlit what switching it off does, and dimming it desires nothing, which
// leaves it as it is, letting in what it is given.
const (
	unlit kind.State = "unlit"
	lit   kind.State = "lit"
)

// lamps are the lamp kind as its ingress strategy would know it: a lamp's
// status says in so many words where it is, and its spec the ports it lets
// in.
type lamps struct{}

var _ locateResources.Kind = lamps{}

func (lamps) Descriptor() kind.Descriptor {
	command := func(name string, desires kind.State) kind.Action {
		return kind.Action{Name: name, Runs: kind.OnNode, Mode: kind.ModeCommand, Desires: desires, Permission: "manage", Payload: kind.NoPayload}
	}

	return kind.Descriptor{
		Name:      "lamp",
		Plural:    "lamps",
		StateBy:   kind.OnNode,
		Endpoints: true,
		Machine: kind.Machine{
			Initial:  unlit,
			States:   []kind.State{unlit, lit, kind.Failed, kind.Deleted},
			Terminal: []kind.State{unlit, kind.Failed, kind.Deleted},
		},
		Actions: []kind.Action{
			command("light", lit),
			command("dim", ""),
			command("switch-off", unlit),
			command("delete", kind.Deleted),
		},
	}
}

func (lamps) Running() kind.State {
	return lit
}

func (lamps) Read(status json.RawMessage) (ingress.Heard, error) {
	var said struct {
		State kind.State  `json:"state"`
		Slug  string      `json:"slug"`
		Ports []port.Port `json:"ports"`
	}

	if err := json.Unmarshal(status, &said); err != nil {
		return ingress.Heard{}, err
	}

	if len(said.State) == 0 {
		return ingress.Heard{}, errors.New("a lamp always says what it is doing")
	}

	return ingress.Heard{Slug: said.Slug, State: said.State, Ports: said.Ports}, nil
}

func (lamps) Allowed(r kind.Raw) ([]port.Port, error) {
	var spec struct {
		Ports []port.Port `json:"ports"`
	}

	if err := json.Unmarshal(r.Spec, &spec); err != nil {
		return nil, err
	}

	return spec.Ports, nil
}

// lampsOnly are the kinds the tests route to.
var lampsOnly = map[string]locateResources.Kind{"lamp": lamps{}}

// recording is locations that keep what they are told and find nothing.
type recording struct {
	lock     sync.Mutex
	heard    []ingress.Heard
	withheld []ingress.Withheld
	answered []ingress.Answered
}

var _ ingress.Locations = &recording{}

func (r *recording) Hear(_ context.Context, heard ingress.Heard) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.heard = append(r.heard, heard)
}

func (r *recording) Withhold(_ context.Context, withheld ingress.Withheld) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.withheld = append(r.withheld, withheld)
}

func (r *recording) Answer(_ context.Context, answered ingress.Answered) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.answered = append(r.answered, answered)
}

func (r *recording) ByUUID(context.Context, string, string) (ingress.Heard, error) {
	return ingress.Heard{}, domain.ErrNotExists
}

func (r *recording) BySlug(context.Context, string, string) (ingress.Heard, error) {
	return ingress.Heard{}, domain.ErrNotExists
}

// told is what the ingress hears of where lamps are, as it hears it: their
// nodes' heartbeats, the commands sent to their nodes and what came of them.
type told struct {
	locations *ingressMemory.Locations

	heartbeat       *locateResources.HeartbeatHandler
	actOnResource   *locateResources.ActOnResourceHandler
	resourceActedOn *locateResources.ResourceActedOnHandler
}

func telling(kinds map[string]locateResources.Kind) *told {
	logger := slog.New(slog.DiscardHandler)
	locations := ingressMemory.NewLocations(time.Minute)

	return &told{
		locations:       locations,
		heartbeat:       locateResources.NewHeartbeatHandler(locations, kinds, logger),
		actOnResource:   locateResources.NewActOnResourceHandler(locations, kinds, logger),
		resourceActedOn: locateResources.NewResourceActedOnHandler(locations, kinds, logger),
	}
}

// beat is a node's heartbeat of a lamp it holds, at a moment, saying status.
func (tl *told) beat(t *testing.T, uuid string, at time.Time, status string) {
	t.Helper()

	require.NoError(t, tl.heartbeat.Handle(context.Background(), message(t, kind.Heartbeat{Node: nodeName, At: at, Observed: kind.Observation{Kind: "lamp", UUID: uuid, Status: json.RawMessage(status)}})))
}

// command is a command sent to the node holding a lamp, carrying it with a
// spec.
func (tl *told) command(t *testing.T, id string, uuid string, action string, spec string) {
	t.Helper()

	require.NoError(t, tl.actOnResource.Handle(context.Background(), message(t, kind.ActOnResource{
		ID:       id,
		Kind:     "lamp",
		UUID:     uuid,
		Action:   action,
		Node:     nodeName,
		Resource: kind.Raw{Kind: "lamp", Metadata: kind.Metadata{UUID: uuid, Node: nodeName}, Spec: json.RawMessage(spec)},
	})))
}

// result is what came of a command.
func (tl *told) result(t *testing.T, result kind.ResourceActedOn) {
	t.Helper()

	result.Kind, result.Node = "lamp", nodeName

	require.NoError(t, tl.resourceActedOn.Handle(context.Background(), message(t, result)))
}

// found is where the lamp a slug names is found, or why it is not.
func (tl *told) found(slug string) (ingress.Heard, error) {
	return tl.locations.BySlug(context.Background(), "lamp", slug)
}

func message(t *testing.T, of any) []byte {
	t.Helper()

	payload, err := json.Marshal(of)
	require.NoError(t, err)

	return payload
}

func status(t *testing.T, of any) json.RawMessage {
	t.Helper()

	return json.RawMessage(message(t, of))
}
