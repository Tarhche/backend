package updatearticle

import (
	"time"
)

type validationErrors map[string]string

type Request struct {
	UUID        string    `json:"uuid"`
	Cover       string    `json:"cover"`
	Video       string    `json:"video"`
	Title       string    `json:"title"`
	Excerpt     string    `json:"excerpt"`
	Body        string    `json:"body"`
	PublishedAt time.Time `json:"published_at"`
	AuthorUUID  string    `json:"-"`
	Tags        []string  `json:"tags"`
}

func (r *Request) Validate() (bool, validationErrors) {
	errors := make(validationErrors)

	if len(r.Title) == 0 {
		errors["title"] = "title is required"
	}

	if len(r.Excerpt) == 0 {
		errors["excerpt"] = "excerpt is required"
	}

	if len(r.Body) == 0 {
		errors["body"] = "body is required"
	}

	if len(r.AuthorUUID) == 0 {
		errors["author"] = "author is required"
	}

	return len(errors) == 0, errors
}
