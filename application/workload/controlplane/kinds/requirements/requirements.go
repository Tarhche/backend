// Package requirements is what each kind's graph.Kind does with the records
// the control plane keeps, which is the same for every kind: a record is made
// through its kind's own admission, asked again for what makes it through its
// kind's own commands, and removed through the control plane's delete, and
// each time it is waited on until its node has said what came of it, the way
// ?wait= waits: this control plane hearing the answer, or the record saying
// that another one heard it.
//
// Which record a graph's node is, whether it is made, and what it waits on
// are its kind's to say, in its control-plane package beside its strategy.
package requirements

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/admitResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/deleteResource"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/kinds/dispatch"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/graph"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// Dependencies are what records are kept in, and asked and waited on
// through: the control plane's own, as its resource API has them.
type Dependencies struct {
	Registry   *kind.Registry[kind.ControlPlaneBinding]
	Resources  resource.Repository
	Dispatcher *dispatch.Dispatcher

	// Admission admits a resource of any kind, and Deletion deletes one.
	Admission *admitResource.UseCase
	Deletion  *deleteResource.UseCase

	// Timeouts are how long a node may take over a command given each
	// timeout (kind.Action.Timeout), as the reconcile loop is given them, and
	// Patience how long any command is waited on beyond that: as long as the
	// loop waits on one before it asks for it again.
	Timeouts map[kind.Timeout]time.Duration
	Patience time.Duration
}

// Records are the records a graph's nodes are, made, asked and removed.
type Records struct {
	Dependencies
}

func New(dependencies Dependencies) *Records {
	return &Records{Dependencies: dependencies}
}

// Admit admits what request asks for, through its kind's admission, sends it
// the first command its kind asks of it, and is its record once what came of
// that is heard, or once it has waited as long as that command may take.
// What its kind refuses is an error saying why, field by field.
func (r *Records) Admit(ctx context.Context, request admitResource.Request) (resource.Record, error) {
	request.Wait = 0

	admitted, err := r.Admission.Execute(ctx, &request)
	switch {
	case err != nil:
		return resource.Record{}, err
	case len(admitted.ValidationErrors) > 0:
		return resource.Record{}, Refused(request.Kind, admitted.ValidationErrors)
	}

	return r.Settled(ctx, request.Kind, admitted.Resource.Metadata.UUID)
}

// Ask asks rec for one of its kind's commands, as the reconcile loop asks
// for what a resource is expected to be, sends it to its node, and is the
// record once what came of it is heard, or once it has waited as long as the
// command may take. What its kind refuses is an error saying why.
func (r *Records) Ask(ctx context.Context, rec resource.Record, action string) (resource.Record, error) {
	binding, err := r.binding(rec.Kind)
	if err != nil {
		return resource.Record{}, err
	}

	asked, invalid, err := r.Dispatcher.Ask(ctx, binding, rec, action, nil, false)
	switch {
	case err != nil:
		return resource.Record{}, err
	case len(invalid) > 0:
		return resource.Record{}, Refused(rec.Kind, invalid)
	case asked.Gone:
		return resource.Record{}, Gone(rec)
	}

	// written down as what it waits on, a command its node is not sent now
	// is sent again by the reconcile loop.
	if asked.Command != nil {
		if _, err := r.Dispatcher.Send(ctx, *asked.Command, 0); err != nil {
			return resource.Record{}, err
		}
	}

	return r.Settled(ctx, rec.Kind, rec.Metadata.UUID)
}

// Settle waits for what came of the command rec waits on, when it waits on
// one, for as long as its node may take over it and the patience beyond, and
// is the record as that left it, or whether it went.
func (r *Records) Settle(ctx context.Context, rec resource.Record) (resource.Record, bool, error) {
	if rec.Pending != nil {
		binding, err := r.binding(rec.Kind)
		if err != nil {
			return resource.Record{}, false, err
		}

		r.Dispatcher.Await(ctx, rec, r.Wait(binding.Descriptor(), rec.Pending.Action))
	}

	latest, there, err := r.Read(ctx, rec.Kind, rec.Metadata.UUID)

	return latest, !there && err == nil, err
}

// Settled is the record of the kind uuid names once what came of the command
// it waits on is heard, or once it has waited as long as that command may
// take: one that went, or was never there, is an error, since it was to be
// made.
func (r *Records) Settled(ctx context.Context, kindName string, uuid string) (resource.Record, error) {
	rec, there, err := r.Read(ctx, kindName, uuid)
	switch {
	case err != nil:
		return resource.Record{}, err
	case !there:
		return resource.Record{}, Gone(resource.Record{Raw: kind.Raw{Kind: kindName, Metadata: kind.Metadata{UUID: uuid}}})
	}

	settled, went, err := r.Settle(ctx, rec)
	switch {
	case err != nil:
		return resource.Record{}, err
	case went:
		return resource.Record{}, Gone(rec)
	}

	return settled, nil
}

