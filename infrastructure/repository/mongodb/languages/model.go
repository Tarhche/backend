package languages

import (
	"time"
)

type LanguageBson struct {
	Code      string    `bson:"_id,omitempty"`
	Name      string    `bson:"name"`
	CreatedAt time.Time `bson:"created_at,omitempty"`
	UpdatedAt time.Time `bson:"updated_at,omitempty"`
}
