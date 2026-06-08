package articles

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/article"
	"github.com/khanzadimahdi/testproject/domain/language"
)

const (
	collectionName = "articles"
	queryTimeout   = 3 * time.Second
)

type ArticlesRepository struct {
	collection *mongo.Collection
}

var _ article.Repository = &ArticlesRepository{}

func NewRepository(database *mongo.Database) *ArticlesRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &ArticlesRepository{
		collection: database.Collection(collectionName),
	}
}

func toDomain(a ArticleBson) article.Article {
	return article.Article{
		UUID:            a.UUID,
		Cover:           a.Cover,
		Video:           a.Video,
		Title:           a.Title,
		Excerpt:         a.Excerpt,
		Body:            a.Body,
		PublishedAt:     a.PublishedAt,
		AuthorUUID:      a.AuthorUUID,
		Tags:            a.Tags,
		ViewCount:       a.ViewCount,
		LanguageCode:    a.LanguageCode,
		CorrelationUUID: a.CorrelationUUID,
	}
}

func (r *ArticlesRepository) GetAll(offset uint, limit uint) ([]article.Article, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "_id", Value: -1}}

	cur, err := r.collection.Find(ctx, bson.D{}, options.Find().SetLimit(l).SetSkip(o).SetSort(desc))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]article.Article, 0, limit)
	for cur.Next(ctx) {
		var a ArticleBson

		if err := cur.Decode(&a); err != nil {
			return nil, err
		}
		items = append(items, toDomain(a))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *ArticlesRepository) GetAllPublished(language string, offset uint, limit uint) ([]article.Article, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "published_at", Value: -1}}

	filter := bson.M{
		"published_at": publishedFilter(),
	}
	if len(language) > 0 {
		filter["language_code"] = language
	}

	cur, err := r.collection.Find(ctx, filter, options.Find().SetLimit(l).SetSkip(o).SetSort(desc))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]article.Article, 0, limit)
	for cur.Next(ctx) {
		var a ArticleBson

		if err := cur.Decode(&a); err != nil {
			return nil, err
		}
		items = append(items, toDomain(a))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *ArticlesRepository) GetByCorrelationUUIDs(correlationUUIDs []string, languageCode string) ([]article.Article, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	desc := bson.D{{Key: "published_at", Value: -1}}
	filter := bson.M{"correlation_uuid": bson.M{"$in": correlationUUIDs}}
	if len(languageCode) > 0 {
		filter["language_code"] = languageCode
	}

	cur, err := r.collection.Find(ctx, filter, options.Find().SetSort(desc))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]article.Article, 0, len(correlationUUIDs))
	for cur.Next(ctx) {
		var a ArticleBson

		if err := cur.Decode(&a); err != nil {
			return nil, err
		}
		items = append(items, toDomain(a))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *ArticlesRepository) GetPublishedLanguages(correlationUUID string) ([]language.Language, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if len(correlationUUID) == 0 {
		return []language.Language{}, nil
	}

	filter := bson.M{
		"correlation_uuid": correlationUUID,
		"published_at":     publishedFilter(),
	}

	var languageCodes []string
	if err := r.collection.Distinct(ctx, "language_code", filter).Decode(&languageCodes); err != nil {
		return nil, err
	}

	languages := make([]language.Language, 0, len(languageCodes))
	for _, code := range languageCodes {
		languages = append(languages, language.Language{Code: code})
	}

	return languages, nil
}

func (r *ArticlesRepository) CorrelationExist(correlationUUID string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if len(correlationUUID) == 0 {
		return false, nil
	}

	filter := bson.M{"correlation_uuid": correlationUUID}

	c, err := r.collection.CountDocuments(ctx, filter, options.Count().SetLimit(1))
	if err != nil {
		return false, err
	}

	return c > 0, nil
}

