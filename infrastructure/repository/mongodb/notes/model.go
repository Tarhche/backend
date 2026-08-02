package notes

import (
	"time"
)

type NoteBson struct {
	UUID            string    `bson:"_id,omitempty"`
	Body            string    `bson:"body"`
	PublishedAt     time.Time `bson:"published_at"`
	AuthorUUID      string    `bson:"author_uuid"`
	Tags            []string  `bson:"tags"`
	LanguageCode    string    `bson:"language_code"`
	CorrelationUUID string    `bson:"correlation_uuid"`
	CreatedAt       time.Time `bson:"created_at,omitempty"`
	UpdatedAt       time.Time `bson:"updated_at,omitempty"`
}
