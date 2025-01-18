package updatearticle

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain"
)

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

var _ domain.Validatable = &Request{}

func (r *Request) Validate() domain.ValidationErrors {
	validationErrors := make(domain.ValidationErrors)

	if len(r.Title) == 0 {
		validationErrors["title"] = "required_field"
	}

	if len(r.Excerpt) == 0 {
		validationErrors["excerpt"] = "required_field"
	}

	if len(r.Body) == 0 {
		validationErrors["body"] = "required_field"
	}

	if len(r.AuthorUUID) == 0 {
		validationErrors["author"] = "required_field"
	}

	return validationErrors
}
