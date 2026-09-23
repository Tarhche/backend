package attachTask

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

// UseCase opens a command inside a task's task and hands back the stream
// it runs on. Closing that stream releases it; ending the session is what stops
// the command.
type UseCase struct {
	taskManager task.Runtime
	validator   domain.Validator
}

func NewUseCase(taskManager task.Runtime, validator domain.Validator) *UseCase {
	return &UseCase{
		taskManager: taskManager,
		validator:   validator,
	}
}

func (uc *UseCase) Execute(ctx context.Context, request *Request) (task.ExecSession, domain.ValidationErrors, error) {
	if validationErrors := uc.validator.Validate(request); len(validationErrors) > 0 {
		return nil, validationErrors, nil
	}

	tasks, err := uc.taskManager.Of(ctx, request.UUID)
	if err != nil {
		return nil, nil, err
	}

	if len(tasks) == 0 {
		return nil, nil, domain.ErrNotExists
	}

	running := tasks[0]

	// whose it is, asked of the task itself. The node holding it wrote the
	// owner on when it created it, so it can answer this without a database and
	// without taking anybody's word for it.
	//
	// A task with no owner is a snippet: nobody's, and so everybody's. It
	// is a public playground, reachable by whoever finds it, and the terminal
	// is opened without asking who is knocking. An owner is what makes a
	// task private, and then it is private to exactly one person.
	if owner := running.OwnerUUID; len(owner) > 0 && owner != request.OwnerUUID {
		return nil, nil, domain.ErrNotExists
	}

	if running.Status != task.StatusRunning {
		return nil, domain.ValidationErrors{"uuid": "task_is_not_running"}, nil
	}

	session, err := uc.taskManager.Exec(ctx, running.ID, task.ExecOptions{
		Command: request.Shell(),
		TTY:     request.TTY,
	})
	if err != nil {
		return nil, nil, err
	}

	return session, nil, nil
}
