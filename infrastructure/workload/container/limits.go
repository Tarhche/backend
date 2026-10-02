package container

import "math"

// memoryLimit is a task's memory limit, as docker takes it.
//
// Docker counts a memory limit in bytes, and so does the workload: a compose
// size like "256M" is turned into bytes when it is read, and the code runner
// and the configured defaults are written in bytes too. So nothing is
// converted on the way through. Taking it for mebibytes and multiplying would
// make a 256 MiB limit 256 TiB, which no node has, and so no limit at all.
//
// Zero is docker's own "no limit". A limit too large for docker's int64 is no
// limit in practice either, and is capped rather than let wrap around into a
// negative number docker would refuse.
func memoryLimit(bytes uint64) int64 {
	return int64(min(bytes, math.MaxInt64))
}
