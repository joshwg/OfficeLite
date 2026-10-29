package autorecover

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"officelite/common/lockfile"
)

// MaxAge is how long a recovery copy nobody dealt with is kept.
const MaxAge = 30 * 24 * time.Hour

// strayAge is how old a copy without a sidecar (or a temporary file) must
// be to be cleaned up: a writer may be between the two files.
const strayAge = 24 * time.Hour

// now is the clock, which tests move.
var now = time.Now

// alive reports whether the process that wrote an entry still runs: its
// pid is live and, where the OS tells, was started when the entry says.
// A pid of 0 belongs to nobody.
var alive = func(pid int, started int64) bool {
	if !lockfile.PIDAlive(pid) {
		return false
	}
	if started == 0 {
		return true
	}
	st, ok := processStart(pid)
	return !ok || st == started
}

// selfStart is this process's start, recorded in the entries it writes.
var selfStart, _ = processStart(os.Getpid())

// Orphans lists the recovery copies in dir ("" for Dir()) that no running
// OfficeLite keeps (it crashed, or a Document Recovery window let them
// go), newest first. Copies older than MaxAge, sidecars without a copy
// and stray files are deleted; copies of running processes are left
// alone.
func Orphans(dir string) []Entry {
	if dir == "" {
		dir = Dir()
	}
	if dir == "" {
		return nil
	}
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Entry
	known := map[string]bool{}
	for _, de := range des {
		name := de.Name()
		if de.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		e, err := readEntry(filepath.Join(dir, name))
		known[strings.TrimSuffix(name, ".json")] = true
		if err != nil {
			if old(e.Sidecar, strayAge) {
				os.Remove(e.Sidecar) // damaged
			}
			continue
		}
		if alive(e.PID, e.Started) {
			continue
		}
		if _, err := os.Stat(e.Copy); err != nil || now().Sub(e.Saved) > MaxAge {
			Discard(e)
			continue
		}
		out = append(out, e)
	}
	// Copies whose sidecar was never written, and atomicfile's temporary
	// files, left by a crash in the middle of a write.
	for _, de := range des {
		name := de.Name()
		ext := filepath.Ext(name)
		stray := ext == ".tmp" || ((ext == ".xlsx" || ext == ".docx") && !known[strings.TrimSuffix(name, ext)])
		if p := filepath.Join(dir, name); stray && !de.IsDir() && old(p, strayAge) {
			os.Remove(p)
		}
	}
	slices.SortFunc(out, func(a, b Entry) int { return b.Saved.Compare(a.Saved) })
	return out
}

// old reports whether file path was last written more than age ago.
func old(path string, age time.Duration) bool {
	fi, err := os.Stat(path)
	return err == nil && now().Sub(fi.ModTime()) > age
}

// Claim marks e as this process's while its Document Recovery window
// lists it, so another OfficeLite started meanwhile doesn't list it too.
func Claim(e Entry) error {
	e.PID, e.Started = os.Getpid(), selfStart
	return writeEntry(e.Sidecar, e)
}

// Release lets e go again (Document Recovery's Close keeps it for the next
// start).
func Release(e Entry) error {
	e.PID, e.Started = 0, 0
	return writeEntry(e.Sidecar, e)
}

// Discard deletes a recovery copy and its sidecar.
func Discard(e Entry) {
	os.Remove(e.Copy)
	os.Remove(e.Sidecar)
}
