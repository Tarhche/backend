package article

import (
	"time"

	"github.com/khanzadimahdi/testproject/domain/author"
)

type Article struct {
	UUID        string
	Cover       string
	Title       string
	Excerpt     string
	Body        string
	PublishedAt time.Time
	Author      author.Author
	Tags        []string
	ViewCount   uint
}

type Repository interface {
	GetAll(offset uint, limit uint) ([]Article, error)
	GetOne(UUID string) (Article, error)
	Count() (uint, error)
	Save(*Article) (string, error)
	Delete(UUID string) error
	IncreaseView(uuid string, inc uint) error
}
