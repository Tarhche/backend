package migrations

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// vmsAsManifests converts the VMs kept before a VM was a kind into the
// manifests the vm kind is kept as, in the same collection:
//
//	{_id, kind: "vm", metadata: {name, slug, owner_uuid, labels:
//	 {workload.flavor}, node, lifetime, expires_at, created_at, updated_at},
//	 spec: {flavor, image, resources, ports, network, persistent_disk,
//	 source: {snapshot}}, status: {state, expected, reason, since,
//	 observed_at, stats, endpoints, started_at, applied}, version: 1,
//	 control: {}}
//
// A VM keeps its uuid, its slug and its node, and is labelled with its
// flavor, which is what a person's Docker VMs are found by. Its lifetime is
// in nanoseconds, as every duration of a manifest is. What it was doing is
// said in the kind's words, and what an old command was in the middle of,
// which nothing is waiting on any more, is said so that the kind's own
// reconcile asks for it again: one being started is stopped and expected
// running, one being stopped is running and expected stopped, one restarted
// runs and is given its config again, one whose restore was cut short failed
// saying so, and one being deleted failed and is expected deleted. One that
// was never made is made from the snapshot it was to be made from.
//
// What its node last applied to a VM that was made is what it was given, so
// nothing is reconfigured for being converted; a VM that was restarting may
// have been in the middle of being given its config, so it is given it again.
//
// Its old indexes go first: the unique one on a slug that is not where a
// manifest keeps it would hold every converted VM to one slug, nothing. The
// indexes the kind is read by are made last, as the control plane makes them
// for every kind it registers.
//
// A VM already a manifest is left as it is, so this runs again as well as it
// ran the first time. The names of collections, fields and states are written
// out rather than taken from the repositories and the kind: this is what they
// were when it was written, and it has to keep meaning that however they
// change later.
var vmsAsManifests = Migration{
	Name: "2026-10-06-vms-as-manifests",
	Up: func(ctx context.Context, database *mongo.Database) error {
		vms := database.Collection("vms")

		for _, index := range []string{"slug_1", "owner_uuid_1__id_-1", "owner_uuid_1_kind_1", "node_name_1"} {
			if err := vms.Indexes().DropOne(ctx, index); err != nil && !indexNotFound(err) {
				return err
			}
		}

		cursor, err := vms.Find(ctx, bson.D{{Key: "metadata", Value: bson.D{{Key: "$exists", Value: false}}}})
		if err != nil {
			return err
		}

		var old []oldVM
		if err := cursor.All(ctx, &old); err != nil {
			return err
		}

		for _, v := range old {
			if _, err := vms.ReplaceOne(ctx, bson.D{{Key: "_id", Value: v.UUID}}, vmManifest(v)); err != nil {
				return err
			}
		}

		_, err = vms.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "metadata.slug", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
			{Keys: bson.D{{Key: "metadata.owner_uuid", Value: 1}, {Key: "_id", Value: -1}}},
			{Keys: bson.D{{Key: "metadata.node", Value: 1}}},
			{Keys: bson.D{{Key: "metadata.owners.uuid", Value: 1}, {Key: "metadata.owners.kind", Value: 1}}},
		})

		return err
	},
}

// oldVM is a VM as it was kept before a VM was a kind.
type oldVM struct {
	UUID           string        `bson:"_id"`
	Name           string        `bson:"name"`
	Slug           string        `bson:"slug"`
	OwnerUUID      string        `bson:"owner_uuid"`
	Kind           string        `bson:"kind"`
	Image          string        `bson:"image"`
	Resources      oldResources  `bson:"resources"`
	Ports          []int64       `bson:"ports"`
	Network        oldNetwork    `bson:"network"`
	PersistentDisk bool          `bson:"persistent_disk"`
	Lifetime       time.Duration `bson:"lifetime"`
	ExpiresAt      time.Time     `bson:"expires_at"`
	CurrentState   int           `bson:"current_state"`
	ExpectedState  int           `bson:"expected_state"`
	Reason         string        `bson:"reason"`
	NodeName       string        `bson:"node_name"`
	Stats          oldStats      `bson:"stats"`
	RestoreFrom    string        `bson:"restore_from"`

	LastHeartbeatAt time.Time `bson:"last_heartbeat_at"`
	CreatedAt       time.Time `bson:"created_at"`
	StartedAt       time.Time `bson:"started_at"`
	UpdatedAt       time.Time `bson:"updated_at"`
}

type oldResources struct {
	CPUs   int64 `bson:"cpus"`
	Memory int64 `bson:"memory"`
	Disk   int64 `bson:"disk"`
}

type oldNetwork struct {
	Ingress string `bson:"ingress"`
	Egress  string `bson:"egress"`
}

type oldStats struct {
	CPUPercent  float64   `bson:"cpu_percent"`
	MemoryUsed  int64     `bson:"memory_used"`
	MemoryLimit int64     `bson:"memory_limit"`
	DiskUsed    int64     `bson:"disk_used"`
	DiskTotal   int64     `bson:"disk_total"`
	NetworkRx   int64     `bson:"network_rx"`
	NetworkTx   int64     `bson:"network_tx"`
	SampledAt   time.Time `bson:"sampled_at"`
}

