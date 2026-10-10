package microsandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/port"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
)

func TestStore(t *testing.T) {
	t.Parallel()

	t.Run("a record reads back as it was written", func(t *testing.T) {
		t.Parallel()

		s, err := newStore(filepath.Join(t.TempDir(), "instances"))
		require.NoError(t, err)

		at := time.Date(2026, 10, 5, 8, 0, 0, 0, time.UTC)
		written := &record{
			Spec: vm.Spec{
				ID:             "0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b",
				Image:          "docker:29-dind",
				Resources:      vm.Resources{CPUs: 2, Memory: 2 << 30, Disk: 20 << 30},
				Ports:          []port.Port{80, 443},
				Network:        vm.Network{Ingress: vm.AccessAllow, Egress: vm.AccessDeny},
				PersistentDisk: true,
				Labels:         map[string]string{vm.LabelOwner: "owner"},
				Env:            []string{"A=1"},
			},
			Image:       "docker:29-dind",
			HostPorts:   map[port.Port]port.Port{80: 20000, 443: 20001},
			Restored:    true,
			MainRunning: true,
			Exit:        &exit{Code: -1, Reason: "lost", At: at},
			StartedAt:   at,
		}

		require.NoError(t, s.save(written.Spec.ID, written))

		records, err := s.load()
		require.NoError(t, err)
		require.Len(t, records, 1)
		assert.Equal(t, written, records[written.Spec.ID])
	})

	t.Run("a record written again replaces the one there was, and leaves nothing beside it", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "instances")
		s, err := newStore(dir)
		require.NoError(t, err)

		require.NoError(t, s.save("vm-1", &record{Image: "first"}))
		require.NoError(t, s.save("vm-1", &record{Image: "second"}))

		records, err := s.load()
		require.NoError(t, err)
		assert.Equal(t, "second", records["vm-1"].Image)
		assert.Equal(t, "vm-1", records["vm-1"].Spec.ID, "a record is named by its file")

		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Len(t, entries, 1)
	})

	t.Run("a removed record is gone, and removing it again is no error", func(t *testing.T) {
		t.Parallel()

		s, err := newStore(filepath.Join(t.TempDir(), "instances"))
		require.NoError(t, err)

		require.NoError(t, s.save("vm-1", &record{}))
		require.NoError(t, s.remove("vm-1"))
		require.NoError(t, s.remove("vm-1"))

		records, err := s.load()
		require.NoError(t, err)
		assert.Empty(t, records)
	})

	t.Run("what is not a record is passed over", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "instances")
		s, err := newStore(dir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(dir, ".vm-1.123.tmp"), []byte("half"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600))

		records, err := s.load()
		require.NoError(t, err)
		assert.Empty(t, records)
	})

	t.Run("a record kept while the orchestrator said what an instance boots into reads as its image says", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "instances")
		s, err := newStore(dir)
		require.NoError(t, err)

		kept := map[string]string{
			"vm-1": `{"spec": {"ID": "vm-1", "Kind": "docker", "Image": "docker:29-dind", "Labels": {"workload.docker": "true"}}, "image": "docker:29-dind"}`,
			"vm-2": `{"spec": {"ID": "vm-2", "Kind": "machine", "Image": "ubuntu:24.04"}, "image": "ubuntu:24.04"}`,
		}

		for id, content := range kept {
			require.NoError(t, os.WriteFile(filepath.Join(dir, id+".json"), []byte(content), 0o600))
		}

		records, err := s.load()
		require.NoError(t, err)
		require.Len(t, records, 2)

		assert.Equal(t, vm.KindDocker, records["vm-1"].kind("docker:30-dind"), "booting the vmhost's own image, an older tag of it")
		assert.Equal(t, vm.KindMachine, records["vm-2"].kind("docker:30-dind"))
	})

	t.Run("a record that cannot be read says whose it is", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "instances")
		s, err := newStore(dir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(dir, "vm-1.json"), []byte("{"), 0o600))

		_, err = s.load()
		assert.ErrorContains(t, err, `"vm-1"`)
	})
}

func TestRecordKind(t *testing.T) {
	t.Parallel()

	const dockerImage = "docker:29-dind"

	assert.Equal(t, vm.KindDocker, (&record{Image: dockerImage}).kind(dockerImage), "the vmhost's own image")
	assert.Equal(t, vm.KindDocker, (&record{Image: "docker:28-dind", Spec: vm.Spec{Image: "docker:28-dind"}}).kind(dockerImage), "another tag of it")
	assert.Equal(t, vm.KindMachine, (&record{Image: "ubuntu:24.04", Spec: vm.Spec{Image: "ubuntu:24.04"}}).kind(dockerImage))
	assert.Equal(t, vm.KindMachine, (&record{Image: "golang:1.27", Spec: vm.Spec{Image: "golang:1.27", Command: []string{"go", "run", "."}}}).kind(dockerImage), "a code runner's run")
}

func TestRecordClone(t *testing.T) {
	t.Parallel()

	r := &record{
		Spec:      vm.Spec{Ports: []port.Port{80}, Labels: map[string]string{"a": "b"}, Env: []string{"A=1"}},
		HostPorts: map[port.Port]port.Port{80: 20000},
		Exit:      &exit{Code: 3},
	}

	c := r.clone()
	c.Spec.Ports[0] = 81
	c.Spec.Labels["a"] = "c"
	c.Spec.Env[0] = "A=2"
	c.HostPorts[80] = 20001
	c.Exit.Code = 4

	assert.Equal(t, port.Port(80), r.Spec.Ports[0])
	assert.Equal(t, "b", r.Spec.Labels["a"])
	assert.Equal(t, "A=1", r.Spec.Env[0])
	assert.Equal(t, port.Port(20000), r.HostPorts[80])
	assert.Equal(t, 3, r.Exit.Code)
}

func TestValidID(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"0199a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b", "vm-1", "a", "run_1.2", strings.Repeat("a", 128)} {
		assert.NoError(t, validID(id), id)
	}

	for _, id := range []string{"", "../etc/passwd", "a/b", "-a", ".a", "a b", "ä", strings.Repeat("a", 129)} {
		assert.Error(t, validID(id), id)
	}
}
