package clients

import "time"

type ClientBson struct {
	ID           string   `bson:"_id,omitempty"`
	Name         string   `bson:"name"`
	URI          string   `bson:"uri,omitempty"`
	RedirectURIs []string `bson:"redirect_uris"`

	GrantTypes              []string `bson:"grant_types"`
	ResponseTypes           []string `bson:"response_types"`
	TokenEndpointAuthMethod string   `bson:"token_endpoint_auth_method"`
	Scope                   string   `bson:"scope,omitempty"`

	Secret SecretBson `bson:"secret,omitempty"`

	CreatedAt time.Time `bson:"created_at,omitempty"`
}

type SecretBson struct {
	Value []byte `bson:"value,omitempty"`
	Salt  []byte `bson:"salt,omitempty"`
}
