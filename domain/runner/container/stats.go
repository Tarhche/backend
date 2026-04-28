package container

// Stats represents the stats of a container
type Stats struct {
	PIDs          uint64  // total number of PIDs
	CPUPercent    float64 // CPU percentage
	MemoryUsage   uint64  // memory usage in bytes
	MemoryLimit   uint64  // memory limit in bytes
	MemoryPercent float64 // memory % (usage / limit)
	NetworkInput  uint64  // network bytes received
	NetworkOutput uint64  // network bytes sent
	BlockInput    uint64  // block bytes read
	BlockOutput   uint64  // block bytes written
}
