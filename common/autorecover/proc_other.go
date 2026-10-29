//go:build !windows

package autorecover

// processStart isn't known here: a live pid counts as the writer.
func processStart(int) (int64, bool) { return 0, false }
