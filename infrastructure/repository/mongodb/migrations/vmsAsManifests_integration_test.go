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
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/repository/mongodb/workload/resources"
)

// TestVMsAsManifests converts VMs kept as they were before a VM was a kind,
// on the server MONGO_TEST_URI names, and reads them back as the vm kind's
// repository does.
func TestVMsAsManifests(t *testing.T) {
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

	// what the VMs were kept as, and indexed by, before.
	require.NoError(t, indexWorkloadVMs.Up(ctx, database))

	created := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

	old := func(uuid string, slug string, kind string, state int, expected int, changes ...bson.E) bson.D {
		document := bson.D{
			{Key: "_id", Value: uuid}, {Key: "name", Value: "box"}, {Key: "slug", Value: slug}, {Key: "owner_uuid", Value: "owner-uuid"},
			{Key: "kind", Value: kind}, {Key: "image", Value: "ubuntu:24.04"},
			{Key: "resources", Value: bson.D{{Key: "cpus", Value: 2}, {Key: "memory", Value: int64(2 << 30)}, {Key: "disk", Value: int64(20 << 30)}}},
			{Key: "ports", Value: bson.A{22, 8080}},
			{Key: "network", Value: bson.D{{Key: "ingress", Value: "allow"}, {Key: "egress", Value: "deny"}}},
			{Key: "persistent_disk", Value: true},
			{Key: "lifetime", Value: int64(time.Hour)},
			{Key: "expires_at", Value: created.Add(time.Hour)},
			{Key: "current_state", Value: state}, {Key: "expected_state", Value: expected},
			{Key: "reason", Value: ""}, {Key: "node_name", Value: "workload-orchestrator-01"},
			{Key: "stats", Value: bson.D{{Key: "cpu_percent", Value: 12.5}, {Key: "memory_used", Value: int64(256 << 20)}, {Key: "sampled_at", Value: created.Add(time.Minute)}}},
			{Key: "restore_from", Value: ""},
			{Key: "last_heartbeat_at", Value: created.Add(2 * time.Minute)},
			{Key: "created_at", Value: created}, {Key: "started_at", Value: created.Add(time.Second)}, {Key: "updated_at", Value: created.Add(time.Second)},
		}

		for _, change := range changes {
			for i := range document {
				if document[i].Key == change.Key {
					document[i].Value = change.Value
				}
			}
		}

		return document
	}

	_, err = database.Collection("vms").InsertMany(ctx, []any{
		old("running-uuid", "box-abcde", "machine", 4, 4),
		old("docker-uuid", "docker-fghij", "docker", 6, 6),
		old("scheduled-uuid", "made-klmno", "machine", 2, 4, bson.E{Key: "restore_from", Value: "snapshot-uuid"}),
		old("deleting-uuid", "gone-pqrst", "machine", 10, 10),
	})
	require.NoError(t, err)

	require.NoError(t, vmsAsManifests.Up(ctx, database))
	require.NoError(t, vmsAsManifests.Up(ctx, database), "run again, it changes nothing")

	repository := resources.NewRepository(database)
	require.NoError(t, repository.EnsureKind(ctx, vmKind.Descriptor()), "the control plane indexes it as it is")

	read := func(uuid string) (resource.Record, vmKind.VM) {
		t.Helper()

		r, err := repository.GetOne(ctx, vmKind.Name, uuid)
		require.NoError(t, err)

		v, err := kind.Decode[vmKind.Spec, vmKind.Status](r.Raw)
		require.NoError(t, err)

		return r, v
	}

	r, running := read("running-uuid")

	assert.Equal(t, int64(1), r.Version)
	assert.Equal(t, "box", running.Metadata.Name)
	assert.Equal(t, "box-abcde", running.Metadata.Slug)
	assert.Equal(t, "owner-uuid", running.Metadata.OwnerUUID)
	assert.Equal(t, map[string]string{vmKind.LabelFlavor: "machine"}, running.Metadata.Labels)
	assert.Equal(t, "workload-orchestrator-01", running.Metadata.Node)
	assert.Equal(t, time.Hour, running.Metadata.Lifetime)
	assert.Equal(t, created.Add(time.Hour), running.Metadata.ExpiresAt)
	assert.Equal(t, created, running.Metadata.CreatedAt)
	assert.Equal(t, vmKind.Spec{
		Flavor:         vmKind.FlavorMachine,
		Image:          "ubuntu:24.04",
		Resources:      vmKind.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
		Ports:          []port.Port{22, 8080},
		Network:        vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
		PersistentDisk: true,
	}, running.Spec)
	assert.Equal(t, vmKind.Running, running.Status.State)
	assert.Equal(t, vmKind.Running, running.Status.Expected)
	assert.Equal(t, created.Add(time.Second), running.Status.Since)
	assert.Equal(t, created.Add(2*time.Minute), running.Status.ObservedAt)
	assert.Equal(t, created.Add(time.Second), running.Status.StartedAt)
	require.NotNil(t, running.Status.Stats)
	assert.Equal(t, 12.5, running.Status.Stats.CPUPercent)
	require.NotNil(t, running.Status.Applied)
	assert.True(t, running.Status.Applied.Equal(running.Spec.Config()), "nothing is reconfigured for being converted")

	_, scheduled := read("scheduled-uuid")
	assert.Equal(t, vmKind.Created, scheduled.Status.State, "made, from what it was to be made from")
	assert.Equal(t, &vmKind.Source{Snapshot: "snapshot-uuid"}, scheduled.Spec.Source)
	assert.Nil(t, scheduled.Status.Applied)

	_, deleting := read("deleting-uuid")
	assert.Equal(t, vmKind.Failed, deleting.Status.State)
	assert.Equal(t, vmKind.Deleted, deleting.Status.Expected, "its delete is asked for again")

	docker, total, err := repository.GetAll(ctx, vmKind.Name, resource.Filter{OwnerUUID: "owner-uuid", Labels: map[string]string{vmKind.LabelFlavor: "docker"}}, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, uint(1), total, "a person's docker vms are found by their flavor")
	require.Len(t, docker, 1)
	assert.Equal(t, "docker-uuid", docker[0].Metadata.UUID)

	held, total, err := repository.GetAll(ctx, vmKind.Name, resource.Filter{Node: "workload-orchestrator-01"}, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, uint(4), total)
	assert.Len(t, held, 4)

	// a vm admitted after them, with a slug of its own, is kept beside them:
	// the old unique slug is gone. One with a slug taken is refused.
	_, err = repository.Create(ctx, resource.Record{Raw: kind.Raw{Kind: vmKind.Name, Metadata: kind.Metadata{Slug: "new-uvwxy", OwnerUUID: "owner-uuid"}}})
	require.NoError(t, err)

	_, err = repository.Create(ctx, resource.Record{Raw: kind.Raw{Kind: vmKind.Name, Metadata: kind.Metadata{Slug: "another-zabcd", OwnerUUID: "owner-uuid"}}})
	require.NoError(t, err)

	_, err = repository.Create(ctx, resource.Record{Raw: kind.Raw{Kind: vmKind.Name, Metadata: kind.Metadata{Slug: "box-abcde", OwnerUUID: "other"}}})
	assert.Error(t, err)

	indexes, err := database.Collection("vms").Indexes().List(ctx)
	require.NoError(t, err)

	var names []string
	for indexes.Next(ctx) {
		names = append(names, indexes.Current.Lookup("name").StringValue())
	}

	for _, gone := range []string{"slug_1", "owner_uuid_1__id_-1", "owner_uuid_1_kind_1", "node_name_1"} {
		assert.NotContains(t, names, gone)
	}

	assert.Contains(t, names, "metadata.slug_1")
	assert.Contains(t, names, "metadata.node_1")
}
