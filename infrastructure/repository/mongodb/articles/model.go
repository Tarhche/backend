package articles

import (
	"time"
)

type ArticleBson struct {
	UUID        string    `bson:"_id,omitempty"`
	Cover       string    `bson:"cover"`
	Video       string    `bson:"video"`
	Title       string    `bson:"title"`
	Excerpt     string    `bson:"excerpt"`
	Body        string    `bson:"body"`
	PublishedAt time.Time `bson:"published_at"`
	AuthorUUID  string    `bson:"author_uuid"`
	Tags        []string  `bson:"tags"`
	ViewCount   uint      `bson:"view_count,omitempty"`
	CreatedAt   time.Time `bson:"created_at,omitempty"`
	UpdatedAt   time.Time `bson:"updated_at,omitempty"`
}
