package notes

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/note"
)

const (
	collectionName = "notes"
	queryTimeout   = 3 * time.Second
)

type NotesRepository struct {
	collection *mongo.Collection
}

var _ note.Repository = &NotesRepository{}

func NewRepository(database *mongo.Database) *NotesRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &NotesRepository{
		collection: database.Collection(collectionName),
	}
}

func toDomain(n NoteBson) note.Note {
	return note.Note{
		UUID:            n.UUID,
		Body:            n.Body,
		PublishedAt:     n.PublishedAt,
		AuthorUUID:      n.AuthorUUID,
		Tags:            n.Tags,
		LanguageCode:    n.LanguageCode,
		CorrelationUUID: n.CorrelationUUID,
	}
}

func (r *NotesRepository) GetCorrelationUUIDs(ctx context.Context, authorUUID string, offset uint, limit uint) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	// One entry per correlation group, ordered by the newest note in the group
	// (its max _id, which is time-ordered) so paging is deterministic.
	// "$gt: \"\"" keeps only non-empty strings (excluding "", null and missing)
	// and, being a range predicate, can use an index unlike $ne/$nin/$expr.
	match := bson.M{"correlation_uuid": bson.M{"$gt": ""}}
	if len(authorUUID) > 0 {
		match["author_uuid"] = authorUUID
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: match}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: -1}}}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id":    "$correlation_uuid",
			"max_id": bson.M{"$first": "$_id"},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "max_id", Value: -1}}}},
		bson.D{{Key: "$skip", Value: int64(offset)}},
		bson.D{{Key: "$limit", Value: int64(limit)}},
	}

	cur, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	correlationUUIDs := make([]string, 0, limit)
	for cur.Next(ctx) {
		var doc struct {
			CorrelationUUID string `bson:"_id"`
		}

		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		correlationUUIDs = append(correlationUUIDs, doc.CorrelationUUID)
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return correlationUUIDs, nil
}

func (r *NotesRepository) CountByCorrelation(ctx context.Context, authorUUID string) (uint, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	match := bson.M{"correlation_uuid": bson.M{"$gt": ""}}
	if len(authorUUID) > 0 {
		match["author_uuid"] = authorUUID
	}

	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: match}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$correlation_uuid"}}},
		bson.D{{Key: "$count", Value: "count"}},
	}

	cur, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)

	var result struct {
		Count uint `bson:"count"`
	}
	if cur.Next(ctx) {
		if err := cur.Decode(&result); err != nil {
			return 0, err
		}
	}

	if err := cur.Err(); err != nil {
		return 0, err
	}

	return result.Count, nil
}

func (r *NotesRepository) GetByCorrelationUUIDs(ctx context.Context, correlationUUIDs []string, languageCode string) ([]note.Note, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
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

	items := make([]note.Note, 0, len(correlationUUIDs))
	for cur.Next(ctx) {
		var n NoteBson

		if err := cur.Decode(&n); err != nil {
			return nil, err
		}
		items = append(items, toDomain(n))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *NotesRepository) GetByCorrelationUUIDAndLanguage(ctx context.Context, correlationUUID string, languageCode string) (note.Note, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	filter := bson.M{"correlation_uuid": correlationUUID}
	if len(languageCode) > 0 {
		filter["language_code"] = languageCode
	}

	var n NoteBson
	if err := r.collection.FindOne(ctx, filter).Decode(&n); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return note.Note{}, err
	}

	return toDomain(n), nil
}

func (r *NotesRepository) GetOnePublished(ctx context.Context, correlationUUID string, languageCode string) (note.Note, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	filter := bson.M{
		"correlation_uuid": correlationUUID,
		"published_at":     publishedFilter(),
	}
	if len(languageCode) > 0 {
		filter["language_code"] = languageCode
	}

	var n NoteBson
	if err := r.collection.FindOne(ctx, filter).Decode(&n); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return note.Note{}, err
	}

	return toDomain(n), nil
}

