//go:build integration

package migrations

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/resources"
)

// TestTasksAsManifests converts the code runner's tasks kept as they were
// before a task was a kind, on the server MONGO_TEST_URI names, and reads them
// back as the task kind's repository does.
func TestTasksAsManifests(t *testing.T) {
	uri := os.Getenv("MONGO_TEST_URI")
	if len(uri) == 0 {
		t.Skip("MONGO_TEST_URI names no database to test against")
	}

	ctx := context.Background()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	require.NoError(t, err)

	database := client.Database(fmt.Sprintf("migrations_test_%s", time.Now().Format("150405000000")))
	t.Cleanup(func() {
		_ = database.Drop(context.Background())
		_ = client.Disconnect(context.Background())
	})

	created := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	// what a task was kept as before, field by field as its repository wrote
	// it: what a running snippet's and an ended one's are.
	old := func(uuid string, slug string, state int, expected int, changes ...bson.E) bson.D {
		document := bson.D{
			{Key: "_id", Value: uuid}, {Key: "name", Value: "request-" + uuid}, {Key: "slug", Value: slug}, {Key: "kind", Value: "job"},
			{Key: "current_state", Value: state}, {Key: "expected_state", Value: expected},
			{Key: "last_heartbeat_at", Value: created.Add(3 * time.Second)},
			{Key: "image", Value: "ghcr.io/tarhche/code-runner:go-1.24-latest"},
			{Key: "exposed_ports", Value: bson.A{int32(3000)}},
			{Key: "network_policy", Value: "isolated"},
			{Key: "endpoints", Value: bson.A{bson.D{{Key: "container_port", Value: int32(3000)}, {Key: "host_port", Value: int32(20000)}}}},
			{Key: "command", Value: bson.A{"--timeout", "120", "serve"}},
			{Key: "interactive", Value: true},
			{Key: "max_retries", Value: int32(0)},
			{Key: "ttl", Value: int64(2 * time.Minute)},
			{Key: "deadline", Value: created.Add(time.Second + 2*time.Minute)},
			{Key: "resource_limits", Value: bson.D{{Key: "cpu", Value: 2.0}, {Key: "memory", Value: int64(512 << 20)}, {Key: "disk", Value: int64(512 << 20)}}},
			{Key: "node_name", Value: "workload-orchestrator-01"},
			{Key: "container_logs", Value: []byte("listening\n")},
			{Key: "container_id", Value: "execution-" + uuid},
			{Key: "owner_uuid", Value: "guest"},
			{Key: "created_at", Value: created},
			{Key: "started_at", Value: created.Add(time.Second)},
		}

		for _, change := range changes {
			replaced := false

			for i := range document {
				if document[i].Key == change.Key {
					document[i].Value, replaced = change.Value, true
				}
			}

			if !replaced {
				document = append(document, change)
			}
		}

		return document
	}

	tasks := database.Collection("tasks")

	// an index of the old shape, on a field a manifest does not have.
	_, err = tasks.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "slug", Value: 1}}, Options: options.Index().SetUnique(true)})
	require.NoError(t, err)

	_, err = tasks.InsertMany(ctx, []any{
		old("running-uuid", "request-running-abcde", 3, 3),
		old("completed-uuid", "request-completed-fghij", 6, 6, bson.E{Key: "finished_at", Value: created.Add(5 * time.Second)}),
		old("scheduled-uuid", "request-scheduled-klmno", 2, 3),
	})
	require.NoError(t, err)

	require.NoError(t, tasksAsManifests.Up(ctx, database))
	require.NoError(t, tasksAsManifests.Up(ctx, database), "run again, it changes nothing")

	indexes, err := tasks.Indexes().List(ctx)
	require.NoError(t, err)

	var names []string
	for indexes.Next(ctx) {
		names = append(names, indexes.Current.Lookup("name").StringValue())
	}

	assert.NotContains(t, names, "slug_1")
	assert.Contains(t, names, "metadata.slug_1")
	assert.Contains(t, names, "metadata.node_1")

	// read where the migrations after this one leave them, workloads, as
	// `app migrate` goes on to apply them.
	migrateAfter(t, database, tasksAsManifests.Name)

	repository := resources.NewRepository(database)
	require.NoError(t, repository.EnsureKind(ctx, taskKind.Descriptor()), "the control plane indexes it as it is")

	read := func(uuid string) (resource.Record, taskKind.Task) {
		t.Helper()

		r, err := repository.GetOne(ctx, taskKind.Name, uuid)
		require.NoError(t, err)

		converted, err := kind.Decode[taskKind.Spec, taskKind.Status](r.Raw)
		require.NoError(t, err)

		return r, converted
	}

	r, running := read("running-uuid")

	none := 0

	assert.Equal(t, int64(1), r.Version)
	assert.Equal(t, "request-running-uuid", running.Metadata.Name)
	assert.Equal(t, "request-running-abcde", running.Metadata.Slug)
	assert.Equal(t, task.GuestOwnerUUID, running.Metadata.OwnerUUID)
	assert.Equal(t, "workload-orchestrator-01", running.Metadata.Node)
	assert.Equal(t, created, running.Metadata.CreatedAt)
	assert.Equal(t, taskKind.Spec{
		Kind:          task.KindJob,
		Image:         "ghcr.io/tarhche/code-runner:go-1.24-latest",
		Command:       []string{"--timeout", "120", "serve"},
		Ports:         []port.Port{3000},
		NetworkPolicy: network.PolicyIsolated,
		Interactive:   true,
		TTL:           2 * time.Minute,
		Limits:        taskKind.Limits{CPU: 2, Memory: 512 << 20, Disk: 512 << 20},
		MaxRetries:    &none,
	}, running.Spec)
	assert.Equal(t, taskKind.Running, running.Status.State)
	assert.Equal(t, taskKind.Running, running.Status.Expected)
	assert.Equal(t, created.Add(3*time.Second), running.Status.ObservedAt)
	require.NotNil(t, running.Status.Run)
	assert.Equal(t, taskKind.Run{
		ID:          "execution-running-uuid",
		Name:        "request-running-uuid",
		Slug:        "request-running-abcde",
		Kind:        task.KindJob,
		Interactive: true,
		StartedAt:   created.Add(time.Second),
		Deadline:    created.Add(time.Second + 2*time.Minute),
		Output:      "listening\n",
	}, *running.Status.Run)

	_, completed := read("completed-uuid")
	assert.Equal(t, taskKind.Completed, completed.Status.State, "and is deleted, as a job that ended is")
	assert.Equal(t, created.Add(5*time.Second), completed.Status.Since)

	_, scheduled := read("scheduled-uuid")
	assert.Equal(t, taskKind.Created, scheduled.Status.State, "run, which its node does by taking the run there")
	assert.Equal(t, taskKind.Running, scheduled.Status.Expected)

	held, total, err := repository.GetAll(ctx, taskKind.Name, resource.Filter{Node: "workload-orchestrator-01", OwnerUUID: task.GuestOwnerUUID}, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, uint(3), total)
	assert.Len(t, held, 3)

	bySlug, err := repository.GetOneBySlug(ctx, taskKind.Name, "request-running-abcde")
	require.NoError(t, err)
	assert.Equal(t, "running-uuid", bySlug.Metadata.UUID)

	// a task admitted after them, with a slug of its own, is kept beside them.
	// One with a slug taken is refused.
	_, err = repository.Create(ctx, resource.Record{Raw: kind.Raw{Kind: taskKind.Name, Metadata: kind.Metadata{Slug: "request-new-uvwxy", OwnerUUID: task.GuestOwnerUUID}}})
	require.NoError(t, err)

	_, err = repository.Create(ctx, resource.Record{Raw: kind.Raw{Kind: taskKind.Name, Metadata: kind.Metadata{Slug: "request-another-zabcd", OwnerUUID: task.GuestOwnerUUID}}})
	require.NoError(t, err)

	_, err = repository.Create(ctx, resource.Record{Raw: kind.Raw{Kind: taskKind.Name, Metadata: kind.Metadata{Slug: "request-running-abcde", OwnerUUID: task.GuestOwnerUUID}}})
	assert.Error(t, err)
}
