//go:build windows

package procmon

import (
	"syscall"
	"time"
	"unsafe"
)

var (
	modKernel32                  = syscall.NewLazyDLL("kernel32.dll")
	modPsapi                     = syscall.NewLazyDLL("psapi.dll")
	procGetCurrentProcess        = modKernel32.NewProc("GetCurrentProcess")
	procGetProcessTimes          = modKernel32.NewProc("GetProcessTimes")
	procGetProcessMemoryInfo     = modPsapi.NewProc("GetProcessMemoryInfo")
)

type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

func sampleProcess() (time.Duration, uint64) {
	h, _, _ := procGetCurrentProcess.Call()
	return windowsCPUTime(h), windowsRSS(h)
}

func windowsCPUTime(handle uintptr) time.Duration {
	var creation, exit, kernel, user syscall.Filetime
	r, _, _ := procGetProcessTimes.Call(
		handle,
		uintptr(unsafe.Pointer(&creation)),
		uintptr(unsafe.Pointer(&exit)),
		uintptr(unsafe.Pointer(&kernel)),
		uintptr(unsafe.Pointer(&user)),
	)
	if r == 0 {
		return 0
	}
	return filetimeDuration(kernel) + filetimeDuration(user)
}

func windowsRSS(handle uintptr) uint64 {
	var counters processMemoryCounters
	counters.cb = uint32(unsafe.Sizeof(counters))
	r, _, _ := procGetProcessMemoryInfo.Call(
		handle,
		uintptr(unsafe.Pointer(&counters)),
		uintptr(counters.cb),
	)
	if r == 0 {
		return 0
	}
	return uint64(counters.workingSetSize)
}

func filetimeDuration(ft syscall.Filetime) time.Duration {
	n := int64(ft.HighDateTime)<<32 | int64(ft.LowDateTime)
	return time.Duration(n) * 100 // 100-ns units
}