// vmManifest is an old VM as the vm kind keeps it.
func vmManifest(v oldVM) bson.D {
	state, expected, reason := vmStates(v.CurrentState, v.ExpectedState, v.Reason)

	metadata := bson.D{
		{Key: "name", Value: v.Name},
		{Key: "slug", Value: v.Slug},
		{Key: "owner_uuid", Value: v.OwnerUUID},
		{Key: "labels", Value: bson.D{{Key: "workload.flavor", Value: v.Kind}}},
	}

	if len(v.NodeName) > 0 {
		metadata = append(metadata, bson.E{Key: "node", Value: v.NodeName})
	}

	if v.Lifetime > 0 {
		metadata = append(metadata, bson.E{Key: "lifetime", Value: int64(v.Lifetime)})
	}

	if !v.ExpiresAt.IsZero() {
		metadata = append(metadata, bson.E{Key: "expires_at", Value: v.ExpiresAt})
	}

	metadata = append(metadata,
		bson.E{Key: "created_at", Value: v.CreatedAt},
		bson.E{Key: "updated_at", Value: v.UpdatedAt},
	)

	ports := bson.A{}
	for _, p := range v.Ports {
		ports = append(ports, p)
	}

	config := bson.D{
		{Key: "resources", Value: bson.D{
			{Key: "cpus", Value: v.Resources.CPUs},
			{Key: "memory", Value: v.Resources.Memory},
			{Key: "disk", Value: v.Resources.Disk},
		}},
		{Key: "ports", Value: ports},
		{Key: "network", Value: bson.D{
			{Key: "ingress", Value: v.Network.Ingress},
			{Key: "egress", Value: v.Network.Egress},
		}},
	}

	spec := bson.D{{Key: "flavor", Value: v.Kind}}
	if len(v.Image) > 0 {
		spec = append(spec, bson.E{Key: "image", Value: v.Image})
	}

	spec = append(spec, config...)

	if v.PersistentDisk {
		spec = append(spec, bson.E{Key: "persistent_disk", Value: true})
	}

	// one never made is made from the snapshot it was to be made from. One
	// whose restore was cut short is restored again by whoever asked for it.
	neverMade := v.CurrentState == 1 || v.CurrentState == 2
	if neverMade && len(v.RestoreFrom) > 0 {
		spec = append(spec, bson.E{Key: "source", Value: bson.D{{Key: "snapshot", Value: v.RestoreFrom}}})
	}

	status := bson.D{
		{Key: "state", Value: state},
		{Key: "expected", Value: expected},
	}

	if len(reason) > 0 {
		status = append(status, bson.E{Key: "reason", Value: reason})
	}

	if !v.UpdatedAt.IsZero() {
		status = append(status, bson.E{Key: "since", Value: moment(v.UpdatedAt)})
	}

	if !v.LastHeartbeatAt.IsZero() {
		status = append(status, bson.E{Key: "observed_at", Value: moment(v.LastHeartbeatAt)})
	}

	var stats any
	if state == "running" && !v.Stats.SampledAt.IsZero() {
		stats = bson.D{
			{Key: "cpu_percent", Value: min(max(v.Stats.CPUPercent, 0), 100)},
			{Key: "memory_used", Value: v.Stats.MemoryUsed},
			{Key: "memory_limit", Value: v.Stats.MemoryLimit},
			{Key: "disk_used", Value: v.Stats.DiskUsed},
			{Key: "disk_total", Value: v.Stats.DiskTotal},
			{Key: "network_rx", Value: v.Stats.NetworkRx},
			{Key: "network_tx", Value: v.Stats.NetworkTx},
			{Key: "sampled_at", Value: moment(v.Stats.SampledAt)},
		}
	}

	status = append(status,
		bson.E{Key: "stats", Value: stats},
		bson.E{Key: "endpoints", Value: nil},
	)

	if !v.StartedAt.IsZero() {
		status = append(status, bson.E{Key: "started_at", Value: moment(v.StartedAt)})
	}

	switch {
	case neverMade:
		// nothing was applied to what was never made: it is made with its
		// spec.
	case v.CurrentState == 7:
		// given its config again: it may have been in the middle of it.
		status = append(status, bson.E{Key: "applied", Value: bson.D{
			{Key: "resources", Value: bson.D{{Key: "cpus", Value: 0}, {Key: "memory", Value: 0}, {Key: "disk", Value: 0}}},
			{Key: "ports", Value: bson.A{}},
			{Key: "network", Value: bson.D{{Key: "ingress", Value: ""}, {Key: "egress", Value: ""}}},
		}})
	default:
		status = append(status, bson.E{Key: "applied", Value: config})
	}

	return bson.D{
		{Key: "_id", Value: v.UUID},
		{Key: "kind", Value: "vm"},
		{Key: "metadata", Value: metadata},
		{Key: "spec", Value: spec},
		{Key: "status", Value: status},
		{Key: "version", Value: int64(1)},
		{Key: "control", Value: bson.D{}},
	}
}

// vmStates are what an old VM's state and expected state are in the vm kind's
// words, and why it is in it. A command it was in the middle of is said as
// what it left behind and what it was asked to be, so that it is asked again.
func vmStates(state int, expected int, reason string) (string, string, string) {
	wanted := map[int]string{4: "running", 6: "stopped", 9: "failed", 10: "deleted"}[expected]
	if len(wanted) == 0 {
		wanted = "running"
	}

	switch state {
	case 1, 2: // created, scheduled
		return "created", wanted, ""
	case 3: // starting
		return "stopped", "running", ""
	case 4: // running
		return "running", wanted, ""
	case 5: // stopping
		return "running", "stopped", ""
	case 6: // stopped
		return "stopped", wanted, ""
	case 7: // restarting
		return "running", wanted, ""
	case 8: // restoring
		return "failed", wanted, "the restore was cut short"
	case 10: // deleting
		return "failed", "deleted", reason
	default: // failed, and anything else
		if len(reason) == 0 {
			reason = "the vm failed"
		}

		return "failed", wanted, reason
	}
}

// moment is a time as a manifest's status keeps it: its JSON, to the
// nanosecond, in UTC.
func moment(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
