// Package refusal reads what the workload refused out of the error it answered
// with, so a use case can hand it back the way it hands back its own
// validation: field by field, in the reader's language.
//
// The workload says no in two ways. The control plane refuses a request it
// would not take with codes by field — a quota, a bound, a Docker VM to choose
// — which it checks itself, since it is the one holding what they are checked
// against. A node refuses what it was asked about a VM it holds: a VM that is
// not running, one that is not a Docker VM, a dockerd that did not come up, a
// request dockerd itself turned down. Both are answers to the request rather
// than failures to answer it, so neither is an error by the time it leaves
// here.
package refusal

import (
	"errors"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/translator"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/controlplane/client"
)

// The fields a node's refusal is reported under: what it is about, since a
// node does not know which field of the request led to it.
const (
	// FieldVM is the VM the request was about.
	FieldVM = "vm"

	// FieldDocker is what dockerd itself turned down, in its own words.
	FieldDocker = "docker"

	// FieldSnapshot is the snapshot a VM was to be made or restored from.
	FieldSnapshot = "snapshot_uuid"
)

// reasons are the refusals the domain has errors for, the field each is about
// and the code it is said by. A node's refusal arrives as a
// *noderequest.Error, which errors.Is reads as these same errors.
var reasons = []struct {
	err   error
	field string
	code  string

	// detailed says what was refused is worth repeating after the code: what
	// dockerd says is the part that tells somebody what to fix.
	detailed bool
}{
	{err: vm.ErrNotRunning, field: FieldVM, code: string(noderequest.CodeNotRunning)},
	{err: vm.ErrNotDocker, field: FieldVM, code: string(noderequest.CodeNotDocker)},
	{err: docker.ErrUnavailable, field: FieldVM, code: string(noderequest.CodeDockerUnavailable)},
	{err: vm.ErrNoCapacity, field: FieldVM, code: "no_capacity"},
	{err: vm.ErrQuotaExceeded, field: FieldVM, code: "quota_exceeded"},
	{err: vm.ErrEngineMismatch, field: FieldSnapshot, code: "engine_mismatch"},
	{err: docker.ErrInvalid, field: FieldDocker, code: string(noderequest.CodeInvalid), detailed: true},
}

// Of tells a refusal apart from a failure.
//
// What the workload refused comes back as the fields it refused and why, each
// reason translated; err is nil then. Anything else comes back as it was: nil
// for nil, and domain.ErrNotExists, a timeout or a failure to reach the
// workload as the error it is, for the caller to answer as such.
func Of(err error, t translator.Translator) (domain.ValidationErrors, error) {
	if err == nil {
		return nil, nil
	}

	if refused, ok := errors.AsType[*client.ValidationError](err); ok && len(refused.ValidationErrors) > 0 {
		said := make(domain.ValidationErrors, len(refused.ValidationErrors))
		for field, reason := range refused.ValidationErrors {
			said[field] = Say(t, field, reason)
		}

		return said, nil
	}

	for _, reason := range reasons {
		if !errors.Is(err, reason.err) {
			continue
		}

		said := Say(t, reason.field, reason.code)
		if node, ok := errors.AsType[*noderequest.Error](err); ok && reason.detailed && len(node.Message) > 0 {
			said += ": " + node.Message
		}

		return domain.ValidationErrors{reason.field: said}, nil
	}

	return nil, err
}

// Say is a reason in the reader's language. A reason there are no words for is
// kept as it was given: the control plane may already have put it in words,
// and an empty message would say less than its own.
func Say(t translator.Translator, field string, reason string) string {
	if said := t.Translate(reason, translator.WithAttribute("field", field)); len(said) > 0 {
		return said
	}

	return reason
}
