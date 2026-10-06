// Package observe takes what the nodes say of resources onto their records:
// what a heartbeat's report observed of each, and what a command's result
// says it left one as.
//
// What a node says a resource is doing is its word for it; what that makes
// the resource is the kind's machine's to say, since only the control plane
// knows what was asked of it. A resource at rest is what its node says it
// is; one in flight takes only the arrivals its machine declares, so that a
// report sent before a stop reached the node does not undo the stop, and one
// its machine says is answered takes them only from its command's result. The
// kind's own fields of what was observed, a VM's usage or a stack's services,
// are taken whatever becomes of the state.
//
// What is expected of a resource is never a node's to say: it is what was
// asked of it, and stays so until something else is.
package observe

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
)

// ErrNotWaitedOn is a result for a command the resource is not waiting on:
// one it was asked for before something else, or one answered already.
var ErrNotWaitedOn = errors.New("the resource is not waiting on that command")

// Change is what taking something onto a record changed.
type Change struct {
	// Gone says the resource reached deleted: its record is to go.
	Gone bool

	// Changed says something about it other than when it was last observed
	// changed, which is what is worth writing down at once.
	Changed bool
}

// Observe takes onto a record what its resource was observed doing at a
// moment: its status, as its node put it. One waiting for the answer to its
// command (kind.Machine.Answered) stays where it is, whatever its node says.
func Observe(d kind.Descriptor, r *resource.Record, status json.RawMessage, at time.Time) (Change, error) {
	return observe(d, r, status, at, false)
}

// observe takes a status onto a record, as its node put it, or as the result
// of the command it was waiting on did when answered is set.
func observe(d kind.Descriptor, r *resource.Record, status json.RawMessage, at time.Time, answered bool) (Change, error) {
	observed, err := resource.Common(status)
	if err != nil {
		return Change{}, err
	}

	recorded, err := r.Common()
	if err != nil {
		return Change{}, err
	}

	merged, err := resource.Merge(r.Status, status)
	if err != nil {
		return Change{}, err
	}

	common := recorded

	// what a node is seen doing says nothing of a command it may not have
	// carried out yet, when it does the same before and after it.
	taken := false
	if len(observed.State) > 0 && (answered || !d.Machine.IsAnswered(recorded.State)) {
		common.State, taken = d.Machine.Next(recorded.State, kind.OnObserved(observed.State))
	}

	if taken {
		if common.State != recorded.State {
			common.Since = at
		}

		common.Reason = observed.Reason
	}

	common.ObservedAt = at

	merged, err = resource.WithCommon(merged, common)
	if err != nil {
		return Change{}, err
	}

	change := Change{Gone: common.State == kind.Deleted, Changed: resource.Differs(r.Status, merged)}

	r.Status = merged

	// it got where it was going: there is no command to wait on any more.
	// The tries it took are forgotten only once it stays there for a while,
	// which is the reconcile loop's to see: one that falls over again as soon
	// as it is brought back is brought back less and less often.
	if d.Machine.IsInFlight(recorded.State) && !d.Machine.IsInFlight(common.State) {
		r.Pending = nil
	}

	return change, nil
}

// Answer takes onto a record what came of a command it was waiting on, at a
// moment. A result for any other command is ErrNotWaitedOn, and changes
// nothing: it came too late to say anything the record does not know better.
//
// A command carried out says what it left the resource as, when its kind's
// strategy said; one that deleted it leaves it gone, whatever it said. A
// command that failed for good fails the resource, with its reason, when the
// command was to make it something: one that was not, a query's or a
// snapshot's say, fails only itself. One its node refused as it was asked
// (kind.Result.Refused) did nothing: the resource is what its node says it
// is, and when the command was to make it something, it is expected to stay
// what it is rather than to be asked the same again, which would be refused
// the same way. A volume a container mounts is not removed, and is not
// removed later either, once nobody remembers it was asked.
func Answer(d kind.Descriptor, r *resource.Record, result kind.Result, at time.Time) (Change, error) {
	if !r.Pending.Answers(result.ID) {
		return Change{}, ErrNotWaitedOn
	}

	action, _ := d.Action(result.Action)

	recorded, err := r.Common()
	if err != nil {
		return Change{}, err
	}

	answer := result
	answer.Status = nil
	r.Answer = &answer

	if result.OK {
		if action.Desires == kind.Deleted {
			r.Pending = nil

			return Change{Gone: true, Changed: true}, nil
		}

		if len(result.Status) > 0 {
			if _, err := observe(d, r, result.Status, at, true); err != nil {
				return Change{}, err
			}
		}

		common, err := r.Common()
		if err != nil {
			return Change{}, err
		}

		// a command that left it in flight is waited on still: what it is on
		// its way to is for its node to report, and the command is sent again
		// if it never does.
		if !d.Machine.IsInFlight(common.State) {
			r.Pending = nil
		}

		return Change{Gone: common.State == kind.Deleted, Changed: true}, nil
	}

	if len(result.Status) > 0 {
		merged, err := resource.Merge(r.Status, result.Status)
		if err != nil {
			return Change{}, err
		}

		r.Status = merged
	}

	r.Pending = nil

	if result.Refused {
		return refused(d, r, recorded, action, result, at)
	}

	if action.Desires == "" && !d.Machine.IsInFlight(recorded.State) {
		return Change{Changed: true}, nil
	}

	reason := result.Reason
	if len(reason) == 0 {
		reason = fmt.Sprintf("the %s failed", result.Action)
	}

	if err := Fail(r, reason, at); err != nil {
		return Change{}, err
	}

	// what a command's result says it is doing is an observation of it too.
	common, err := r.Common()
	if err != nil {
		return Change{}, err
	}

	common.ObservedAt = at

	if err := r.SetCommon(common); err != nil {
		return Change{}, err
	}

	return Change{Changed: true}, nil
}

// refused takes onto a record a command its node refused as it was asked:
// what the resource is doing is what its node says, when it says it is at
// rest, and what it is expected to be is that, when the command was to make
// it something else. What its node did not say leaves it as it was recorded,
// in flight as the command left it, until its node reports it.
func refused(d kind.Descriptor, r *resource.Record, recorded kind.Status, action kind.Action, result kind.Result, at time.Time) (Change, error) {
	common := recorded

	observed, err := resource.Common(result.Status)
	if err != nil {
		return Change{}, err
	}

	if state := observed.State; len(state) > 0 && !d.Machine.IsInFlight(state) && state != kind.Failed {
		if state != common.State {
			common.State = state
			common.Since = at
		}

		if len(action.Desires) > 0 {
			common.Expected = state
		}
	}

	common.ObservedAt = at

	if err := r.SetCommon(common); err != nil {
		return Change{}, err
	}

	return Change{Changed: true}, nil
}

// Fail fails a resource, for reason, at a moment: one whose command failed
// for good, or whose node is lost. What is expected of it stays as it was, so
// that it is brought back once it can be, and it waits on no command any
// more.
func Fail(r *resource.Record, reason string, at time.Time) error {
	common, err := r.Common()
	if err != nil {
		return err
	}

	if common.State != kind.Failed {
		common.State = kind.Failed
		common.Since = at
	}

	common.Reason = reason

	if err := r.SetCommon(common); err != nil {
		return err
	}

	r.Pending = nil

	return nil
}