func (r *NotesRepository) GetPublishedLanguageCodes(ctx context.Context, correlationUUID string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if len(correlationUUID) == 0 {
		return []string{}, nil
	}

	filter := bson.M{
		"correlation_uuid": correlationUUID,
		"published_at":     publishedFilter(),
	}

	languageCodes := make([]string, 0, 2)
	if err := r.collection.Distinct(ctx, "language_code", filter).Decode(&languageCodes); err != nil {
		return nil, err
	}

	return languageCodes, nil
}

func (r *NotesRepository) CountPublishedByHashtags(ctx context.Context, hashtags []string, languageCode string) (uint, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, publishedByHashtagsFilter(hashtags, languageCode))
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *NotesRepository) GetPublishedByHashtags(ctx context.Context, hashtags []string, languageCode string, offset uint, limit uint) ([]note.Note, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "published_at", Value: -1}}

	cur, err := r.collection.Find(
		ctx,
		publishedByHashtagsFilter(hashtags, languageCode),
		options.Find().SetLimit(l).SetSkip(o).SetSort(desc),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	items := make([]note.Note, 0, limit)
	for cur.Next(ctx) {
		var n NoteBson

		if err := cur.Decode(&n); err != nil {
			return nil, err
		}
		items = append(items, toDomain(n))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *NotesRepository) CountPublishedByAuthor(ctx context.Context, authorUUID string, languageCode string) (uint, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	filter := bson.M{
		"author_uuid":  authorUUID,
		"published_at": publishedFilter(),
	}
	if len(languageCode) > 0 {
		filter["language_code"] = languageCode
	}

	c, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

func (r *NotesRepository) GetPublishedByAuthor(ctx context.Context, authorUUID string, languageCode string, offset uint, limit uint) ([]note.Note, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "published_at", Value: -1}}
	filter := bson.M{
		"author_uuid":  authorUUID,
		"published_at": publishedFilter(),
	}
	if len(languageCode) > 0 {
		filter["language_code"] = languageCode
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

	items := make([]note.Note, 0, limit)
	for cur.Next(ctx) {
		var n NoteBson

		if err := cur.Decode(&n); err != nil {
			return nil, err
		}
		items = append(items, toDomain(n))
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *NotesRepository) CorrelationExist(ctx context.Context, correlationUUID string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
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

func (r *NotesRepository) Save(ctx context.Context, n *note.Note) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	if len(n.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		n.UUID = UUID.String()
	}

	update := NoteBson{
		UUID:            n.UUID,
		Body:            n.Body,
		PublishedAt:     n.PublishedAt,
		AuthorUUID:      n.AuthorUUID,
		Tags:            n.Tags,
		LanguageCode:    n.LanguageCode,
		CorrelationUUID: n.CorrelationUUID,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if _, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: n.UUID}},
		bson.M{"$set": update},
		options.UpdateOne().SetUpsert(true),
	); err != nil {
		return "", err
	}

	return n.UUID, nil
}

func (r *NotesRepository) DeleteByCorrelationUUIDAndLanguage(ctx context.Context, correlationUUID string, languageCode string) error {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	filter := bson.M{"correlation_uuid": correlationUUID}
	if len(languageCode) > 0 {
		filter["language_code"] = languageCode
	}

	_, err := r.collection.DeleteOne(ctx, filter)

	return err
}

func publishedByHashtagsFilter(hashtags []string, languageCode string) bson.M {
	filter := bson.M{
		"tags":         bson.M{"$in": hashtags},
		"published_at": publishedFilter(),
	}
	if len(languageCode) > 0 {
		filter["language_code"] = languageCode
	}
	return filter
}

func publishedFilter() bson.M {
	return bson.M{
		"$lte": bson.NewDateTimeFromTime(time.Now()),
		"$ne":  time.Time{},
	}
}
