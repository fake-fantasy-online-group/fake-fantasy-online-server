//go:build !unix

package scene

// CPUNS=0 explicitly means unavailable on this diagnostic host. Wall time and
// allocation metrics remain available; do not relabel wall time as process CPU.
func replicationCPUTime() int64 { return 0 }
