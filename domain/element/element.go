package element

import (
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/domain/element/component"
)

var ErrUnSupportedComponent error = errors.New("unsupported component type")

type Component interface {
	Items() []component.Item
}

type Element struct {
	UUID      string
	Type      string
	Body      Component
	Venues    []string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Repository interface {
	GetAll(offset uint, limit uint) ([]Element, error)
	GetByVenues(Venues []string) ([]Element, error)
	GetOne(UUID string) (Element, error)
	Count() (uint, error)
	Save(*Element) (string, error)
	Delete(UUID string) error
}
