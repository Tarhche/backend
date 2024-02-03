package element

import (
	"time"
)

type Element struct {
	UUID      string
	Type      string
	Body      any
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
