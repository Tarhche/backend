package mock

import (
	"context"
	"sync"

	"github.com/khanzadimahdi/testproject/domain/workload/noderequest"
)

// Asked is one request a Requester was given, and the node it was asked of.
type Asked struct {
	NodeName string
	Request  noderequest.Request
}

// Requester stands in for the nodes the control plane asks questions of.
// Every request is kept, and answered by Answer; without one, every node
// answers that it did what it was asked, with nothing to say about it.
type Requester struct {
	lock  sync.Mutex
	asked []Asked

	Answer func(ctx context.Context, nodeName string, request noderequest.Request) (noderequest.Reply, error)
}

var _ noderequest.Requester = &Requester{}

func (r *Requester) Request(ctx context.Context, nodeName string, request noderequest.Request) (noderequest.Reply, error) {
	r.lock.Lock()
	r.asked = append(r.asked, Asked{NodeName: nodeName, Request: request})
	answer := r.Answer
	r.lock.Unlock()

	if answer == nil {
		return noderequest.Reply{OK: true}, nil
	}

	return answer(ctx, nodeName, request)
}

// Asked is every request given so far, in order.
func (r *Requester) Asked() []Asked {
	r.lock.Lock()
	defer r.lock.Unlock()

	return append([]Asked(nil), r.asked...)
}
