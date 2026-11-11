// scan.go finds orphaned recovery copies (Orphans), cleans up stale and
// stray files, and claims, releases and discards entries.

package autorecover

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"officelite/common/limits"
	"officelite/common/lockfile"
)

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
// go), newest first. Copies older than limits.RecoveryMaxAge, sidecars
// without a copy and stray files are deleted; copies of running processes
// are left alone.
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
			if old(e.Sidecar, limits.RecoveryStrayAge) {
				if err := os.Remove(e.Sidecar); err != nil && !os.IsNotExist(err) {
					log.Printf("remove damaged AutoRecover sidecar %s: %v", e.Sidecar, err)
				}
			}
			continue
		}
		if alive(e.PID, e.Started) {
			continue
		}
		if _, err := os.Stat(e.Copy); err != nil || now().Sub(e.Saved) > limits.RecoveryMaxAge {
			Discard(e)
			continue
		}
		out = append(out, e)
	}
	// Copies whose sidecar was never written, atomicfile's temporary
	// files and claim files, left by a crash in the middle of a write.
	for _, de := range des {
		name := de.Name()
		ext := filepath.Ext(name)
		stray := ext == ".tmp" || ext == ".claim" || ((ext == ".xlsx" || ext == ".docx") && !known[strings.TrimSuffix(name, ext)])
		if p := filepath.Join(dir, name); stray && !de.IsDir() && old(p, limits.RecoveryStrayAge) {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				log.Printf("remove stale AutoRecover file %s: %v", p, err)
			}
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

// ClaimAll claims each entry that no other running OfficeLite holds and
// returns the ones now this process's. Claiming is exclusive: each entry
// is re-checked and claimed while holding its claim file (see claimOne),
// so of two OfficeLites starting at once only one keeps an entry. Entries
// that can't be claimed are left out.
func ClaimAll(entries []Entry) []Entry {
	return claimAll(entries, os.Getpid(), selfStart)
}

// claimAll is ClaimAll for the process pid started at started.
func claimAll(entries []Entry, pid int, started int64) []Entry {
	var mine []Entry
	for _, e := range entries {
		if claimOne(e, pid, started) {
			mine = append(mine, e)
		}
	}
	return mine
}

// claimPath is the claim file of the entry whose sidecar is at sidecar.
func claimPath(sidecar string) string {
	return strings.TrimSuffix(sidecar, ".json") + ".claim"
}

// claimOne claims e for process pid (started at started) and reports
// whether it is now that process's. It creates e's claim file exclusively
// (O_EXCL: of racing claimers only the creator goes on), re-reads the
// sidecar, writes its own pid there unless another live process already
// holds the entry, and removes the claim file again. A claim file left by
// a claimer that died is removed and the exclusive create retried once.
// (Two claimers taking over the same dead claimer's file at the same
// moment could both succeed; that needs a crash inside this short window
// first.)
func claimOne(e Entry, pid int, started int64) bool {
	claim := claimPath(e.Sidecar)
	f, err := os.OpenFile(claim, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil && os.IsExist(err) && staleClaim(claim) {
		removeClaim(claim)
		f, err = os.OpenFile(claim, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	}
	if err != nil {
		return false // another process is claiming it, or the directory is unwritable
	}
	defer removeClaim(claim)
	_, werr := fmt.Fprintf(f, "%d %d", pid, started)
	if cerr := f.Close(); werr != nil || cerr != nil {
		return false
	}
	got, err := readEntry(e.Sidecar)
	if err != nil {
		return false
	}
	if got.PID == pid && got.Started == started {
		return true // already ours
	}
	if alive(got.PID, got.Started) {
		return false // another OfficeLite claimed it first
	}
	e.PID, e.Started = pid, started
	return writeEntry(e.Sidecar, e) == nil
}

// removeClaim deletes the claim file at path, retrying a while on
// failure: on Windows another claimer reading it blocks deletion for a
// moment.
func removeClaim(path string) {
	err := os.Remove(path)
	for i := 1; i <= limits.RecoveryClaimRemoveRetries && err != nil && !os.IsNotExist(err); i++ {
		time.Sleep(time.Duration(i) * limits.RecoveryClaimRemoveBackoff)
		err = os.Remove(path)
	}
	if err != nil && !os.IsNotExist(err) {
		log.Printf("remove AutoRecover claim %s: %v", path, err)
	}
}

// staleClaim reports whether the claim file at path was left by a claimer
// that no longer runs: the process it names is gone, or it names none and
// is older than limits.RecoveryClaimStale.
func staleClaim(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false // gone already (the retry decides), or unreadable
	}
	var pid int
	var started int64
	if n, _ := fmt.Sscan(string(b), &pid, &started); n == 2 && pid > 0 {
		return !alive(pid, started)
	}
	return old(path, limits.RecoveryClaimStale)
}

// Release lets e go again (Document Recovery's Close keeps it for the next
// start).
func Release(e Entry) error {
	e.PID, e.Started = 0, 0
	return writeEntry(e.Sidecar, e)
}

// Discard deletes a recovery copy and its sidecar.
func Discard(e Entry) {
	for _, path := range []string{e.Copy, e.Sidecar} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("remove AutoRecover file %s: %v", path, err)
		}
	}
}
