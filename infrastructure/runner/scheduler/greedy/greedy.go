package greedy

import (
	"math"
	"time"

	"github.com/khanzadimahdi/testproject/domain/runner/node"
	"github.com/khanzadimahdi/testproject/domain/runner/task"
)

type Greedy struct{}

var _ task.Scheduler = &Greedy{}

func New() *Greedy {
	return &Greedy{}
}

func (g *Greedy) Pick(t *task.Task, candidates []node.Node) node.Node {
	minCpu := 0.00

	candidates = g.nominate(t, candidates)
	scores := g.score(t, candidates)

	var bestNode node.Node
	for idx, node := range candidates {
		if idx == 0 {
			minCpu = scores[node.Name]
			bestNode = node
			continue
		}

		if scores[node.Name] < minCpu {
			minCpu = scores[node.Name]
			bestNode = node
		}
	}

	return bestNode
}

func (g *Greedy) nominate(t *task.Task, nodes []node.Node) []node.Node {
	var candidates []node.Node
	for node := range nodes {
		if checkDisk(t, nodes[node].Stats.Disk.Available) {
			candidates = append(candidates, nodes[node])
		}
	}

	return candidates
}

func (g *Greedy) score(t *task.Task, nodes []node.Node) map[string]float64 {
	nodeScores := make(map[string]float64)
	_ = t

	for _, node := range nodes {
		cpuUsage := calculateCpuUsage(&node)
		cpuLoad := calculateLoad(float64(cpuUsage), math.Pow(2, 0.8))
		nodeScores[node.Name] = cpuLoad
	}

	return nodeScores
}

func checkDisk(t *task.Task, diskAvailable uint64) bool {
	return t.ResourceLimits.Disk <= diskAvailable
}

func calculateLoad(usage float64, capacity float64) float64 {
	return usage / capacity
}

func calculateCpuUsage(node *node.Node) float64 {
	stat1 := node.Stats
	time.Sleep(3 * time.Second)
	stat2 := node.Stats

	stat1Idle := stat1.CPU.Idle + stat1.CPU.IOWait
	stat2Idle := stat2.CPU.Idle + stat2.CPU.IOWait
	stat1NonIdle := stat1.CPU.User + stat1.CPU.Nice + stat1.CPU.System + stat1.CPU.IRQ + stat1.CPU.SoftIRQ + stat1.CPU.Steal
	stat2NonIdle := stat2.CPU.User + stat2.CPU.Nice + stat2.CPU.System + stat2.CPU.IRQ + stat2.CPU.SoftIRQ + stat2.CPU.Steal
	stat1Total := stat1Idle + stat1NonIdle
	stat2Total := stat2Idle + stat2NonIdle
	total := stat2Total - stat1Total
	idle := stat2Idle - stat1Idle

	var cpuPercentUsage float64
	if total == 0 && idle == 0 {
		cpuPercentUsage = 0.00
	} else {
		cpuPercentUsage = (float64(total) - float64(idle)) / float64(total)
	}

	return cpuPercentUsage
}
