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
	LanguageCode string           `bson:"language_code"`
	PasswordHash PasswordHashBson `bson:"hash,omitempty"`
	Identities   []IdentityBson   `bson:"identities,omitempty"`
	CreatedAt    time.Time        `bson:"created_at,omitempty"`
	BannedAt     time.Time        `bson:"banned_at"`
}

// IdentityBson is an account elsewhere that signs this user in. The pair is
// what a lookup matches on, so the same id issued by two providers is two
// different people.
type IdentityBson struct {
	Provider string `bson:"provider"`
	ID       string `bson:"id"`
}

type PasswordHashBson struct {
	Value []byte `bson:"value,omitempty"`
	Salt  []byte `bson:"salt,omitempty"`
}
