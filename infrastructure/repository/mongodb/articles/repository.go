package articles

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/khanzadimahdi/testproject.git/domain"
	"github.com/khanzadimahdi/testproject.git/domain/article"
	"github.com/khanzadimahdi/testproject.git/domain/author"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	collectionName = "articles"
	queryTimeout   = 3 * time.Second
)

type ArticlesRepository struct {
	collection *mongo.Collection
}

var _ article.Repository = &ArticlesRepository{}

func NewArticlesRepository(database *mongo.Database) *ArticlesRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &ArticlesRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *ArticlesRepository) GetAll(offset uint, limit uint) ([]article.Article, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)

	cur, err := r.collection.Find(ctx, bson.D{}, &options.FindOptions{
		Skip:  &o,
		Limit: &l,
	})

	if err != nil {
		return nil, err
	}

	items := make([]article.Article, 0, limit)
	for cur.Next(ctx) {
		var a ArticleBson

		if err := cur.Decode(&a); err != nil {
			return nil, err
		}
		items = append(items, article.Article{
			UUID:        a.UUID,
			Cover:       a.Cover,
			Title:       a.Title,
			Body:        a.Body,
			PublishedAt: a.PublishedAt,
			Author: author.Author{
				UUID: a.AuthorUUID,
			},
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *ArticlesRepository) GetOne(UUID string) (article.Article, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var a ArticleBson
	if err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: UUID}}, nil).Decode(&a); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return article.Article{}, err
	}

	return article.Article{
		UUID:        a.UUID,
		Cover:       a.Cover,
		Title:       a.Title,
		Body:        a.Body,
		PublishedAt: a.PublishedAt,
		Author: author.Author{
			UUID: a.AuthorUUID,
		},
	}, nil
}

func (r *ArticlesRepository) Count() (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{}, nil)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *ArticlesRepository) Save(a *article.Article) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if len(a.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		a.UUID = UUID.String()
	}

	update := ArticleBson{
		UUID:        a.UUID,
		Cover:       a.Cover,
		Title:       a.Title,
		Body:        a.Body,
		PublishedAt: a.PublishedAt,
		AuthorUUID:  a.Author.UUID,
	}

	upsert := true
	result, err := r.collection.UpdateOne(ctx, bson.D{{Key: "_id", Value: a.UUID}}, SetWrapper{Set: update}, &options.UpdateOptions{
		Upsert: &upsert,
	})

	if err != nil {
		log.Println(err)
	} else {
		log.Println(result.UpsertedCount)
	}

	return err
}

func (r *ArticlesRepository) Delete(UUID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: UUID}}, nil)

	return err
}
