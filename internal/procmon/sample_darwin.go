//go:build darwin

package procmon

import (
	"syscall"
	"time"
)

func sampleProcess() (time.Duration, uint64) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, 0
	}
	cpu := timevalDuration(ru.Utime) + timevalDuration(ru.Stime)
	return cpu, uint64(ru.Maxrss)
}

func timevalDuration(tv syscall.Timeval) time.Duration {
	return time.Duration(tv.Sec)*time.Second + time.Duration(tv.Usec)*time.Microsecond
}
