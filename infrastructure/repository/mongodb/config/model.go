package roles

import (
	"time"
)

type configBson struct {
	UUID                 string    `bson:"_id,omitempty"`
	Revision             uint      `bson:"revision"`
	UserDefaultRoleUUIDs []string  `bson:"user_default_role_uuids"`
	CreatedAt            time.Time `bson:"created_at,omitempty"`
}
