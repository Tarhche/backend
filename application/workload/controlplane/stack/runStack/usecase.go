package runStack

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/runtime/allowed"
	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/placement"
	runTask "github.com/khanzadimahdi/testproject/application/workload/controlplane/task/runTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/runtime"
	"github.com/khanzadimahdi/testproject/domain/workload/stack"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/slug"
)

// UseCase runs a set of services as one stack.
//
// Every service is a task of its own, so the workload has one kind of thing
// to schedule and one lifecycle to reason about. What makes them a stack is the
// private network they share — and because a bridge is local to the node that
// created it, and belongs to the class that made it, they are all run with one
// class, on one node.
type UseCase struct {
	stackRepository stack.Repository
	runTask         *runTask.UseCase
	placement       *placement.Placement
	defaults        task.ResourceLimits
	classes         allowed.Classes
	validator       domain.Validator
	logger          *slog.Logger
}

func NewUseCase(
	stackRepository stack.Repository,
	runTaskUseCase *runTask.UseCase,
	placement *placement.Placement,
	defaults task.ResourceLimits,
	classes allowed.Classes,
	validator domain.Validator,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		stackRepository: stackRepository,
		runTask:         runTaskUseCase,
		placement:       placement,
		defaults:        defaults,
		classes:         classes,
		validator:       validator,
		logger:          logger,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(admission{Request: request, classes: uc.classes}); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	// the one class every service runs with, which the stack is stored with
	// and each of its services is created with.
	class, _ := request.Class(uc.classes.Default())

	// sorted, so a stack's services are always created in the same order and
	// a failure part-way through is reproducible.
	names := make([]string, 0, len(request.Services))
	for name := range request.Services {
		names = append(names, name)
	}
	slices.Sort(names)

	services := make([]*runTask.Request, len(names))
	needs := make([]placement.Need, len(names))

	for i, name := range names {
		service := request.Services[name]

		serviceRequest := runTask.FromSpec(request.Name+"-"+name, &service, uc.defaults)
		serviceRequest.Runtime = class
		serviceRequest.ServiceName = name
		serviceRequest.OwnerUUID = request.OwnerUUID

		services[i] = serviceRequest
		needs[i] = placement.Need{
			NetworkPolicy: serviceRequest.Policy(),
			ReadOnly:      serviceRequest.ReadOnly,
			RestartPolicy: serviceRequest.RestartPolicy,
			Memory:        serviceRequest.ResourceLimits.Memory,
			CPU:           serviceRequest.ResourceLimits.Cpu,
			StackNetwork:  true,
		}
	}

	nodeName, err := uc.pickNode(ctx, class, needs)
	if err != nil {
		return nil, err
	}

	stackSlug, err := slug.Generate(request.Name)
	if err != nil {
		return nil, err
	}

	s := stack.Stack{
		Name:          request.Name,
		Slug:          stackSlug,
		Runtime:       class,
		ExpectedState: task.Running,
		NodeName:      nodeName,
		OwnerUUID:     request.OwnerUUID,
	}

	uuid, err := uc.stackRepository.Save(ctx, &s)
	if err != nil {
		return nil, err
	}

	for i, serviceRequest := range services {
		serviceRequest.StackUUID = uuid
		serviceRequest.StackSlug = stackSlug
		serviceRequest.NominatedNode = nodeName

		response, err := uc.runTask.Execute(ctx, serviceRequest)
		if err != nil {
			return nil, err
		}

		if response != nil && len(response.ValidationErrors) > 0 {
			// the spec was validated before anything was created, so a service
			// rejected here is a mistake in the workload rather than in what was
			// asked for.
			uc.logger.ErrorContext(ctx, "a validated service was rejected", "stack", uuid, "service", names[i], "errors", response.ValidationErrors)

			return &Response{ValidationErrors: response.ValidationErrors}, nil
		}
	}

	return &Response{UUID: uuid, Slug: stackSlug}, nil
}

// pickNode chooses the one node a stack's services all run on: one that offers
// the stack's class and can run every one of its services.
//
// When no node can take the stack right now, or no node offers its class at
// all, the stack is stood up without one rather than refused. Each service is
// then placed as it is created, the first one placed deciding for the rest,
// and each waits, or fails for its class, exactly as a task of its own would —
// which is also what a stack asked for while every node was being redeployed
// does, rather than failing outright.
func (uc *UseCase) pickNode(ctx context.Context, class runtime.Class, needs []placement.Need) (string, error) {
	selected, err := uc.placement.PlaceTogether(ctx, class, needs...)

	switch {
	case errors.Is(err, placement.ErrNoNodeOffersRuntime), errors.Is(err, placement.ErrNoNodeReady):
		uc.logger.InfoContext(ctx, "no node can take a stack right now; its services are placed as they are created",
			"runtime", class.String(), "because", err.Error())

		return "", nil

	case err != nil:
		return "", err
	}

	return selected.Name, nil
}
