package roles

import (
	"time"
)

type RoleBson struct {
	UUID        string    `bson:"_id,omitempty"`
	Name        string    `bson:"name,omitempty"`
	Description string    `bson:"description,omitempty"`
	Permissions []string  `bson:"permissions,omitempty"`
	UserUUIDs   []string  `bson:"user_uuids,omitempty"`
	CreatedAt   time.Time `bson:"created_at,omitempty"`
	UpdatedAt   time.Time `bson:"updated_at,omitempty"`
}

type SetWrapper struct {
	Set interface{} `bson:"$set,omitempty"`
}
