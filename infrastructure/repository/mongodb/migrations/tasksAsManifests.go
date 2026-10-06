package migrations

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// tasksAsManifests converts the tasks kept before a task was a kind, which
// are the code runner's runs in flight, into the manifests the task kind is
// kept as, in the same collection:
//
//	{_id, kind: "task", metadata: {name, slug, owner_uuid, node, created_at,
//	 updated_at}, spec: {kind, image, entrypoint, command, environment, ports,
//	 network_policy, interactive, ttl, limits: {cpu, memory, disk},
//	 max_retries}, status: {state, expected, reason, since, observed_at,
//	 retries, run: {id, name, slug, kind, interactive, started_at, deadline,
//	 output}}, version: 1, control: {}}
//
// A task keeps its uuid, its name, which is the request a snippet's answers
// go to, its slug and its node, so the run its node holds is still its, and
// still answered. Its ports are the ones it exposed and was bound to, its
// ttl is in nanoseconds, as every duration of a manifest is, and a policy it
// never said is the public one it ran under before there were policies. What
// it was doing is said in the kind's words, and what an old command was in the
// middle of, which nothing waits on any more, is said so that the kind's own
// reconcile asks for it again: one being placed or run is to be run, which
// its node does by taking the run of it there or making one; one being
// stopped runs and is expected stopped; one restarting runs. A job that has
// ended is deleted by the kind, as one always was. What it printed is its
// run's output, the end of it.
//
// Its old indexes go first: one on a field a manifest does not have would
// hold every converted task, and every new one, to it. The indexes the kind is
// read by are made last, as the control plane makes them for every kind it
// registers.
//
// A task already a manifest is left as it is, so this runs again as well as
// it ran the first time. The names of collections, fields and states are
// written out rather than taken from the repositories and the kind: this is
// what they were when it was written, and it has to keep meaning that however
// they change later.
var tasksAsManifests = Migration{
	Name: "2026-10-06-tasks-as-manifests",
	Up: func(ctx context.Context, database *mongo.Database) error {
		tasks := database.Collection("tasks")

		if err := dropOldIndexes(ctx, tasks); err != nil {
			return err
		}

		cursor, err := tasks.Find(ctx, bson.D{{Key: "metadata", Value: bson.D{{Key: "$exists", Value: false}}}})
		if err != nil {
			return err
		}

		var old []oldTask
		if err := cursor.All(ctx, &old); err != nil {
			return err
		}

		for _, t := range old {
			if _, err := tasks.ReplaceOne(ctx, bson.D{{Key: "_id", Value: t.UUID}}, taskManifest(t)); err != nil {
				return err
			}
		}

		_, err = tasks.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "metadata.slug", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)},
			{Keys: bson.D{{Key: "metadata.owner_uuid", Value: 1}, {Key: "_id", Value: -1}}},
			{Keys: bson.D{{Key: "metadata.node", Value: 1}}},
			{Keys: bson.D{{Key: "metadata.owners.uuid", Value: 1}, {Key: "metadata.owners.kind", Value: 1}}},
		})

		return err
	},
}

// maxTaskOutput is the most of what a task printed its run keeps, in bytes,
// counted from the end: what a task kind's run carries.
const maxTaskOutput = 256 << 10

// dropOldIndexes drops every index of a collection that is not on _id or on
// a manifest's fields.
func dropOldIndexes(ctx context.Context, collection *mongo.Collection) error {
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return err
	}

	var indexes []struct {
		Name string `bson:"name"`
		Key  bson.D `bson:"key"`
	}
	if err := cursor.All(ctx, &indexes); err != nil {
		return err
	}

	for _, index := range indexes {
		manifests := len(index.Key) > 0 && slices.IndexFunc(index.Key, func(key bson.E) bool {
			return key.Key != "_id" && !strings.HasPrefix(key.Key, "metadata.")
		}) < 0

		if index.Name == "_id_" || manifests {
			continue
		}

		if err := collection.Indexes().DropOne(ctx, index.Name); err != nil && !indexNotFound(err) {
			return err
		}
	}

	return nil
}

