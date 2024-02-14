package users

import (
	"time"
)

type UserBson struct {
	UUID      string    `bson:"_id,omitempty"`
	Name      string    `bson:"name,omitempty"`
	Avatar    string    `bson:"avatar,omitempty"`
	Username  string    `bson:"username,omitempty"`
	Password  string    `bson:"password,omitempty"`
	CreatedAt time.Time `bson:"created_at,omitempty"`
}

type SetWrapper struct {
	Set interface{} `bson:"$set,omitempty"`
}
