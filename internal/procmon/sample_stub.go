//go:build !linux && !windows && !darwin

package procmon

import "time"

func sampleProcess() (time.Duration, uint64) {
	return 0, 0
}
