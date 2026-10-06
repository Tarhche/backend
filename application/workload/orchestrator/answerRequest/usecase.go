// Package answerRequest answers what the control plane asks a node and waits
// for that no kind answers: whatever is asked of a Docker VM's dockerd.
package answerRequest

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/docker/containers"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/docker/images"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/docker/networks"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/docker/volumes"
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/internal/reply"
	"github.com/khanzadimahdi/testproject/domain/workload/docker"
	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Daemons are the dockerds of this node's Docker VMs.
type Daemons interface {
	Daemon(vmUUID string) docker.Daemon
}

// Recorder is where how long answering a request to a Docker VM's dockerd
// took is recorded, by operation.
type Recorder interface {
	DockerRequest(ctx context.Context, op string, took time.Duration)
}

// group answers the operations of one kind of docker object.
type group interface {
	Answer(ctx context.Context, request noderequest.Request) (result any, truncated bool, err error)
}

// UseCase answers node requests.
//
// A request to a Docker VM waits for its dockerd first: a VM that was just
// made, started or restarted has a daemon that comes up after it does, and
// asking it before then would fail what is only early. Waiting stops at once
// when it cannot help — the VM is not here, is not running, or has no docker
// at all — and otherwise after the time a dockerd is given to come up, as
// docker_unavailable. Whatever goes wrong is the reply's error, in the codes
// every side knows; nothing is returned, because the node is the only one that
// can say what it was.
type UseCase struct {
	daemons  Daemons
	groups   map[string]group
	recorder Recorder
}

var _ noderequest.Handler = &UseCase{}

func NewUseCase(daemons Daemons, recorder Recorder) *UseCase {
	return &UseCase{
		daemons: daemons,
		groups: map[string]group{
			"docker.containers.": containers.NewUseCase(daemons),
			"docker.images.":     images.NewUseCase(daemons),
			"docker.networks.":   networks.NewUseCase(daemons),
			"docker.volumes.":    volumes.NewUseCase(daemons),
		},
		recorder: recorder,
	}
}

func (uc *UseCase) Handle(ctx context.Context, request noderequest.Request) noderequest.Reply {
	if !request.Op.IsValid() {
		return noderequest.Failed(reply.Invalid("%q is not an operation a node answers", request.Op))
	}

	if err := reply.Required("vm_uuid", request.VMUUID); err != nil {
		return noderequest.Failed(err)
	}

	started := time.Now()
	defer func() {
		uc.recorder.DockerRequest(ctx, string(request.Op), time.Since(started))
	}()

	if err := uc.daemons.Daemon(request.VMUUID).Ping(ctx); err != nil {
		return noderequest.Failed(err)
	}

	if request.Op == noderequest.OpPing {
		return noderequest.Reply{OK: true}
	}

	for prefix, answering := range uc.groups {
		if !strings.HasPrefix(string(request.Op), prefix) {
			continue
		}

		result, truncated, err := answering.Answer(ctx, request)
		if err != nil {
			return noderequest.Failed(err)
		}

		return succeeded(result, truncated)
	}

	return noderequest.Failed(reply.Invalid("%q is not an operation a node answers", request.Op))
}

// succeeded is the reply to a request that was answered, with what it was
// answered with, if anything.
func succeeded(result any, truncated bool) noderequest.Reply {
	if result == nil {
		return noderequest.Reply{OK: true}
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return noderequest.Failed(err)
	}

	return noderequest.Reply{OK: true, Result: encoded, Truncated: truncated}
}
