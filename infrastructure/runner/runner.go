package runner

import (
	"context"
	"fmt"

	runnerDomain "github.com/khanzadimahdi/testproject/domain/runner"
)

type runner struct {
	containerManager runnerDomain.ContainerManager
	imageManager     runnerDomain.ImageManager
}

var _ runnerDomain.Runner = &runner{}

func NewRunner(
	containerManager runnerDomain.ContainerManager,
	imageManager runnerDomain.ImageManager,
) *runner {
	return &runner{
		containerManager: containerManager,
		imageManager:     imageManager,
	}
}

func (r *runner) Run(ctx context.Context, tasksDefinition runnerDomain.TasksDefinition) error {
	for _, task := range tasksDefinition.Tasks {
		if err != r.run(ctx, task); err != nil {
			return err
		}
	}
}

func (r *runner) run(ctx context.Context, task runnerDomain.Task) error {
	fmt.Println("preparing task - ", task.Name)
	if err := r.imageManager.PullImage(ctx, task.Image); err != nil {
		return err
	}

	id, err := r.containerManager.CreateContainer(ctx, task)
	if err != nil {
		return err
	}

	fmt.Println("starting task - ", task.Name)
	err = r.containerManager.StartContainer(ctx, id)
	if err != nil {
		return err
	}

	statusSuccess, err := r.containerManager.WaitForContainer(ctx, id)
	if err != nil {
		return err
	}

	if statusSuccess {
		fmt.Println("completed task - ", task.Name)

		// cleanup by removing the task container
		if task.Cleanup {
			fmt.Println("cleanup task - ", task.Name)
			err = r.containerManager.RemoveContainer(ctx, id)
			if err != nil {
				fmt.Println(err)
			}
		}
	} else {
		fmt.Println("failed task - ", task.Name)
	}
}
