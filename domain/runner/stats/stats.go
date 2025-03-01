package stats

// Stats represents the stats of the system
type Stats struct {
	Memory Memory
	Disk   Disk
	CPU    CPU
	Load   Load
}

// Collector represents a collector of stats
type Collector interface {
	Collect() (Stats, error)
}

// Memory represents the memory stats
type Memory struct {
	Total     uint64
	Used      uint64
	Available uint64
	SwapTotal uint64
	SwapFree  uint64
}

// UsedKb returns the used memory in kilobytes
func (m *Memory) UsedKb() uint64 {
	return m.Total - m.Available
}

// UsedPercent returns the percentage of memory used
func (m *Memory) UsedPercent() uint64 {
	return m.Available / m.Total
}

// AvailableKb returns the available memory in kilobytes
func (m *Memory) AvailableKb() uint64 {
	return m.Available
}

// TotalKb returns the total memory in kilobytes
func (m *Memory) TotalKb() uint64 {
	return m.Total
}

// Disk represents the disk stats
type Disk struct {
	Total      uint64
	Used       uint64
	Available  uint64
	FreeInodes uint64
}

// CPU represents the CPU stats
type CPU struct {
	ID        string
	User      uint64
	Nice      uint64
	System    uint64
	Idle      uint64
	IOWait    uint64
	IRQ       uint64
	SoftIRQ   uint64
	Steal     uint64
	Guest     uint64
	GuestNice uint64
}

// UsagePercent returns the percentage of CPU usage
func (c *CPU) UsagePercent() float64 {
	idle := c.Idle + c.IOWait
	nonIdle := c.User + c.Nice + c.System + c.IRQ + c.SoftIRQ + c.Steal
	total := idle + nonIdle

	if total == 0 && idle == 0 {
		return 0.00
	}

	return (float64(total) - float64(idle)) / float64(total)
}

// Load represents the average load
type Load struct {
	Last1Min       float64
	Last5Min       float64
	Last15Min      float64
	ProcessRunning uint64
	ProcessTotal   uint64
	LastPID        uint64
}
