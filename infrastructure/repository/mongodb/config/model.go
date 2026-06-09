package roles

import (
	"time"
)

type configBson struct {
	UUID                 string    `bson:"_id,omitempty"`
	Revision             uint      `bson:"revision"`
	UserDefaultRoleUUIDs []string  `bson:"user_default_role_uuids"`
	DefaultLanguageCode  string    `bson:"default_language_code"`
	CreatedAt            time.Time `bson:"created_at,omitempty"`
}
