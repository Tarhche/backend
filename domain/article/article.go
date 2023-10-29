package article

import (
	"time"

	"github.com/khanzadimahdi/testproject.git/domain/author"
)

type Article struct {
	UUID        string
	Cover       string
	Title       string
	Body        string
	PublishedAt time.Time
	Author      author.Author
}

type Repository interface {
	GetAll(offset uint, limit uint) ([]Article, error)
	GetOne(UUID string) (Article, error)
	Count() (uint, error)
}
