package contact

import (
	"context"
	"time"
)

// Message is a "contact us" message sent by a visitor. It is not tied to a user
// account — the sender identifies themselves by email, phone number, or both.
type Message struct {
	UUID      string
	Subject   string
	Body      string
	Email     string
	Phone     string
	ReadAt    time.Time
	CreatedAt time.Time
}

// IsRead reports whether the message has been marked as read.
func (m Message) IsRead() bool {
	return !m.ReadAt.IsZero()
}

type Repository interface {
	GetAll(ctx context.Context, offset uint, limit uint) ([]Message, error)
	GetOne(ctx context.Context, UUID string) (Message, error)
	Count(ctx context.Context) (uint, error)
	Save(ctx context.Context, m *Message) (string, error)
	Delete(ctx context.Context, UUID string) error
}
