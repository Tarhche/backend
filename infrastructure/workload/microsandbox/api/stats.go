package api

// Stats is what a run is using, as microsandbox measures its VM, or what a
// node's running runs use between them. Sizes and counters are bytes.
//
// There is no count of processes, because microsandbox does not report the
// guest's, so a client reports none.
type Stats struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsage   uint64  `json:"memory_usage"`
	MemoryLimit   uint64  `json:"memory_limit"`
	NetworkInput  uint64  `json:"network_input"`  // received
	NetworkOutput uint64  `json:"network_output"` // sent
	BlockInput    uint64  `json:"block_input"`    // read from disk
	BlockOutput   uint64  `json:"block_output"`   // written to disk
}
