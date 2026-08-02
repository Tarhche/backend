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
	CreatedAt    time.Time        `bson:"created_at,omitempty"`
	// Nullable rather than a zero time, so lifting a ban writes an explicit
	// null instead of leaving the previous date behind.
	BannedAt *time.Time `bson:"banned_at"`
}

type PasswordHashBson struct {
	Value []byte `bson:"value,omitempty"`
	Salt  []byte `bson:"salt,omitempty"`
}
