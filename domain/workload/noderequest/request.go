// Package noderequest is the synchronous half of how the control plane talks
// to a node: a question asked over core NATS request/reply and answered at
// once.
//
// Commands are messages, because what they start takes a while and is
// reported as it happens. Docker's objects are not records anybody keeps, so
// asking about one is a request with an answer, and so is a kind's query, a
// VM's log say (kind.Query): nothing is stored to read it from.
//
// A request names a VM, or a kind's resource, and an operation, and carries
// that operation's own payload; the reply carries its result or why there is
// none. The payloads and results of the Docker passthrough are the types in
// payloads.go, which is all the blog, the control plane and the nodes have to
// agree on: the control plane passes the payload through as it came.
package noderequest

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
)

// SubjectPrefix is what every node's requests are asked on, followed by the
// node's name.
const SubjectPrefix = "workloadNodeRequest"

// Subject is what the named node answers requests on.
func Subject(nodeName string) string {
	return SubjectPrefix + "." + nodeName
}

// What a reply may hold. NATS refuses a message over its max_payload, which is
// 1 MiB unless it is configured otherwise, so a reply is kept under it and says
// so when it had to leave something out.
const (
	MaxReplyBytes = 1 << 20

	// MaxLogLines is the most lines of a log a reply carries: the last ones.
	MaxLogLines = 1000
)

// Op is what a request asks for.
type Op string

const (
	// OpPing asks whether a Docker VM's dockerd answers: no payload, no
	// result. It is what docker.Daemon's Ping is through the control plane, so
	// that every method of the Daemon is one operation here.
	OpPing Op = "docker.ping"

	// OpContainersList lists a Docker VM's containers: ContainersRequest in,
	// []Container out.
	OpContainersList Op = "docker.containers.list"

	// OpContainersInspect is one container: ContainerRequest in, Container out.
	OpContainersInspect Op = "docker.containers.inspect"

	// OpContainersCreate creates a container and starts it, pulling its image
	// first when the VM does not hold it: ContainerSpec in, Container out.
	OpContainersCreate Op = "docker.containers.create"

	// OpContainersStart, OpContainersStop and OpContainersRestart take a
	// ContainerRequest and answer with nothing.
	OpContainersStart   Op = "docker.containers.start"
	OpContainersStop    Op = "docker.containers.stop"
	OpContainersRestart Op = "docker.containers.restart"

	// OpContainersRemove takes a RemoveRequest and answers with nothing.
	OpContainersRemove Op = "docker.containers.remove"

	// OpContainersLogs reads a container's log: ContainerLogsRequest in,
	// []LogLine out.
	OpContainersLogs Op = "docker.containers.logs"

	// OpContainersStats samples what a container uses: ContainerRequest in,
	// Stats out.
	OpContainersStats Op = "docker.containers.stats"

	// OpContainersConnect takes a ConnectRequest, and OpContainersDisconnect a
	// DisconnectRequest; both answer with nothing.
	OpContainersConnect    Op = "docker.containers.connect"
	OpContainersDisconnect Op = "docker.containers.disconnect"

	// OpImagesList lists a Docker VM's images: no payload, []Image out.
	OpImagesList Op = "docker.images.list"

	// OpImagesPull pulls an image: PullRequest in, Image out.
	OpImagesPull Op = "docker.images.pull"

	// OpImagesRemove takes a RemoveRequest and answers with nothing.
	OpImagesRemove Op = "docker.images.remove"

	// OpNetworksList lists a Docker VM's networks: no payload, []Network out.
	OpNetworksList Op = "docker.networks.list"

	// OpNetworksCreate creates a network: NetworkSpec in, Network out.
	OpNetworksCreate Op = "docker.networks.create"

	// OpNetworksRemove takes a RemoveRequest, whose Force means nothing to a
	// network, and answers with nothing.
	OpNetworksRemove Op = "docker.networks.remove"

	// OpVolumesList lists a Docker VM's volumes: no payload, []Volume out.
	OpVolumesList Op = "docker.volumes.list"

	// OpVolumesCreate creates a volume: VolumeSpec in, Volume out.
	OpVolumesCreate Op = "docker.volumes.create"

	// OpVolumesRemove takes a RemoveRequest naming the volume as its ID, and
	// answers with nothing.
	OpVolumesRemove Op = "docker.volumes.remove"
)

// ops is every operation a node answers.
var ops = []Op{
	OpPing,
	OpContainersList,
	OpContainersInspect,
	OpContainersCreate,
	OpContainersStart,
	OpContainersStop,
	OpContainersRestart,
	OpContainersRemove,
	OpContainersLogs,
	OpContainersStats,
	OpContainersConnect,
	OpContainersDisconnect,
	OpImagesList,
	OpImagesPull,
	OpImagesRemove,
	OpNetworksList,
	OpNetworksCreate,
	OpNetworksRemove,
	OpVolumesList,
	OpVolumesCreate,
	OpVolumesRemove,
}

// IsValid reports whether o is an operation a node answers.
func (o Op) IsValid() bool {
	return slices.Contains(ops, o)
}

// IsDocker reports whether o is asked of a Docker VM's dockerd, which only a
// Docker VM has, and which is waited for while the VM is still coming up.
func (o Op) IsDocker() bool {
	return o.IsValid() && strings.HasPrefix(string(o), "docker.")
}

// MayPull reports whether o may have to pull an image before it can answer,
// which is what makes it slow: it is given the pull timeout rather than the
// one every other request has.
func (o Op) MayPull() bool {
	return o == OpContainersCreate || o == OpImagesPull
}

// Request is one question for a node.
type Request struct {
	Op Op `json:"op"`

	// VMUUID is the VM the question is about.
	VMUUID string `json:"vm_uuid"`

	// Payload is the operation's own request, as it was given.
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Reply is a node's answer.
type Reply struct {
	OK bool `json:"ok"`

	// Result is the operation's own answer, when it has one.
	Result json.RawMessage `json:"result,omitempty"`

	// Error is why there is no answer, when OK is false.
	Error *Error `json:"error,omitempty"`

	// Truncated says the result was cut to fit in a reply: a log keeps its
	// last lines, a listing its first items.
	Truncated bool `json:"truncated,omitempty"`
}

// errNoReason is a reply that says it failed and not why, which only a node
// that is broken in some other way sends.
var errNoReason = errors.New("the node did not say why the request failed")

// Err is why the request failed, or nil when it did not.
func (r Reply) Err() error {
	if r.OK {
		return nil
	}

	if r.Error == nil {
		return &Error{Code: CodeInternal, Message: errNoReason.Error()}
	}

	return r.Error
}

// Requester asks nodes questions.
type Requester interface {
	// Request asks the named node and waits for its answer for as long as the
	// context allows. An error is a question nobody answered: no node of
	// that name listening, or the time running out. A node that answered and
	// refused does so in the Reply.
	Request(ctx context.Context, nodeName string, request Request) (Reply, error)
}

// Handler answers questions on a node.
type Handler interface {
	// Handle answers one request. It does not fail: whatever goes wrong is the
	// reply's Error, because the node is the only one that can say what it was.
	Handle(ctx context.Context, request Request) Reply
}