// Remove asks for the record ref names to be deleted, through the control
// plane's delete, with payload, and is done once it is gone: one gone already
// is. One still there once its delete may have been carried out, which it
// was refused say, is an error saying what it is and why.
func (r *Records) Remove(ctx context.Context, ref graph.Ref, payload json.RawMessage) error {
	if _, there, err := r.Read(ctx, ref.Kind, ref.UUID); err != nil || !there {
		return err
	}

	deleted, err := r.Deletion.Execute(ctx, &deleteResource.Request{Kind: ref.Kind, UUID: ref.UUID, Payload: payload})
	switch {
	case errors.Is(err, domain.ErrNotExists):
		return nil
	case err != nil:
		return err
	case len(deleted.ValidationErrors) > 0:
		return Refused(ref.Kind, deleted.ValidationErrors)
	case deleted.Gone:
		return nil
	}

	rec, there, err := r.Read(ctx, ref.Kind, ref.UUID)
	if err != nil || !there {
		return err
	}

	rec, went, err := r.Settle(ctx, rec)
	if err != nil || went {
		return err
	}

	return NotDone(rec, "removed")
}

// Read is the record of the kind uuid names, as it is now, and whether there
// is one.
func (r *Records) Read(ctx context.Context, kindName string, uuid string) (resource.Record, bool, error) {
	rec, err := r.Resources.GetOne(ctx, kindName, uuid)
	switch {
	case errors.Is(err, domain.ErrNotExists):
		return resource.Record{}, false, nil
	case err != nil:
		return resource.Record{}, false, err
	}

	return rec, true, nil
}

// Wait is how long a command for action, of the kind d describes, is waited
// on: what its node may take over it, and the patience beyond.
func (r *Records) Wait(d kind.Descriptor, action string) time.Duration {
	a, _ := d.Action(action)

	return r.Timeouts[a.Timeout] + r.Patience
}

func (r *Records) binding(kindName string) (kind.ControlPlaneBinding, error) {
	binding, registered := r.Registry.Lookup(kindName)
	if !registered {
		return nil, fmt.Errorf("%w: %q", kind.ErrUnknownKind, kindName)
	}

	return binding, nil
}

// Want is spec as W, a kind's own requirement type, handed over as one or as
// a pointer to one: anything else is kind.ErrInvalidPayload.
func Want[W any](kindName string, spec any) (W, error) {
	switch want := spec.(type) {
	case W:
		return want, nil
	case *W:
		if want != nil {
			return *want, nil
		}
	}

	var none W

	return none, fmt.Errorf("%w: a %s is required as a %T, not as a %T", kind.ErrInvalidPayload, kindName, none, spec)
}

// RefOf is the record a node became, as a graph keeps it.
func RefOf(rec resource.Record) graph.Ref {
	return graph.Ref{Kind: rec.Kind, UUID: rec.Metadata.UUID}
}

// Going reports whether a resource is on its way out: expected deleted, or
// deleted, which nothing is made again from.
func Going(common kind.Status) bool {
	return common.Expected == kind.Deleted || common.State == kind.Deleted
}

// Refused is an error saying what of what was asked a kind refused, field by
// field.
func Refused(kindName string, invalid domain.ValidationErrors) error {
	fields := slices.Sorted(maps.Keys(invalid))

	said := make([]string, len(fields))
	for i, field := range fields {
		said[i] = field + ": " + invalid[field]
	}

	return fmt.Errorf("the %s was refused: %s", kindName, strings.Join(said, ", "))
}

// NotDone is an error saying a record is not done, as it was to be, and
// what it is instead, and why, as it says: its reason, or what its last
// command failed with.
func NotDone(rec resource.Record, done string) error {
	common, err := rec.Common()
	if err != nil {
		return err
	}

	said := fmt.Sprintf("the %s %s is not %s: it is %s", rec.Kind, nameOf(rec), done, common.State)
	if why := whyOf(rec.Status, common.Reason); len(why) > 0 {
		said += ": " + why
	}

	return errors.New(said)
}

// Gone is an error saying a record that was to be made went instead.
func Gone(rec resource.Record) error {
	return fmt.Errorf("the %s %s is gone", rec.Kind, nameOf(rec))
}

// nameOf is what a record is called, or its uuid when it is called nothing.
func nameOf(rec resource.Record) string {
	return cmp.Or(rec.Metadata.Name, rec.Metadata.UUID)
}

// whyOf is why a resource is what it is: reason, or what its status says its
// last command failed with.
func whyOf(status json.RawMessage, reason string) string {
	if len(reason) > 0 {
		return reason
	}

	var said struct {
		Failure *noderequest.Error `json:"failure"`
	}

	if len(status) > 0 && json.Unmarshal(status, &said) == nil && said.Failure != nil {
		return said.Failure.Message
	}

	return ""
}
