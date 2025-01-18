package runner

import (
	"context"
)

type TasksDefinition struct {
	Tasks []Task
}

type Task struct {
	Name           string
	Image          string
	Command        []string
	Cleanup        bool
	Resources      Resources
	WaitToComplete bool
}

type Resources struct {
	Limits       ResourceLimits
	Reservations ResourceReservations
}

type ResourceLimits struct {
	CPUs   string
	Memory string
}

type ResourceReservations struct {
	CPUs   string
	Memory string
}

type Runner interface {
	Run(ctx context.Context, isDone chan<- bool)
}

type ContainerManager interface {
	CreateContainer(ctx context.Context, task Task) (string, error)
	StartContainer(ctx context.Context, id string) error
	WaitForContainer(ctx context.Context, id string) (bool, error)
	RemoveContainer(ctx context.Context, id string) error
}

type ImageManager interface {
	PullImage(ctx context.Context, image string) error
}