// oldTask is a task as it was kept before a task was a kind.
type oldTask struct {
	UUID          string `bson:"_id"`
	Name          string `bson:"name"`
	Slug          string `bson:"slug"`
	Kind          string `bson:"kind"`
	CurrentState  int    `bson:"current_state"`
	ExpectedState int    `bson:"expected_state"`

	LastHeartbeatAt time.Time `bson:"last_heartbeat_at"`

	Image         string                     `bson:"image"`
	PortBindings  []map[string]bson.RawValue `bson:"port_bindings"`
	ExposedPorts  []int64                    `bson:"exposed_ports"`
	NetworkPolicy string                     `bson:"network_policy"`
	Environment   []string                   `bson:"environment"`
	Command       []string                   `bson:"command"`
	Entrypoint    []string                   `bson:"entrypoint"`
	Interactive   bool                       `bson:"interactive"`
	MaxRetries    *int64                     `bson:"max_retries"`
	Retries       int64                      `bson:"retries"`
	TTL           int64                      `bson:"ttl"`
	Deadline      time.Time                  `bson:"deadline"`
	Reason        string                     `bson:"reason"`

	ResourceLimits oldLimits `bson:"resource_limits"`

	NodeName      string    `bson:"node_name"`
	ExecutionLogs []byte    `bson:"container_logs"`
	ExecutionID   string    `bson:"container_id"`
	OwnerUUID     string    `bson:"owner_uuid"`
	CreatedAt     time.Time `bson:"created_at"`
	StartedAt     time.Time `bson:"started_at"`
	FinishedAt    time.Time `bson:"finished_at"`
}

type oldLimits struct {
	CPU    float64 `bson:"cpu"`
	Memory int64   `bson:"memory"`
	Disk   int64   `bson:"disk"`
}

// taskManifest is an old task as the task kind keeps it.
func taskManifest(t oldTask) bson.D {
	taskKind := t.Kind
	if taskKind != "job" && taskKind != "service" {
		taskKind = "job"
	}

	state, expected := taskStates(t.CurrentState, t.ExpectedState)
	updated := latestOf(t.CreatedAt, t.StartedAt, t.FinishedAt)

	metadata := bson.D{{Key: "name", Value: t.Name}}

	if len(t.Slug) > 0 {
		metadata = append(metadata, bson.E{Key: "slug", Value: t.Slug})
	}

	metadata = append(metadata, bson.E{Key: "owner_uuid", Value: t.OwnerUUID})

	if len(t.NodeName) > 0 {
		metadata = append(metadata, bson.E{Key: "node", Value: t.NodeName})
	}

	metadata = append(metadata,
		bson.E{Key: "created_at", Value: t.CreatedAt},
		bson.E{Key: "updated_at", Value: updated},
	)

	retries := map[string]int64{"job": 0, "service": 3}[taskKind]
	if t.MaxRetries != nil {
		retries = *t.MaxRetries
	}

	spec := bson.D{
		{Key: "kind", Value: taskKind},
		{Key: "image", Value: t.Image},
	}

	for _, list := range []struct {
		key    string
		values []string
	}{
		{key: "entrypoint", values: t.Entrypoint},
		{key: "command", values: t.Command},
		{key: "environment", values: t.Environment},
	} {
		if len(list.values) > 0 {
			spec = append(spec, bson.E{Key: list.key, Value: list.values})
		}
	}

	if ports := taskPorts(t); len(ports) > 0 {
		spec = append(spec, bson.E{Key: "ports", Value: ports})
	}

	spec = append(spec, bson.E{Key: "network_policy", Value: taskPolicy(t.NetworkPolicy)})

	if t.Interactive {
		spec = append(spec, bson.E{Key: "interactive", Value: true})
	}

	if t.TTL > 0 {
		spec = append(spec, bson.E{Key: "ttl", Value: t.TTL})
	}

	spec = append(spec,
		bson.E{Key: "limits", Value: bson.D{
			{Key: "cpu", Value: t.ResourceLimits.CPU},
			{Key: "memory", Value: t.ResourceLimits.Memory},
			{Key: "disk", Value: t.ResourceLimits.Disk},
		}},
		bson.E{Key: "max_retries", Value: retries},
	)

	status := bson.D{
		{Key: "state", Value: state},
		{Key: "expected", Value: expected},
	}

	if state == "failed" {
		reason := t.Reason
		if len(reason) == 0 {
			reason = "the task failed"
		}

		status = append(status, bson.E{Key: "reason", Value: reason})
	}

	if !updated.IsZero() {
		status = append(status, bson.E{Key: "since", Value: moment(updated)})
	}

	if !t.LastHeartbeatAt.IsZero() {
		status = append(status, bson.E{Key: "observed_at", Value: moment(t.LastHeartbeatAt)})
	}

	if t.Retries > 0 {
		status = append(status, bson.E{Key: "retries", Value: t.Retries})
	}

	if run := taskRun(t, taskKind); len(run) > 0 {
		status = append(status, bson.E{Key: "run", Value: run})
	}

	return bson.D{
		{Key: "_id", Value: t.UUID},
		{Key: "kind", Value: "task"},
		{Key: "metadata", Value: metadata},
		{Key: "spec", Value: spec},
		{Key: "status", Value: status},
		{Key: "version", Value: int64(1)},
		{Key: "control", Value: bson.D{}},
	}
}

