package article

import (
	"time"
)

type Article struct {
	UUID        string
	Cover       string
	Video       string
	Title       string
	Excerpt     string
	Body        string
	PublishedAt time.Time
	AuthorUUID  string
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
	CountPublishedByHashtags(hashtags []string) (uint, error)
	GetPublishedByHashtags(hashtags []string, offset uint, limit uint) ([]Article, error)
	CountPublishedByAuthor(authorUUID string) (uint, error)
	GetPublishedByAuthor(authorUUID string, offset uint, limit uint) ([]Article, error)
	Count() (uint, error)
	CountPublished() (uint, error)
	Save(*Article) (string, error)
	Delete(UUID string) error
	IncreaseView(uuid string, inc uint) error
}
