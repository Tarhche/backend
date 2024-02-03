package elements

import (
	"time"
)

type ElementBson struct {
	UUID      string    `bson:"_id,omitempty"`
	Type      string    `bson:"type,omitempty"`
	Body      any       `bson:"body,omitempty"`
	Venues    []string  `bson:"venues,omitempty"`
	CreatedAt time.Time `bson:"created_at,omitempty"`
	UpdatedAt time.Time `bson:"updated_at,omitempty"`
}

type SetWrapper struct {
	Set interface{} `bson:"$set,omitempty"`
}