// taskRun is the run of an old task its node was last known to hold, when it
// was known to hold one: what it was made as, when it started, and what it
// printed.
func taskRun(t oldTask, taskKind string) bson.D {
	if len(t.ExecutionID) == 0 && t.StartedAt.IsZero() && len(t.ExecutionLogs) == 0 {
		return nil
	}

	run := bson.D{}

	if len(t.ExecutionID) > 0 {
		run = append(run, bson.E{Key: "id", Value: t.ExecutionID})
	}

	run = append(run, bson.E{Key: "name", Value: t.Name})

	if len(t.Slug) > 0 {
		run = append(run, bson.E{Key: "slug", Value: t.Slug})
	}

	run = append(run, bson.E{Key: "kind", Value: taskKind})

	if t.Interactive {
		run = append(run, bson.E{Key: "interactive", Value: true})
	}

	if !t.StartedAt.IsZero() {
		run = append(run, bson.E{Key: "started_at", Value: moment(t.StartedAt)})
	}

	if !t.Deadline.IsZero() {
		run = append(run, bson.E{Key: "deadline", Value: moment(t.Deadline)})
	}

	if output := lastOf(string(t.ExecutionLogs), maxTaskOutput); len(output) > 0 {
		run = append(run, bson.E{Key: "output", Value: output})
	}

	return run
}

// taskStates are what an old task's state and expected state are in the task
// kind's words. A command it was in the middle of is said as what it left
// behind and what it was asked to be, so that it is asked again.
func taskStates(state int, expected int) (string, string) {
	wanted := map[int]string{3: "running", 5: "stopped", 6: "completed", 7: "failed"}[expected]
	if len(wanted) == 0 {
		wanted = "running"
	}

	switch state {
	case 1, 2: // created, scheduled: to be run, by taking the run there or making one
		return "created", wanted
	case 3, 8: // running, restarting
		return "running", wanted
	case 4: // stopping
		return "running", "stopped"
	case 5: // stopped
		return "stopped", wanted
	case 6: // completed
		return "completed", wanted
	default: // failed, and anything else
		return "failed", wanted
	}
}

// taskPorts are the ports an old task exposed or was bound to, lowest first,
// each once.
func taskPorts(t oldTask) bson.A {
	ports := slices.Clone(t.ExposedPorts)

	for _, bindings := range t.PortBindings {
		for key := range bindings {
			if p, err := strconv.ParseInt(key, 10, 64); err == nil {
				ports = append(ports, p)
			}
		}
	}

	slices.Sort(ports)
	ports = slices.Compact(ports)

	listed := make(bson.A, 0, len(ports))
	for _, p := range ports {
		if p > 0 {
			listed = append(listed, p)
		}
	}

	return listed
}

// taskPolicy is an old task's network policy: one stored before there were
// policies ran on the default bridge, which is the public one.
func taskPolicy(stored string) string {
	switch stored {
	case "none", "isolated", "public":
		return stored
	}

	return "public"
}

// latestOf is the last of some moments.
func latestOf(moments ...time.Time) time.Time {
	var last time.Time

	for _, moment := range moments {
		if moment.After(last) {
			last = moment
		}
	}

	return last
}

// lastOf is the last limit bytes of s, starting at a whole character.
func lastOf(s string, limit int) string {
	if len(s) <= limit {
		return s
	}

	s = s[len(s)-limit:]

	for i := 0; i < len(s) && i < utf8.UTFMax; i++ {
		if utf8.RuneStart(s[i]) {
			return s[i:]
		}
	}

	return s
}
