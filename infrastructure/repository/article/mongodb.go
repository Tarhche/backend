package article

import (
	"context"
	"errors"
	"github.com/Tarhche/backend/domain/article"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

type MongoDBRepository struct {
	client        *mongo.Client
	uri, database string
}

func NewMongoDBRepository(uri string, database string) *MongoDBRepository {
	return &MongoDBRepository{
		uri:      uri,
		database: database,
	}
}

func (i *MongoDBRepository) Articles() ([]article.Entity, error) {
	client, disconnect, err := i.connect()
	if err != nil {
		return nil, err
	}
	defer disconnect()

	collection := client.Database("test").Collection("articles")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cur, err := collection.Find(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var articles []article.Entity
	for cur.Next(ctx) {
		var anArticle article.Entity
		if err := cur.Decode(&anArticle); err != nil {
			return nil, err
		}

		articles = append(articles, anArticle)
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return articles, nil
}

func (i *MongoDBRepository) CreateArticle(article *article.Entity) error {
	article.ID = uuid.NewString()

	client, disconnect, err := i.connect()
	if err != nil {
		return err
	}
	defer disconnect()

	collection := client.Database("test").Collection("articles")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = collection.InsertOne(ctx, article)

	return err
}

func (i *MongoDBRepository) Article(ID string) (*article.Entity, error) {
	client, disconnect, err := i.connect()
	if err != nil {
		return nil, err
	}
	defer disconnect()

	collection := client.Database("test").Collection("articles")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var anArticle article.Entity
	err = collection.FindOne(ctx, map[string]string{"id": ID}).Decode(&anArticle)
	if err == mongo.ErrNoDocuments {
		return nil, errors.New("article not found")
	} else if err != nil {
		return nil, err
	}

	return &anArticle, nil
}

func (i *MongoDBRepository) UpdateArticle(article *article.Entity) error {
	client, disconnect, err := i.connect()
	if err != nil {
		return err
	}
	defer disconnect()

	collection := client.Database("test").Collection("articles")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = collection.UpdateOne(ctx, map[string]string{"id": article.ID}, article)

	return err
}

func (i *MongoDBRepository) DeleteArticle(ID string) error {
	client, disconnect, err := i.connect()
	if err != nil {
		return err
	}
	defer disconnect()

	collection := client.Database("test").Collection("articles")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err = collection.DeleteOne(ctx, map[string]string{"id": ID})

	return err
}

func (i *MongoDBRepository) connect() (*mongo.Client, func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(i.uri))
	if err != nil {
		return nil, nil, err
	}

	disconnect := func() {
		if err = client.Disconnect(ctx); err != nil {
			panic(err)
		}
	}

	return client, disconnect, err
}
