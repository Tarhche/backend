package resourcetest

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/kind"
	blockKinds "github.com/khanzadimahdi/testproject/domain/workload/kinds/blocks"
	containerKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/container"
	imageKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/image"
	networkKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/network"
	snapshotKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/snapshot"
	stackKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/stack"
	taskKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/task"
	vmKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/vm"
	volumeKind "github.com/khanzadimahdi/testproject/domain/workload/kinds/volume"
	"github.com/khanzadimahdi/testproject/domain/workload/network"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/resource"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

// theKinds are the kinds whose resources asWritten keeps.
var theKinds = []string{vmKind.Name, taskKind.Name, stackKind.Name, containerKind.Name, networkKind.Name, volumeKind.Name, imageKind.Name, snapshotKind.Name}

// labelSets are labels a person, or an image, may give something, which a
// node's report says again: under keys that extended JSON reads as types, or
// that the database holds only beneath a document's own fields.
var labelSets = map[string]map[string]string{
	"of a key named as extended JSON names a date, of what is no date, first among others": {"$date": "x", "com.example.tier": "web"},
	"of a key named as it names a long, alone":                                             {"$numberLong": "5"},
	"of a key named as it names an object id, alone":                                       {"$oid": "5f0c8a7e9d1b2c3d4e5f6a7b"},
	"of a dot, a dollar alone, and nothing":                                                {"": "none", "$": "dollar", "a.b": "dotted"},
}

// written is a resource as its kind writes it: its record, and the spec and
// the status its record was encoded from, which it is to be read back as.
type written struct {
	record resource.Record
	spec   any
	status any
}

