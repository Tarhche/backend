package roundrobin

import (
	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type RoundRobin struct {
	lastNode int
}

var _ task.Scheduler = &RoundRobin{}

func New() *RoundRobin {
	return &RoundRobin{
		lastNode: 0,
	}
}

func (r *RoundRobin) Pick(t *task.Task, candidates []node.Node) node.Node {
	var bestNode node.Node
	var lowestScore float64

	scores := r.score(t, candidates)

	for idx, node := range candidates {
		nodeName := node.Name

		if idx == 0 {
			bestNode = node
			lowestScore = scores[nodeName]
			continue
		}

		if scores[nodeName] < lowestScore {
			bestNode = node
			lowestScore = scores[nodeName]
		}
	}

	return bestNode
}

func (r *RoundRobin) score(t *task.Task, nodes []node.Node) map[string]float64 {
	nodeScores := make(map[string]float64)
	_ = t

	var newNode int
	if r.lastNode+1 < len(nodes) {
		newNode = r.lastNode + 1
		r.lastNode++
	} else {
		newNode = 0
		r.lastNode = 0
	}

	for idx, node := range nodes {
		if idx == newNode {
			nodeScores[node.Name] = 0.1
		} else {
			nodeScores[node.Name] = 1.0
		}
	}

	return nodeScores
}
