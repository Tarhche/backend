package blockstest

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/vm"
	"github.com/khanzadimahdi/testproject/infrastructure/workload/vm/memory"
)

// DockerImage is the image the node boots Docker VMs from, as a vmhost's
// Docker image is: a VM booting it is a Docker VM, which its engine says.
const DockerImage = "docker:29-dind"

// Node is a node's engine, kept in memory, and the dockerds of its Docker
// VMs.
type Node struct {
	Engine *memory.Engine

	lock     sync.Mutex
	dockerds map[string]*Dockerd
}

// NewNode is a node with no VM on it.
func NewNode() *Node {
	return &Node{Engine: memory.New(memory.WithCapacity(64, 256<<30, 4096<<30), memory.WithDockerImage(DockerImage)), dockerds: make(map[string]*Dockerd)}
}

// DockerVM is a running Docker VM on the node, made as a node makes one, from
// the Docker image, and its dockerd.
func (n *Node) DockerVM(t testing.TB, uuid string) *Dockerd {
	t.Helper()

	_, err := n.Engine.Create(t.Context(), vm.Spec{
		ID:     uuid,
		Image:  DockerImage,
		Labels: map[string]string{vm.LabelPurpose: vm.PurposeVM, vm.LabelVM: uuid},
	})
	require.NoError(t, err)

	d := NewDockerd()

	n.lock.Lock()
	n.dockerds[uuid] = d
	n.lock.Unlock()

	return d
}

// Machine is a running machine VM on the node, which has no dockerd.
func (n *Node) Machine(t testing.TB, uuid string) {
	t.Helper()

	_, err := n.Engine.Create(t.Context(), vm.Spec{
		ID:     uuid,
		Image:  "ubuntu:24.04",
		Labels: map[string]string{vm.LabelPurpose: vm.PurposeVM, vm.LabelVM: uuid},
	})
	require.NoError(t, err)
}

// Stop stops a VM on the node.
func (n *Node) Stop(t testing.TB, uuid string) {
	t.Helper()

	require.NoError(t, n.Engine.Stop(t.Context(), uuid))
}

// Daemon is the dockerd of a Docker VM on the node, and one that never
// answers for any other VM.
func (n *Node) Daemon(vmUUID string) docker.Daemon {
	n.lock.Lock()
	defer n.lock.Unlock()

	if d, ok := n.dockerds[vmUUID]; ok {
		return d
	}

	return &Dockerd{Down: true, calls: make(map[string]int)}
}

// Failing is an engine that cannot say which instances it holds.
type Failing struct {
	vm.Engine
}

func (Failing) List(context.Context) ([]vm.Instance, error) {
	return nil, errors.New("the engine is not answering")
}
