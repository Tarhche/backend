package users

import (
	"time"
)

type UserBson struct {
	UUID         string           `bson:"_id,omitempty"`
	Name         string           `bson:"name"`
	Avatar       string           `bson:"avatar"`
	Email        string           `bson:"email"`
	Username     string           `bson:"username"`
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
