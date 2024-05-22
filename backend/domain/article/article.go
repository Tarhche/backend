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
	GetAllPublished(offset uint, limit uint) ([]Article, error)
	GetOne(UUID string) (Article, error)
	GetOnePublished(UUID string) (Article, error)
	GetByUUIDs(UUIDs []string) ([]Article, error)
	GetMostViewed(limit uint) ([]Article, error)
	GetByHashtag(hashtags []string, offset uint, limit uint) ([]Article, error)
	Count() (uint, error)
	CountPublished() (uint, error)
	Save(*Article) (string, error)
	Delete(UUID string) error
	IncreaseView(uuid string, inc uint) error
}
