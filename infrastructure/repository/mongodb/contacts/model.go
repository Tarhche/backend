package contacts

import (
	"time"
)

type ContactMessageBson struct {
	UUID      string    `bson:"_id,omitempty"`
	Subject   string    `bson:"subject"`
	Body      string    `bson:"body"`
	Email     string    `bson:"email,omitempty"`
	Phone     string    `bson:"phone,omitempty"`
	ReadAt    time.Time `bson:"read_at"`
	CreatedAt time.Time `bson:"created_at,omitempty"`
	UpdatedAt time.Time `bson:"updated_at,omitempty"`
}
