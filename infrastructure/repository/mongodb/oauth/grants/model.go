package grants

import "time"

type GrantBson struct {
	ID     string     `bson:"_id,omitempty"`
	Secret SecretBson `bson:"secret"`

	ClientID string `bson:"client_id"`
	UserUUID string `bson:"user_uuid"`

	RedirectURI string `bson:"redirect_uri"`
	Scope       string `bson:"scope,omitempty"`

	CodeChallenge       string `bson:"code_challenge"`
	CodeChallengeMethod string `bson:"code_challenge_method"`

	ExpiredAt time.Time `bson:"expired_at"`
	CreatedAt time.Time `bson:"created_at,omitempty"`
}

type SecretBson struct {
	Value []byte `bson:"value,omitempty"`
	Salt  []byte `bson:"salt,omitempty"`
}
