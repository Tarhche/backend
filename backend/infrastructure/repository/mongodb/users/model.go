package users

import (
	"time"
)

type UserBson struct {
	UUID         string           `bson:"_id,omitempty"`
	Name         string           `bson:"name,omitempty"`
	Avatar       string           `bson:"avatar,omitempty"`
	Email        string           `bson:"email,omitempty"`
	Username     string           `bson:"username,omitempty"`
	PasswordHash PasswordHashBson `bson:"hash,omitempty"`
	CreatedAt    time.Time        `bson:"created_at,omitempty"`
}

type PasswordHashBson struct {
	Value []byte `bson:"value,omitempty"`
	Salt  []byte `bson:"salt,omitempty"`
}

type SetWrapper struct {
	Set interface{} `bson:"$set,omitempty"`
}
