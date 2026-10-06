// Package runCommand carries out what the control plane commands of the
// resources this node holds, whatever their kind, and says what came of it.
package runCommand

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

// Locks keeps what is done to one resource from overlapping. A node's VMs
// have theirs already, and a command takes the same ones: a VM's command
// waits for whatever else is being done to that VM.
type Locks interface {
	// Lock holds the lock of the resource uuid names, waiting for whoever
	// holds it now, and hands back what releases it. Giving up on waiting
	// is ctx ending, which holds nothing.
	Lock(ctx context.Context, uuid string) (release func(), err error)
}

// UseCase carries out commands.
//
// A command reaches the kind it names through the registry of the kinds this
// node runs, and that kind's node strategy carries it out while nothing else
// is done to the resource: two commands to one resource take turns, the
// first one's result said before the second begins, and commands to
// different resources never wait for each other.
//
// What came of a command is said as its Result, a failure as much as a
// success, because a command that failed fails the same way when it is
// carried out again. So is a command this node cannot carry out at all: one
// of a kind it does not run, an action its kind does not have, a payload that
// cannot be read. None of those is ever asked for again behind anybody's
// back. What is returned is only a failure to say what came of it, or the
// node going away halfway, either of which is worth another delivery: the
// command is carried out again, here or by whoever takes it next.
//
// A command is given as long as its kind's strategy takes. Each kind bounds
// its own commands, as the engine bounds a VM's, since only it knows how long
// restoring a disk or pulling a stack's images may take.
type UseCase struct {
	kinds    *kind.Registry[kind.NodeBinding]
	locks    Locks
	producer domain.Producer
}

func NewUseCase(kinds *kind.Registry[kind.NodeBinding], locks Locks, producer domain.Producer) *UseCase {
	return &UseCase{kinds: kinds, locks: locks, producer: producer}
}

// Execute carries out a command addressed to this node and says what came of
// it, on kind.ResultName.
func (uc *UseCase) Execute(ctx context.Context, command kind.Command) error {
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String("workload.kind", command.Kind),
		attribute.String("workload.resource", command.UUID),
		attribute.String("workload.action", command.Action),
	)

	binding, runs := uc.kinds.Lookup(command.Kind)
	if !runs {
		return uc.say(ctx, failed(command, fmt.Errorf("%w: this node runs no %q", kind.ErrUnknownKind, command.Kind)))
	}

	// a command about nothing in particular would be carried out on whatever
	// its resource says it is, under nobody's lock.
	if len(command.UUID) == 0 {
		return uc.say(ctx, failed(command, fmt.Errorf("%w: the command names no %s", kind.ErrInvalidPayload, command.Kind)))
	}

	release, err := uc.locks.Lock(ctx, command.UUID)
	if err != nil {
		return err
	}

	// released once the result is said, so the results of one resource's
	// commands are said in the order they were carried out.
	defer release()

	result := binding.Execute(ctx, command)

	// a node going away has not failed the command: whoever takes it next
	// carries it out.
	if !result.OK && ctx.Err() != nil {
		return fmt.Errorf("the %s %q was left halfway through its %s: %w", command.Kind, command.UUID, command.Action, ctx.Err())
	}

	return uc.say(ctx, result)
}

// say sends what came of a command. It is detached from the command's
// context, so it is said even when whoever was waiting has gone.
func (uc *UseCase) say(ctx context.Context, result kind.Result) error {
	result.At = time.Now()

	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return uc.producer.Produce(context.WithoutCancel(ctx), kind.ResultName, payload)
}

// failed is the result of a command that was not carried out, and why.
func failed(command kind.Command, cause error) kind.Result {
	return kind.Result{
		ID:      command.ID,
		Kind:    command.Kind,
		UUID:    command.UUID,
		Action:  command.Action,
		Node:    command.Node,
		Attempt: command.Attempt,
		OK:      false,
		Reason:  cause.Error(),
	}
}
