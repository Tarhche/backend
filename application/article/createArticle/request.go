package createarticle

import (
	"time"
)

type validationErrors map[string]string

type Request struct {
	Cover       string    `json:"cover"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	AuthorUUID  string    `json:"author_uuid"`
}

func (r *Request) Validate() (bool, validationErrors) {
	return true, nil
}
