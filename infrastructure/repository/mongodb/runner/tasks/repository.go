package tasks

import (
	"context"
	"errors"
	"time"

	"github.com/gofrs/uuid/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

const (
	collectionName = "tasks"
	queryTimeout   = 3 * time.Second
)

type TasksRepository struct {
	collection *mongo.Collection
}

var _ task.Repository = &TasksRepository{}

func NewRepository(database *mongo.Database) *TasksRepository {
	if database == nil {
		panic("database should not be nil")
	}

	return &TasksRepository{
		collection: database.Collection(collectionName),
	}
}

func (r *TasksRepository) GetAll(offset uint, limit uint) ([]task.Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	o := int64(offset)
	l := int64(limit)
	desc := bson.D{{Key: "_id", Value: -1}}

	cur, err := r.collection.Find(ctx, bson.D{}, &options.FindOptions{
		Skip:  &o,
		Limit: &l,
		Sort:  desc,
	})

	if err != nil {
		return nil, err
	}

	defer cur.Close(ctx)

	items := make([]task.Task, 0, limit)
	for cur.Next(ctx) {
		var t TaskBson

		if err := cur.Decode(&t); err != nil {
			return nil, err
		}
		items = append(items, task.Task{
			UUID:          t.UUID,
			Name:          t.Name,
			State:         task.State(t.State),
			Image:         t.Image,
			PortBindings:  t.PortBindings,
			RestartPolicy: t.RestartPolicy,
			RestartCount:  t.RestartCount,
			HealthCheck:   t.HealthCheck,
			AttachStdin:   t.AttachStdin,
			AttachStdout:  t.AttachStdout,
			AttachStderr:  t.AttachStderr,
			Environment:   t.Environment,
			Command:       t.Command,
			Entrypoint:    t.Entrypoint,
			Mounts:        convertMounts(t.Mounts),
			ResourceLimits: task.ResourceLimits{
				Cpu:    t.ResourceLimits.Cpu,
				Memory: t.ResourceLimits.Memory,
				Disk:   t.ResourceLimits.Disk,
			},
			ContainerID: t.ContainerID,
			OwnerUUID:   t.OwnerUUID,
			CreatedAt:   t.CreatedAt,
			StartedAt:   t.StartedAt,
			FinishedAt:  t.FinishedAt,
		})
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *TasksRepository) GetOne(UUID string) (task.Task, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	filter := bson.D{{Key: "_id", Value: UUID}}

	var t TaskBson
	if err := r.collection.FindOne(ctx, filter, nil).Decode(&t); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err = domain.ErrNotExists
		}
		return task.Task{}, err
	}

	return task.Task{
		UUID:          t.UUID,
		Name:          t.Name,
		State:         task.State(t.State),
		Image:         t.Image,
		PortBindings:  t.PortBindings,
		RestartPolicy: t.RestartPolicy,
		RestartCount:  t.RestartCount,
		HealthCheck:   t.HealthCheck,
		AttachStdin:   t.AttachStdin,
		AttachStdout:  t.AttachStdout,
		AttachStderr:  t.AttachStderr,
		Environment:   t.Environment,
		Command:       t.Command,
		Entrypoint:    t.Entrypoint,
		Mounts:        convertMounts(t.Mounts),
		ResourceLimits: task.ResourceLimits{
			Cpu:    t.ResourceLimits.Cpu,
			Memory: t.ResourceLimits.Memory,
			Disk:   t.ResourceLimits.Disk,
		},
		ContainerID: t.ContainerID,
		OwnerUUID:   t.OwnerUUID,
		CreatedAt:   t.CreatedAt,
		StartedAt:   t.StartedAt,
		FinishedAt:  t.FinishedAt,
	}, nil
}

func (r *TasksRepository) Save(t *task.Task) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if len(t.UUID) == 0 {
		UUID, err := uuid.NewV7()
		if err != nil {
			return "", err
		}
		t.UUID = UUID.String()
	}

	mounts := make([]Mount, len(t.Mounts))
	for i, m := range t.Mounts {
		mounts[i] = Mount{
			Source:   m.Source,
			Target:   m.Target,
			Type:     m.Type,
			ReadOnly: m.ReadOnly,
		}
	}

	update := TaskBson{
		UUID:          t.UUID,
		Name:          t.Name,
		State:         uint(t.State),
		Image:         t.Image,
		PortBindings:  t.PortBindings,
		RestartPolicy: t.RestartPolicy,
		RestartCount:  t.RestartCount,
		HealthCheck:   t.HealthCheck,
		AttachStdin:   t.AttachStdin,
		AttachStdout:  t.AttachStdout,
		AttachStderr:  t.AttachStderr,
		Environment:   t.Environment,
		Command:       t.Command,
		Entrypoint:    t.Entrypoint,
		Mounts:        mounts,
		ResourceLimits: ResourceLimits{
			Cpu:    t.ResourceLimits.Cpu,
			Memory: t.ResourceLimits.Memory,
			Disk:   t.ResourceLimits.Disk,
		},
		ContainerID: t.ContainerID,
		OwnerUUID:   t.OwnerUUID,
		CreatedAt:   time.Now(),
		StartedAt:   t.StartedAt,
		FinishedAt:  t.FinishedAt,
	}

	upsert := true
	if _, err := r.collection.UpdateOne(
		ctx,
		bson.D{{Key: "_id", Value: t.UUID}},
		bson.M{"$set": update},
		&options.UpdateOptions{Upsert: &upsert},
	); err != nil {
		return "", err
	}

	return t.UUID, nil
}

func (r *TasksRepository) Delete(UUID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	_, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: UUID}}, nil)

	return err
}

func (r *TasksRepository) Count() (uint, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	c, err := r.collection.CountDocuments(ctx, bson.D{}, nil)
	if err != nil {
		return uint(c), err
	}

	return uint(c), nil
}

// Convert from repository Mount to domain Mount
func convertMounts(mounts []Mount) []task.Mount {
	result := make([]task.Mount, len(mounts))
	for i, m := range mounts {
		result[i] = task.Mount{
			Source:   m.Source,
			Target:   m.Target,
			Type:     m.Type,
			ReadOnly: m.ReadOnly,
		}
	}
	return result
}
