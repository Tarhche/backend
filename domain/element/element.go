package element

import (
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
	GetAll(offset uint, limit uint) ([]Element, error)
	GetByVenues(Venues []string) ([]Element, error)
	GetOne(UUID string) (Element, error)
	Count() (uint, error)
	Save(*Element) (string, error)
	Delete(UUID string) error
}
