// proc_windows.go reads a process's creation time on Windows, so a reused
// pid isn't taken for the writer of a recovery copy.

package autorecover

import "syscall"

const processQueryLimited = 0x1000

// processStart is when process pid was created (a FILETIME), which tells
// it from a later process given the same pid.
func processStart(pid int) (int64, bool) {
	h, err := syscall.OpenProcess(processQueryLimited, false, uint32(pid))
	if err != nil {
		return 0, false
	}
	defer syscall.CloseHandle(h)
	var created, exited, kernel, user syscall.Filetime
	if syscall.GetProcessTimes(h, &created, &exited, &kernel, &user) != nil {
		return 0, false
	}
	return created.Nanoseconds(), true
}
