package reconcile

import (
	"context"
	"errors"
	"time"

	"github.com/khanzadimahdi/testproject/application/workload/controlplane/task/placement"
	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
	"github.com/khanzadimahdi/testproject/domain/workload/task"
)

// outage reports whether a silent task is one its node cannot see right now,
// rather than one that has gone.
//
// A node lists what it holds through the driver of each class it offers, and
// one whose driver is out — vmhost being redeployed, say — cannot list what
// that driver holds, so its tasks go quiet while they are still running. Asking
// for one of those again would run it twice once the class comes back. So a
// silent task is left as it is while its node is speaking and says the class
// running it is out: it is unknown, not gone. That lasts as long as the grace
// does, counted from when the task was last heard from; a class out for longer
// than that is not coming back soon, and its tasks are treated like any other
// silent ones.
func (uc *UseCase) outage(ctx context.Context, t *task.Task, now time.Time, holders *nodes) (bool, error) {
	if uc.outageGrace <= 0 || len(t.NodeName) == 0 || t.Silent(now, uc.outageGrace) {
		return false, nil
	}

	holder, found, err := holders.get(ctx, t.NodeName)
	if err != nil || !found {
		return false, err
	}

	// a node that is not speaking either is not saying anything about its
	// classes: that is the node gone, which the drift rules already answer.
	if !placement.Healthy(holder, now) {
		return false, nil
	}

	offer, offered := placement.OfferOf(holder, t.Runtime.OrSysbox())

	return offered && !offer.Healthy, nil
}

// nodes are the nodes one pass has asked about, each read once however many
// of its tasks have gone quiet.
type nodes struct {
	repository node.Repository
	read       map[string]*node.Node
}

func newNodes(repository node.Repository) *nodes {
	return &nodes{repository: repository, read: make(map[string]*node.Node)}
}

// get is the node by its name, and whether there is one.
func (n *nodes) get(ctx context.Context, name string) (node.Node, bool, error) {
	if known, ok := n.read[name]; ok {
		if known == nil {
			return node.Node{}, false, nil
		}

		return *known, true, nil
	}

	found, err := n.repository.GetOne(ctx, name)
	if errors.Is(err, domain.ErrNotExists) {
		n.read[name] = nil

		return node.Node{}, false, nil
	} else if err != nil {
		return node.Node{}, false, err
	}

	n.read[name] = &found

	return found, true, nil
}
