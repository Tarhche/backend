package roles

import (
	"time"
)

type RoleBson struct {
	UUID        string    `bson:"_id,omitempty"`
	Name        string    `bson:"name"`
	Description string    `bson:"description"`
	Permissions []string  `bson:"permissions"`
	UserUUIDs   []string  `bson:"user_uuids"`
	CreatedAt   time.Time `bson:"created_at,omitempty"`
	UpdatedAt   time.Time `bson:"updated_at,omitempty"`
}
