package procinfo

import (
	"github.com/c9s/goprocinfo/linux"
	"github.com/khanzadimahdi/testproject/domain/runner/stats"
)

const (
	memoryInfoPath string = "/proc/meminfo" // See https://godoc.org/github.com/c9s/goprocinfo/linux#MemInfo
	diskPath       string = "/"             // See https://godoc.org/github.com/c9s/goprocinfo/linux#Disk
	statPath       string = "/proc/stat"    // See https://godoc.org/github.com/c9s/goprocinfo/linux#CPUStat
	avgLoadPath    string = "/proc/loadavg" // See https://godoc.org/github.com/c9s/goprocinfo/linux#LoadAvg
)

type StatsCollector struct {
	memInfoPath string
	diskPath    string
	statPath    string
	avgLoadPath string
}

var _ stats.Collector = &StatsCollector{}

func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		memInfoPath: memoryInfoPath,
		diskPath:    diskPath,
		statPath:    statPath,
		avgLoadPath: statPath,
	}
}

func (s *StatsCollector) Collect() (stats.Stats, error) {
	memoryStats, err := s.memoryInfo()
	if err != nil {
		return stats.Stats{}, err
	}

	diskStats, err := s.diskInfo()
	if err != nil {
		return stats.Stats{}, err
	}

	cpuStats, err := s.cpuStat()
	if err != nil {
		return stats.Stats{}, err
	}

	load, err := s.loadAvg()
	if err != nil {
		return stats.Stats{}, err
	}

	return stats.Stats{
		Memory: stats.Memory{
			Total:     memoryStats.MemTotal,
			Available: memoryStats.MemAvailable,
			SwapTotal: memoryStats.SwapTotal,
			SwapFree:  memoryStats.SwapFree,
		},
		Disk: stats.Disk{
			Total:      diskStats.All,
			Used:       diskStats.Used,
			Available:  diskStats.Free,
			FreeInodes: diskStats.FreeInodes,
		},
		CPU: stats.CPU{
			ID:        cpuStats.Id,
			User:      cpuStats.User,
			Nice:      cpuStats.Nice,
			System:    cpuStats.System,
			Idle:      cpuStats.Idle,
			IOWait:    cpuStats.IOWait,
			IRQ:       cpuStats.IRQ,
			SoftIRQ:   cpuStats.SoftIRQ,
			Steal:     cpuStats.Steal,
			Guest:     cpuStats.Guest,
			GuestNice: cpuStats.GuestNice,
		},
		Load: stats.Load{
			Last1Min:       load.Last1Min,
			Last5Min:       load.Last5Min,
			Last15Min:      load.Last15Min,
			ProcessRunning: load.ProcessRunning,
			ProcessTotal:   load.ProcessTotal,
			LastPID:        load.LastPID,
		},
	}, nil
}

func (s *StatsCollector) memoryInfo() (*linux.MemInfo, error) {
	return linux.ReadMemInfo(s.memInfoPath)
}

func (s *StatsCollector) diskInfo() (*linux.Disk, error) {
	return linux.ReadDisk(s.diskPath)
}

func (s *StatsCollector) cpuStat() (*linux.CPUStat, error) {
	stats, err := linux.ReadStat(s.statPath)
	if err != nil {
		return nil, err
	}

	return &stats.CPUStatAll, nil
}

func (s *StatsCollector) loadAvg() (*linux.LoadAvg, error) {
	return linux.ReadLoadAvg(s.avgLoadPath)
}
