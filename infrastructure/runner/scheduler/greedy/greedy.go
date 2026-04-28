package greedy

import (
	"cmp"
	"slices"

	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type Greedy struct{}

var _ task.Scheduler = &Greedy{}

func New() *Greedy {
	return &Greedy{}
}

func (g *Greedy) Pick(_ *task.Task, candidates []node.Node) node.Node {
	if len(candidates) == 0 {
		return node.Node{}
	}

	return slices.MinFunc(candidates, func(a, b node.Node) int {
		return cmp.Compare(a.Stats.CPUPercent, b.Stats.CPUPercent)
	})
}
