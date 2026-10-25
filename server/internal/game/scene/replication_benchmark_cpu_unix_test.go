//go:build unix

package scene

import "syscall"

func replicationCPUTime() int64 {
	var usage syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &usage)
	return usage.Utime.Nano() + usage.Stime.Nano()
}