// asWritten holds a repository to keeping every kind's resource as its kind
// wrote it, keys and numbers and all: by every way what somebody gives reaches
// a spec or a status, under whatever keys they give it, and every other
// kind's, whose numbers are big and fractional, and some of whose values are
// null or empty. Each is read back as it was written, and so again once it is
// written back over itself as it was read, as the control plane writes back
// what it reads.
func asWritten(t *testing.T, make Maker) {
	t.Helper()

	at := moment(12)

	ways := map[string]func(t *testing.T, labels map[string]string) []written{
		"a network as its owner asks for it, labelled as they say": func(t *testing.T, labels map[string]string) []written {
			return []written{writtenAs(t, networkKind.Name, kind.Metadata{Name: "front", Labels: labels},
				networkKind.Spec{Name: "front", Driver: "bridge", Labels: labels},
				networkKind.Status{Status: kind.Status{State: networkKind.Pending, Expected: networkKind.Present, Since: at}},
			)}
		},
		"a volume as its owner asks for it, labelled as they say": func(t *testing.T, labels map[string]string) []written {
			return []written{writtenAs(t, volumeKind.Name, kind.Metadata{Name: "data", Labels: labels},
				volumeKind.Spec{Name: "data", Driver: "local", Labels: labels},
				volumeKind.Status{Status: kind.Status{State: volumeKind.Pending, Expected: volumeKind.Present, Since: at}},
			)}
		},
		"a container as its owner asks for it, labelled and given its environment as they say": func(t *testing.T, labels map[string]string) []written {
			return []written{writtenAs(t, containerKind.Name, kind.Metadata{Name: "web", Labels: labels},
				containerKind.Spec{
					VM:        stackKind.VMChoice{UUID: "vm-uuid"},
					Name:      "web",
					Image:     "nginx:1.27",
					Env:       environment(labels),
					Networks:  []string{"shop_front.net"},
					Aliases:   map[string][]string{"shop_front.net": {"web", "www"}},
					CPUs:      0.5,
					Memory:    1<<62 + 1,
					PidsLimit: math.MaxInt64,
				},
				containerKind.Status{Status: kind.Status{State: containerKind.Pending, Expected: containerKind.Running, Since: at}},
			)}
		},
		"a stack whose file gives a service its environment as they say, and the container it makes of it": func(t *testing.T, labels map[string]string) []written {
			var compose strings.Builder
			compose.WriteString("services:\n  web:\n    image: nginx:1.27\n    environment:\n")

			for _, key := range slices.Sorted(maps.Keys(labels)) {
				fmt.Fprintf(&compose, "      %q: %q\n", key, labels[key])
			}

			return []written{
				writtenAs(t, stackKind.Name, kind.Metadata{Name: "shop"},
					stackKind.Spec{VM: stackKind.VMChoice{UUID: "vm-uuid"}, Compose: compose.String()},
					stackKind.Status{Status: kind.Status{State: stackKind.Deploying, Expected: stackKind.Running, Since: at}, Services: []stackKind.ServiceStatus{{Name: "web", State: "starting"}}},
				),
				writtenAs(t, containerKind.Name, kind.Metadata{Name: "shop-abcde_web", Labels: map[string]string{stackKind.LabelStack: "stack-uuid", stackKind.LabelService: "web"}},
					containerKind.Spec{VM: stackKind.VMChoice{UUID: "vm-uuid"}, Name: "shop-abcde_web", Image: "nginx:1.27", Env: environment(labels), Networks: []string{"shop-abcde_default"}, Aliases: map[string][]string{"shop-abcde_default": {"web"}}},
					containerKind.Status{Status: kind.Status{State: containerKind.Pending, Expected: containerKind.Running, Since: at}},
				),
			}
		},
		"what a vm's dockerd says of a container, the labels its image brings among them": func(t *testing.T, labels map[string]string) []written {
			return []written{writtenAs(t, containerKind.Name, kind.Metadata{Name: "web"},
				containerKind.Spec{VM: stackKind.VMChoice{UUID: "vm-uuid"}, Name: "web", Image: "acme/web:1"},
				containerKind.Status{
					Status: kind.Status{State: containerKind.Running, Expected: containerKind.Running, Since: at, ObservedAt: at},
					Docker: &containerKind.Docker{ID: "c0ffee", Name: "web", Image: "acme/web:1", State: "running", Status: "Up 2 minutes", Ports: []containerKind.PortBinding{}, Networks: []string{"bridge"}, Mounts: []containerKind.Mount{}, Labels: platforms(containerKind.Name, labels), CreatedAt: at},
				},
			)}
		},
		"and of a network, and of a volume": func(t *testing.T, labels map[string]string) []written {
			return []written{
				writtenAs(t, networkKind.Name, kind.Metadata{Name: "front"},
					networkKind.Spec{Name: "front", Labels: labels},
					networkKind.Status{Status: kind.Status{State: networkKind.Present, Expected: networkKind.Present, Since: at, ObservedAt: at}, Docker: &networkKind.Docker{ID: "n1", Name: "front", Driver: "bridge", Scope: "local", Containers: []string{}, Labels: platforms(networkKind.Name, labels), CreatedAt: at}},
				),
				writtenAs(t, volumeKind.Name, kind.Metadata{Name: "data"},
					volumeKind.Spec{Name: "data", Labels: labels},
					volumeKind.Status{Status: kind.Status{State: volumeKind.Present, Expected: volumeKind.Present, Since: at, ObservedAt: at}, Docker: &volumeKind.Docker{Name: "data", Driver: "local", Mountpoint: "/var/lib/docker/volumes/data/_data", Labels: platforms(volumeKind.Name, labels), CreatedAt: at}},
				),
			}
		},
		"what is taken in from a restored vm's disk, as its vm's dockerd says it": func(t *testing.T, labels map[string]string) []written {
			spec := containerKind.Spec{Name: "web", Image: "acme/web:1", Env: environment(labels)}

			label, err := containerKind.SpecLabel(spec)
			require.NoError(t, err)

			found := platforms(containerKind.Name, labels)
			found[containerKind.LabelSpec] = label

			spec.VM = stackKind.VMChoice{UUID: "vm-uuid"}

			return []written{
				writtenAs(t, containerKind.Name, kind.Metadata{UUID: "from-the-snapshot", Name: "web"}, spec,
					containerKind.Status{Status: kind.Status{State: containerKind.Stopped, Expected: containerKind.Stopped, Since: at, ObservedAt: at}, Docker: &containerKind.Docker{ID: "c0ffee", Name: "web", Image: "acme/web:1", State: "exited", Status: "Exited (0) 1 minute ago", Ports: []containerKind.PortBinding{}, Networks: []string{}, Mounts: []containerKind.Mount{}, Labels: found, CreatedAt: at}},
				),
				writtenAs(t, networkKind.Name, kind.Metadata{UUID: "network-from-the-snapshot", Name: "front"},
					networkKind.Spec{Name: "front", Driver: "bridge", Labels: labels},
					networkKind.Status{Status: kind.Status{State: networkKind.Present, Expected: networkKind.Present, Since: at, ObservedAt: at}, Docker: &networkKind.Docker{ID: "n1", Name: "front", Driver: "bridge", Scope: "local", Containers: []string{"c0ffee"}, Labels: platforms(networkKind.Name, labels), CreatedAt: at}},
				),
				writtenAs(t, volumeKind.Name, kind.Metadata{UUID: "volume-from-the-snapshot", Name: "data"},
					volumeKind.Spec{Name: "data", Driver: "local", Labels: labels},
					volumeKind.Status{Status: kind.Status{State: volumeKind.Present, Expected: volumeKind.Present, Since: at, ObservedAt: at}, Docker: &volumeKind.Docker{Name: "data", Driver: "local", Mountpoint: "/var/lib/docker/volumes/data/_data", Labels: platforms(volumeKind.Name, labels), InUse: true, CreatedAt: at}},
				),
			}
		},
	}

	for way, writes := range ways {
		for set, labels := range labelSets {
			t.Run(way+", "+set, func(t *testing.T) {
				t.Parallel()

				repository := make(t, theKinds...)

				for _, w := range writes(t, labels) {
					keptAsWritten(t, repository, w)
				}
			})
		}
	}

	t.Run("and every other kind's, whose numbers are big and fractional, and some of whose values are null or empty", func(t *testing.T) {
		t.Parallel()

		repository := make(t, theKinds...)
		none := 0

		for _, w := range []written{
			writtenAs(t, vmKind.Name, kind.Metadata{Name: "box", Slug: "box-abcde"},
				vmKind.Spec{Image: "docker", Resources: vmKind.Resources{CPUs: 4, Memory: 1<<62 + 1, Disk: math.MaxInt64}, Ports: []port.Port{}, Network: vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}, PersistentDisk: true},
				vmKind.Status{
					Status:    kind.Status{State: vmKind.Running, Expected: vmKind.Running, Since: at, ObservedAt: at},
					Stats:     &vmKind.Stats{CPUPercent: 0.1 + 0.2, MemoryUsed: 1<<53 + 1, MemoryLimit: math.MaxInt64, DiskUsed: 0, DiskTotal: 1 << 40, NetworkRx: 0, NetworkTx: math.MaxInt32 + 1, SampledAt: at},
					StartedAt: at,
					Applied:   &vmKind.Config{Resources: vmKind.Resources{CPUs: 4, Memory: 1<<62 + 1, Disk: math.MaxInt64}, Network: vmKind.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny}},
					Restore:   kind.Restore{RestoredAt: at},
					LookedAt:  at,
				},
			),
			writtenAs(t, taskKind.Name, kind.Metadata{Name: "job", Slug: "job-abcde"},
				taskKind.Spec{Kind: task.KindJob, Image: "python:3.13", Entrypoint: []string{}, Environment: []string{"EMPTY=", "$date=x"}, NetworkPolicy: network.PolicyIsolated, TTL: 90 * time.Second, Limits: taskKind.Limits{CPU: 1e-7, Memory: 512 << 20, Disk: 1 << 40}, MaxRetries: &none},
				taskKind.Status{Status: kind.Status{State: taskKind.Running, Expected: taskKind.Completed, Since: at}, Retries: 2, Run: &taskKind.Run{ID: "run-1", Attempt: 3, Kind: task.KindJob, StartedAt: at, Deadline: at.Add(time.Hour), ExitCode: -1, Endpoints: []taskKind.Endpoint{}}},
			),
			writtenAs(t, imageKind.Name, kind.Metadata{Name: "nginx:1.27"},
				imageKind.Spec{Reference: "nginx:1.27"},
				imageKind.Status{Status: kind.Status{State: imageKind.Present, Expected: imageKind.Present, Since: at}, Docker: &imageKind.Docker{Reference: "nginx:1.27", ID: "sha256:abc", Tags: []string{}, Size: math.MaxInt64, CreatedAt: at}, Failure: &noderequest.Error{}},
			),
			writtenAs(t, snapshotKind.Name, kind.Metadata{Name: "before"},
				snapshotKind.Spec{VM: snapshotKind.VMRef{UUID: "vm-uuid"}},
				snapshotKind.Status{Status: kind.Status{State: snapshotKind.Ready, Expected: snapshotKind.Ready, Since: at}, Image: "docker", Disk: 1 << 62, Engine: "microsandbox", Size: math.MaxInt64, CompletedAt: at},
			),
		} {
			keptAsWritten(t, repository, w)
		}
	})
}

