// Package getKinds says which kinds the control plane runs, as each
// describes itself: its states, its actions and the states each is allowed
// in, so that the dashboard offers exactly what a resource's state allows.
package getKinds

import (
	"context"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
)

type UseCase struct {
	registry *kind.Registry[kind.ControlPlaneBinding]
}

func NewUseCase(registry *kind.Registry[kind.ControlPlaneBinding]) *UseCase {
	return &UseCase{registry: registry}
}

// Execute is every kind registered, in the order they were.
func (uc *UseCase) Execute(context.Context) (*Response, error) {
	descriptors := uc.registry.Descriptors()
	if descriptors == nil {
		descriptors = []kind.Descriptor{}
	}

	return &Response{Items: descriptors}, nil
}
