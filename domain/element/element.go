package element

import (
	"context"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/domain/element/component"
)

// ErrUnSupportedComponent is an error that is returned when a component type is not supported.
var ErrUnSupportedComponent error = errors.New("unsupported component type")

// Component is an interface that represents a component of an element.
type Component interface {
	Items() []component.Item
	Type() string
}

// Element represents an element.
type Element struct {
	UUID      string
	Body      Component
	Venues    []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Repository represents a repository of elements.
type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]Element, error)
	GetOne(ctx context.Context, UUID string) (Element, error)
	Count(ctx context.Context) (uint, error)
	Save(ctx context.Context, e *Element) (string, error)
	Delete(ctx context.Context, UUID string) error
}