// writtenAs is a resource of a kind, of what metadata says, as its kind
// writes its spec and its status.
func writtenAs(t *testing.T, kindName string, metadata kind.Metadata, spec any, status any) written {
	t.Helper()

	encodedSpec, err := json.Marshal(spec)
	require.NoError(t, err)

	encodedStatus, err := json.Marshal(status)
	require.NoError(t, err)

	metadata.OwnerUUID = "owner-uuid"
	metadata.CreatedAt, metadata.UpdatedAt = moment(12), moment(12)

	return written{
		record: resource.Record{Raw: kind.Raw{Kind: kindName, Metadata: metadata, Spec: encodedSpec, Status: encodedStatus}},
		spec:   spec,
		status: status,
	}
}

// keptAsWritten fails t unless repository keeps what w writes as it is
// written, and keeps it so once it is written back over itself as it was
// read.
func keptAsWritten(t *testing.T, repository resource.Repository, w written) {
	t.Helper()

	ctx := context.Background()

	created, err := repository.Create(ctx, w.record)
	require.NoError(t, err, "a %s is kept", w.record.Kind)

	stored, err := repository.GetOne(ctx, w.record.Kind, created.Metadata.UUID)
	require.NoError(t, err)

	readAsWritten(t, w, stored)

	_, err = repository.Update(ctx, stored)
	require.NoError(t, err, "and written back as it was read")

	again, err := repository.GetOne(ctx, w.record.Kind, created.Metadata.UUID)
	require.NoError(t, err)

	readAsWritten(t, w, again)
}