func (r *ArticlesRepository) GetMostViewed(language string, limit uint) ([]article.Article, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	l := int64(limit)
	desc := bson.D{{Key: "view_count", Value: -1}}
	filter := bson.M{
		"published_at": publishedFilter(),
	}
	if len(language) > 0 {
		filter["language_code"] = language
	}

	cur, err := r.collection.Find(ctx, filter, options.Find().SetLimit(l).SetSort(desc))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]article.Article, 0, limit)
	for cur.Next(ctx) {
		var a ArticleBson

		if err := cur.Decode(&a); err != nil {
			return nil, err
		}
		items = append(items, toDomain(a))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *ArticlesRepository) CountPublishedByHashtags(hashtags []string, language string) (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.M{
		"tags":         bson.M{"$in": hashtags},
		"published_at": publishedFilter(),
	}
	if len(language) > 0 {
		filter["language_code"] = language
	}

	c, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *ArticlesRepository) GetPublishedByHashtags(hashtags []string, language string, offset uint, limit uint) ([]article.Article, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "published_at", Value: -1}}
	filter := bson.M{
		"tags":         bson.M{"$in": hashtags},
		"published_at": publishedFilter(),
	}
	if len(language) > 0 {
		filter["language_code"] = language
	}

	cur, err := r.collection.Find(
		ctx,
		filter,
		options.Find().SetLimit(l).SetSkip(o).SetSort(desc),
	)

	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]article.Article, 0, limit)
	for cur.Next(ctx) {
		var a ArticleBson

		if err := cur.Decode(&a); err != nil {
			return nil, err
		}
		items = append(items, toDomain(a))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *ArticlesRepository) CountPublishedByAuthor(authorUUID string, language string) (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.M{
		"author_uuid":  authorUUID,
		"published_at": publishedFilter(),
	}
	if len(language) > 0 {
		filter["language_code"] = language
	}

	c, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *ArticlesRepository) GetPublishedByAuthor(authorUUID string, language string, offset uint, limit uint) ([]article.Article, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "published_at", Value: -1}}
	filter := bson.M{
		"author_uuid":  authorUUID,
		"published_at": publishedFilter(),
	}
	if len(language) > 0 {
		filter["language_code"] = language
	}

	cur, err := r.collection.Find(
		ctx,
		filter,
		options.Find().SetLimit(l).SetSkip(o).SetSort(desc),
	)

	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]article.Article, 0, limit)
	for cur.Next(ctx) {
		var a ArticleBson

		if err := cur.Decode(&a); err != nil {
			return nil, err
		}
		items = append(items, toDomain(a))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *ArticlesRepository) GetOne(UUID string) (article.Article, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.D{{Key: "_id", Value: UUID}}

	var a ArticleBson
	if err := r.collection.FindOne(ctx, filter).Decode(&a); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return article.Article{}, err
	}

	return toDomain(a), nil
}

func (r *ArticlesRepository) GetOnePublished(correlationUUID string, languageCode string) (article.Article, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.M{
		"correlation_uuid": correlationUUID,
		"published_at":     publishedFilter(),
	}
	if len(languageCode) > 0 {
		filter["language_code"] = languageCode
	}

	var a ArticleBson
	if err := r.collection.FindOne(ctx, filter).Decode(&a); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return article.Article{}, err
	}

	return toDomain(a), nil
}

func (r *ArticlesRepository) Count() (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{})
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *ArticlesRepository) CountPublished(language string) (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.M{
		"published_at": publishedFilter(),
	}
	if len(language) > 0 {
		filter["language_code"] = language
	}

	c, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *ArticlesRepository) Save(a *article.Article) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if len(a.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		a.UUID = UUID.String()
	}

	if len(a.CorrelationUUID) == 0 {
		a.CorrelationUUID = a.UUID
	}

	update := ArticleBson{
		UUID:            a.UUID,
		Cover:           a.Cover,
		Title:           a.Title,
		Video:           a.Video,
		Excerpt:         a.Excerpt,
		Body:            a.Body,
		PublishedAt:     a.PublishedAt,
		AuthorUUID:      a.AuthorUUID,
		Tags:            a.Tags,
		ViewCount:       a.ViewCount,
		LanguageCode:    a.LanguageCode,
		CorrelationUUID: a.CorrelationUUID,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if _, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: a.UUID}},
		bson.M{"$set": update},
		options.UpdateOne().SetUpsert(true),
	); err != nil {
		return "", err
	}

	return a.UUID, nil
}

func (r *ArticlesRepository) Delete(UUID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: UUID}})

	return err
}

func (r *ArticlesRepository) IncreaseView(uuid string, inc uint) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: uuid}},
		bson.D{{Key: "$inc", Value: bson.D{{Key: "view_count", Value: inc}}}},
	)

	return err
}

func publishedFilter() bson.M {
	return bson.M{
		"$lte": bson.NewDateTimeFromTime(time.Now()),
		"$ne":  time.Time{},
	}
}
