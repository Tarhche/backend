package runStack

import (
	"context"
	"log/slog"
	"slices"
	"time"

	runTask "github.com/khanzadimahdi/testproject/application/runner/manager/task/runTask"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/stack"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
	"github.com/khanzadimahdi/testproject/infrastructure/runner/slug"
)

const (
	// nominatedNodesLimit is how many nodes are considered when placing a
	// stack.
	nominatedNodesLimit = 10

	// heartbeatGracePeriod is how stale a node's last heartbeat may be before
	// it is passed over.
	heartbeatGracePeriod = 3 * time.Second
)

// UseCase runs a set of services as one stack.
//
// Every service is a container of its own, so the runner has one kind of thing
// to schedule and one lifecycle to reason about. What makes them a stack is the
// private network they share — and because a bridge is local to the node that
// created it, they are all placed on one node.
type UseCase struct {
	stackRepository stack.Repository
	nodeRepository  node.Repository
	runTask         *runTask.UseCase
	scheduler       task.Scheduler
	defaults        task.ResourceLimits
	validator       domain.Validator
	logger          *slog.Logger
}

func NewUseCase(
	stackRepository stack.Repository,
	nodeRepository node.Repository,
	runTaskUseCase *runTask.UseCase,
	scheduler task.Scheduler,
	defaults task.ResourceLimits,
	validator domain.Validator,
	logger *slog.Logger,
) *UseCase {
	return &UseCase{
		stackRepository: stackRepository,
		nodeRepository:  nodeRepository,
		runTask:         runTaskUseCase,
		scheduler:       scheduler,
		defaults:        defaults,
		validator:       validator,
		logger:          logger,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (*Response, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return &Response{ValidationErrors: validationErrors}, nil
	}

	selectedNode, err := uc.pickNode(ctx)
	if err != nil {
		return nil, err
	}

	stackSlug, err := slug.Generate(request.Name)
	if err != nil {
		return nil, err
	}

	s := stack.Stack{
		Name:      request.Name,
		Slug:      stackSlug,
		NodeName:  selectedNode.Name,
		OwnerUUID: request.OwnerUUID,
	}

	uuid, err := uc.stackRepository.Save(ctx, &s)
	if err != nil {
		return nil, err
	}

	// sorted, so a stack's services are always created in the same order and
	// a failure part-way through is reproducible.
	names := make([]string, 0, len(request.Services))
	for name := range request.Services {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
		service := request.Services[name]

		serviceRequest := runTask.FromSpec(request.Name+"-"+name, &service, uc.defaults)
		serviceRequest.StackUUID = uuid
		serviceRequest.StackSlug = stackSlug
		serviceRequest.ServiceName = name
		serviceRequest.NominatedNode = selectedNode.Name
		serviceRequest.OwnerUUID = request.OwnerUUID

		response, err := uc.runTask.Execute(ctx, serviceRequest)
		if err != nil {
			return nil, err
		}

		if response != nil && len(response.ValidationErrors) > 0 {
			// the spec was validated before anything was created, so a service
			// rejected here is a mistake in the runner rather than in what was
			// asked for.
			uc.logger.ErrorContext(ctx, "a validated service was rejected", "stack", uuid, "service", name, "errors", response.ValidationErrors)

			return &Response{ValidationErrors: response.ValidationErrors}, nil
		}
	}

	return &Response{UUID: uuid, Slug: stackSlug}, nil
}

// pickNode chooses the one node a stack's services all run on.
func (uc *UseCase) pickNode(ctx context.Context) (node.Node, error) {
	nodes, err := uc.nodeRepository.GetAll(ctx, 0, nominatedNodesLimit)
	if err != nil {
		return node.Node{}, err
	}

	healthy := make([]node.Node, 0, len(nodes))
	for _, n := range nodes {
		if n.LastHeartbeatAt.After(time.Now().Add(-heartbeatGracePeriod)) {
			healthy = append(healthy, n)
		}
	}

	if len(healthy) == 0 {
		return node.Node{}, node.ErrNoNodesAvailable
	}

	return uc.scheduler.Pick(&task.Task{}, healthy), nil
}
