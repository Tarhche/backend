// Package answerRequest answers what the control plane asks a node and waits
// for: a VM's log, and whatever is asked of a Docker VM's dockerd.
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
	"github.com/khanzadimahdi/testproject/application/workload/orchestrator/vm/getVMLogs"
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
	vmLogs   *getVMLogs.UseCase
	recorder Recorder
}

var _ noderequest.Handler = &UseCase{}

func NewUseCase(daemons Daemons, vmLogs *getVMLogs.UseCase, recorder Recorder) *UseCase {
	return &UseCase{
		daemons: daemons,
		groups: map[string]group{
			"docker.containers.": containers.NewUseCase(daemons),
			"docker.images.":     images.NewUseCase(daemons),
			"docker.networks.":   networks.NewUseCase(daemons),
			"docker.volumes.":    volumes.NewUseCase(daemons),
		},
		vmLogs:   vmLogs,
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

	if request.Op == noderequest.OpVMLogs {
		return uc.logs(ctx, request)
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

// logs reads a VM's log, which needs no dockerd and so waits for none.
func (uc *UseCase) logs(ctx context.Context, request noderequest.Request) noderequest.Reply {
	var asked noderequest.LogsRequest
	if err := reply.Decode(request.Payload, &asked); err != nil {
		return noderequest.Failed(err)
	}

	read, err := uc.vmLogs.Execute(ctx, &getVMLogs.Request{VMUUID: request.VMUUID, Since: asked.Since, Tail: asked.Tail})
	if err != nil {
		return noderequest.Failed(err)
	}

	if len(read.ValidationErrors) > 0 {
		return noderequest.Failed(reply.Invalid("the request was refused: %v", read.ValidationErrors))
	}

	lines := make([]noderequest.VMLogLine, len(read.Lines))
	for n, line := range read.Lines {
		lines[n] = noderequest.NewVMLogLine(line)
	}

	fitted, cut := reply.Last(lines)

	return succeeded(fitted, read.Truncated || cut)
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
