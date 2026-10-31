// Package nodes keeps the workload's nodes in memory, the way the MongoDB
// repository keeps them: by name.
package nodes

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/khanzadimahdi/testproject/domain"
	"github.com/khanzadimahdi/testproject/domain/workload/node"
)

type Repository struct {
	lock  sync.Mutex
	nodes map[string]node.Node

	// Fail, when set, is what every call reports instead of doing anything.
	Fail error
}

var _ node.Repository = &Repository{}

func NewRepository(nodes ...node.Node) *Repository {
	r := &Repository{nodes: make(map[string]node.Node, len(nodes))}

	for _, n := range nodes {
		r.nodes[n.Name] = n
	}

	return r
}

func (r *Repository) GetAll(_ context.Context, offset uint, limit uint) ([]node.Node, error) {
	if r.Fail != nil {
		return nil, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	items := make([]node.Node, 0, len(r.nodes))
	for _, n := range r.nodes {
		items = append(items, n)
	}

	slices.SortFunc(items, func(a, b node.Node) int { return strings.Compare(a.Name, b.Name) })

	if offset >= uint(len(items)) {
		return []node.Node{}, nil
	}

	return items[offset:min(offset+limit, uint(len(items)))], nil
}

func (r *Repository) GetOne(_ context.Context, name string) (node.Node, error) {
	if r.Fail != nil {
		return node.Node{}, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	n, ok := r.nodes[name]
	if !ok {
		return node.Node{}, domain.ErrNotExists
	}

	return n, nil
}

func (r *Repository) Save(_ context.Context, n *node.Node) (string, error) {
	if r.Fail != nil {
		return "", r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	r.nodes[n.Name] = *n

	return n.Name, nil
}

func (r *Repository) Count(_ context.Context) (uint, error) {
	if r.Fail != nil {
		return 0, r.Fail
	}

	r.lock.Lock()
	defer r.lock.Unlock()

	return uint(len(r.nodes)), nil
}