// readAsWritten fails t unless got is what w wrote: the same JSON, which reads
// as what it was encoded from, number for number.
func readAsWritten(t *testing.T, w written, got resource.Record) {
	t.Helper()

	want := w.record
	want.Metadata.UUID = got.Metadata.UUID
	want.Version = got.Version

	Equal(t, want, got)

	for what, encoded := range map[string]struct {
		from any
		read json.RawMessage
	}{"spec": {w.spec, got.Spec}, "status": {w.status, got.Status}} {
		decoded := reflect.New(reflect.TypeOf(encoded.from))
		if !assert.NoError(t, json.Unmarshal(encoded.read, decoded.Interface()), "a %s's %s is read as its kind's", w.record.Kind, what) {
			continue
		}

		before, err := json.Marshal(encoded.from)
		require.NoError(t, err)

		after, err := json.Marshal(decoded.Elem().Interface())
		require.NoError(t, err)

		assert.Equal(t, string(before), string(after), "a %s's %s is read as it was written", w.record.Kind, what)
	}
}

// environment is labels as an environment, each as KEY=value, in the order of
// their keys.
func environment(labels map[string]string) []string {
	variables := make([]string, 0, len(labels))
	for _, key := range slices.Sorted(maps.Keys(labels)) {
		variables = append(variables, key+"="+labels[key])
	}

	return variables
}

// platforms are labels as a VM's dockerd says them of something of a kind the
// platform made: beside the platform's own.
func platforms(kindName string, labels map[string]string) map[string]string {
	all := blockKinds.Labels(kindName, "its-uuid")
	maps.Copy(all, labels)

	return all
}
