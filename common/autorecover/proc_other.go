// proc_other.go stubs processStart where process creation times aren't
// read (everywhere but Windows).

//go:build !windows

package autorecover

// processStart isn't known here: a live pid counts as the writer.
func processStart(int) (int64, bool) { return 0, false }
