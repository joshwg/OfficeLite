// autorecover_test.go tests the Saver, the orphan scan, the settings file
// and recovery after a real child process crashes.

package autorecover

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"officelite/common/limits"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDoc is a window's document for a Saver.
type fakeDoc struct {
	rev      int
	modified bool
	path     string
	writes   int
}

// snapshot is the document's Snapshot while modified, nil otherwise; its
// Write writes "rev N" and counts the writes.
func (d *fakeDoc) snapshot() *Snapshot {
	if !d.modified {
		return nil
	}
	rev := d.rev
	return &Snapshot{Rev: rev, Path: d.path, Name: filepath.Base(d.path), Format: filepath.Ext(d.path),
		Write: func(w io.Writer) error {
			d.writes++
			_, err := io.WriteString(w, "rev "+string(rune('0'+rev)))
			return err
		}}
}

// files lists the names in dir.
func files(t *testing.T, dir string) []string {
	t.Helper()
	des, _ := os.ReadDir(dir)
	var out []string
	for _, de := range des {
		out = append(out, de.Name())
	}
	return out
}

// TestBaseName checks that copy names are unique, cleaned, cut to length
// and default to "Document".
func TestBaseName(t *testing.T) {
	a, b := baseName(`Q1: "plan"/v2?.xlsx`), baseName(`Q1: "plan"/v2?.xlsx`)
	if a == b {
		t.Errorf("two windows got the same name %q", a)
	}
	if !strings.HasPrefix(a, "Q1_ _plan__v2_~") || strings.ContainsAny(a, `<>:"/\|?*`) {
		t.Errorf("baseName = %q", a)
	}
	if n := baseName(""); !strings.HasPrefix(n, "Document~") {
		t.Errorf("empty name: %q", n)
	}
	if n := baseName(strings.Repeat("x", 200) + ".docx"); len(n) > 80 {
		t.Errorf("long name not cut: %d", len(n))
	}
}

// TestSaver walks a Saver through edits, an unchanged tick, Remove, an
// undo to the saved state and Close, and checks a nil Saver does nothing.
func TestSaver(t *testing.T) {
	dir := t.TempDir()
	doc := &fakeDoc{path: filepath.Join("Users", "Josh", "Budget.xls")}
	s := NewSaver(Excel, func(f func()) { f() }, doc.snapshot)
	s.Dir = dir
	tick := func() { s.Tick(); s.Wait() }

	tick()
	if f := files(t, dir); len(f) != 0 {
		t.Fatalf("unmodified document copied: %v", f)
	}
	doc.modified, doc.rev = true, 1
	tick()
	f := files(t, dir)
	if len(f) != 2 || doc.writes != 1 {
		t.Fatalf("after a change: %v, %d writes", f, doc.writes)
	}
	copyPath, side := s.paths()
	if b, _ := os.ReadFile(copyPath); string(b) != "rev 1" || filepath.Ext(copyPath) != ".xlsx" {
		t.Errorf("copy %s: %q", copyPath, b)
	}
	e, err := readEntry(side)
	if err != nil || e.Path != doc.path || e.Name != "Budget.xls" || e.App != Excel || e.Format != ".xls" ||
		e.PID != os.Getpid() || time.Since(e.Saved) > time.Minute || e.Copy != copyPath {
		t.Errorf("sidecar %+v, %v", e, err)
	}

	// Nothing changed since: not written again.
	tick()
	if doc.writes != 1 {
		t.Errorf("unchanged document written again (%d writes)", doc.writes)
	}
	doc.rev = 2
	tick()
	if b, _ := os.ReadFile(copyPath); doc.writes != 2 || string(b) != "rev 2" {
		t.Errorf("second change: %d writes, %q", doc.writes, b)
	}
	if f := files(t, dir); len(f) != 2 {
		t.Errorf("one window has files %v", f)
	}

	// Saved: the copy goes; editing again writes it again.
	s.Remove()
	s.Wait()
	if f := files(t, dir); len(f) != 0 {
		t.Fatalf("after Remove: %v", f)
	}
	tick()
	if doc.writes != 3 || len(files(t, dir)) != 2 {
		t.Errorf("after editing again: %d writes, %v", doc.writes, files(t, dir))
	}
	// Undone back to the saved state: nothing to recover.
	doc.modified = false
	tick()
	if f := files(t, dir); len(f) != 0 {
		t.Errorf("unmodified again: %v", f)
	}
	doc.modified = true
	tick()
	s.Close()
	s.Wait()
	if f := files(t, dir); len(f) != 0 {
		t.Errorf("after Close: %v", f)
	}
	s.Tick() // closed: nothing
	s.Wait()
	if f := files(t, dir); len(f) != 0 {
		t.Errorf("tick after Close wrote %v", f)
	}
	var nilSaver *Saver
	nilSaver.Tick()
	nilSaver.Remove()
	nilSaver.Close()
}

// TestSaverTimer checks that Start's timer posts a tick that writes a Word
// copy.
func TestSaverTimer(t *testing.T) {
	dir := t.TempDir()
	doc := &fakeDoc{path: "", modified: true, rev: 1}
	ui := make(chan func(), 16)
	s := NewSaver(Word, func(f func()) { ui <- f }, doc.snapshot)
	s.Dir = dir
	s.Interval = func() time.Duration { return 10 * time.Millisecond }
	s.Start()
	defer s.Close()
	select {
	case f := <-ui:
		f() // on the "UI goroutine"
	case <-time.After(5 * time.Second):
		t.Fatal("timer didn't tick")
	}
	s.Wait()
	copyPath, _ := s.paths()
	if filepath.Ext(copyPath) != ".docx" {
		t.Errorf("Word copy %s", copyPath)
	}
	if _, err := os.Stat(copyPath); err != nil {
		t.Error(err)
	}
}

// testExe is this test binary, to run as a child process.
func testExe(t *testing.T) string {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

// deadPID is the pid of a process that has exited.
func deadPID(t *testing.T) int {
	cmd := exec.Command(testExe(t), "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.Process.Pid
}

// writeFake writes a recovery copy and sidecar for e named base in dir and
// returns e with Copy and Sidecar set.
func writeFake(t *testing.T, dir, base string, e Entry) Entry {
	t.Helper()
	e.Sidecar = filepath.Join(dir, base+".json")
	e.Copy = filepath.Join(dir, base+Ext(e.App))
	if err := os.WriteFile(e.Copy, []byte("copy"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeEntry(e.Sidecar, e); err != nil {
		t.Fatal(err)
	}
	return e
}

// TestOrphans checks which entries Orphans lists and cleans up, and Claim,
// Release and Discard.
func TestOrphans(t *testing.T) {
	dir := t.TempDir()
	dead := deadPID(t)
	at := time.Now()
	crashed := writeFake(t, dir, "Budget~01", Entry{Path: "/home/j/Budget.xls", Name: "Budget.xls", App: Excel,
		Format: ".xls", Saved: at.Add(-time.Hour), PID: dead})
	released := writeFake(t, dir, "Document1~02", Entry{Name: "Document1", App: Word, Saved: at, PID: 0})
	live := writeFake(t, dir, "Live~03", Entry{Name: "Live.docx", App: Word, Saved: at, PID: os.Getpid(), Started: selfStart})
	expired := writeFake(t, dir, "Old~04", Entry{Name: "Old.xlsx", App: Excel, Saved: at.Add(-limits.RecoveryMaxAge - time.Hour), PID: dead})
	noCopy := writeFake(t, dir, "Gone~05", Entry{Name: "Gone.xlsx", App: Excel, Saved: at, PID: dead})
	if err := os.Remove(noCopy.Copy); err != nil {
		t.Fatal(err)
	}
	// A copy whose sidecar was never written: kept a while, then deleted.
	young := filepath.Join(dir, "Half~06.xlsx")
	stale := filepath.Join(dir, "Half~07.docx")
	tmp := filepath.Join(dir, "Half~08.xlsx.123.tmp")
	for _, p := range []string{young, stale, tmp} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	day := at.Add(-limits.RecoveryStrayAge - time.Hour)
	if err := os.Chtimes(stale, day, day); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tmp, day, day); err != nil {
		t.Fatal(err)
	}

	got := Orphans(dir)
	if len(got) != 2 || got[0].Sidecar != released.Sidecar || got[1].Sidecar != crashed.Sidecar {
		t.Fatalf("orphans %+v", got)
	}
	if e := got[1]; e.Path != crashed.Path || e.Name != "Budget.xls" || e.Format != ".xls" || e.App != Excel || e.Copy != crashed.Copy {
		t.Errorf("crashed entry %+v", e)
	}
	for _, p := range []string{expired.Copy, expired.Sidecar, noCopy.Sidecar, stale, tmp} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s not cleaned up", filepath.Base(p))
		}
	}
	for _, p := range []string{live.Copy, live.Sidecar, young, crashed.Copy} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s deleted", filepath.Base(p))
		}
	}

	// Claimed by this process's Document Recovery window: not an orphan
	// for another OfficeLite; released, it is again.
	if err := Claim(got[1]); err != nil {
		t.Fatal(err)
	}
	if o := Orphans(dir); len(o) != 1 {
		t.Errorf("claimed entry still listed: %+v", o)
	}
	if err := Release(got[1]); err != nil {
		t.Fatal(err)
	}
	if o := Orphans(dir); len(o) != 2 {
		t.Errorf("released entry not listed: %+v", o)
	}
	Discard(got[0])
	if o := Orphans(dir); len(o) != 1 || o[0].Name != "Budget.xls" {
		t.Errorf("after Discard: %+v", o)
	}
	if Orphans(filepath.Join(dir, "missing")) != nil {
		t.Error("orphans in a missing directory")
	}
}

// TestClaimAll checks that ClaimAll keeps the entries it claimed, drops one
// another process claimed after it, and drops one that can't be written.
func TestClaimAll(t *testing.T) {
	dir := t.TempDir()
	at := time.Now()
	a := writeFake(t, dir, "A~01", Entry{Name: "A.docx", App: Word, Saved: at})
	b := writeFake(t, dir, "B~02", Entry{Name: "B.docx", App: Word, Saved: at})
	gone := a
	gone.Sidecar = t.TempDir() // a directory: the claim can't replace it
	got := ClaimAll([]Entry{a, gone, b})
	if len(got) != 2 || got[0].Sidecar != a.Sidecar || got[1].Sidecar != b.Sidecar {
		t.Fatalf("claimed %+v", got)
	}
	if o := Orphans(dir); len(o) != 0 {
		t.Errorf("claimed entries still orphans: %+v", o)
	}
	// Another live process holds b: a re-claim leaves it to that one.
	other := b
	other.PID, other.Started = 4242, 7
	if err := writeEntry(other.Sidecar, other); err != nil {
		t.Fatal(err)
	}
	fakeAlive(t, func(pid int, started int64) bool { return pid == 4242 || pid == os.Getpid() })
	if got := ClaimAll([]Entry{b}); len(got) != 0 {
		t.Errorf("claimed another live process's entry: %+v", got)
	}
	// Once it is gone, b is free again.
	fakeAlive(t, func(pid int, started int64) bool { return pid == os.Getpid() })
	if got := ClaimAll([]Entry{b}); len(got) != 1 {
		t.Errorf("re-claim of a dead process's entry %+v", got)
	}
	if _, err := os.Stat(claimPath(b.Sidecar)); !os.IsNotExist(err) {
		t.Errorf("claim file left behind: %v", err)
	}
}

// fakeAlive replaces the process-liveness test for the rest of t.
func fakeAlive(t *testing.T, f func(pid int, started int64) bool) {
	t.Helper()
	saved := alive
	alive = f
	t.Cleanup(func() { alive = saved })
}

// TestClaimAllConcurrent checks that when several OfficeLites (simulated
// pids, all alive) claim the same entries at once, each entry ends up
// claimed by exactly one of them, and the sidecar names that one.
func TestClaimAllConcurrent(t *testing.T) {
	const claimers, rounds = 8, 20
	fakeAlive(t, func(pid int, started int64) bool { return pid >= 1000 && pid < 1000+claimers })
	for r := range rounds {
		dir := t.TempDir()
		var entries []Entry
		for i := range 6 {
			entries = append(entries, writeFake(t, dir, fmt.Sprintf("E%d~%02d", r, i), Entry{Name: "E.docx", App: Word, Saved: time.Now()}))
		}
		got := make([][]Entry, claimers)
		var wg sync.WaitGroup
		for c := range claimers {
			wg.Go(func() { got[c] = claimAll(entries, 1000+c, int64(c)) })
		}
		wg.Wait()
		for _, e := range entries {
			var by []int
			for c := range claimers {
				for _, g := range got[c] {
					if g.Sidecar == e.Sidecar {
						by = append(by, 1000+c)
					}
				}
			}
			if len(by) != 1 {
				t.Fatalf("round %d: %s claimed by %v", r, filepath.Base(e.Sidecar), by)
			}
			if side, err := readEntry(e.Sidecar); err != nil || side.PID != by[0] || side.Started != int64(by[0]-1000) {
				t.Errorf("round %d: sidecar names %d/%d, claimer %d (%v)", r, side.PID, side.Started, by[0], err)
			}
			if _, err := os.Stat(claimPath(e.Sidecar)); !os.IsNotExist(err) {
				t.Errorf("claim file left behind: %v", err)
			}
		}
	}
}

// TestStaleClaim checks that a claim file left by a dead claimer, or an
// old one naming nobody, is taken over, and that a live claimer's or a
// fresh empty one blocks the claim.
func TestStaleClaim(t *testing.T) {
	fakeAlive(t, func(pid int, started int64) bool { return pid == 77 })
	dir := t.TempDir()
	for _, tc := range []struct {
		content string
		age     time.Duration
		want    bool
	}{
		{"66 1", 0, true},  // its claimer died
		{"77 1", 0, false}, // its claimer is still claiming
		{"", 0, false},     // being written
		{"", limits.RecoveryClaimStale + time.Minute, true},
	} {
		e := writeFake(t, dir, "S~"+strconv.Itoa(len(files(t, dir))), Entry{Name: "S.docx", App: Word, Saved: time.Now()})
		claim := claimPath(e.Sidecar)
		if err := os.WriteFile(claim, []byte(tc.content), 0o600); err != nil {
			t.Fatal(err)
		}
		if tc.age > 0 {
			at := time.Now().Add(-tc.age)
			if err := os.Chtimes(claim, at, at); err != nil {
				t.Fatal(err)
			}
		}
		got := len(claimAll([]Entry{e}, 88, 0)) == 1
		if got != tc.want {
			t.Errorf("claim file %q aged %v: claimed %v", tc.content, tc.age, got)
		}
		if _, err := os.Stat(claim); tc.want != os.IsNotExist(err) {
			t.Errorf("claim file %q aged %v: removed %v", tc.content, tc.age, os.IsNotExist(err))
		}
	}
}

// TestReusedPID checks that a reused pid, alive but started later than the
// process that wrote the entry, isn't taken for the writer (only Windows
// can tell).
func TestReusedPID(t *testing.T) {
	if selfStart == 0 {
		t.Skip("process start times aren't known here")
	}
	if !alive(os.Getpid(), selfStart) || alive(os.Getpid(), selfStart-1) {
		t.Error("start time not compared")
	}
}

// TestAdopt checks that an adopted copy is no longer an orphan, isn't
// rewritten unchanged, is replaced in place on a change and goes on Close.
func TestAdopt(t *testing.T) {
	dir := t.TempDir()
	e := writeFake(t, dir, "Budget~0a", Entry{Path: "/home/j/Budget.xls", Name: "Budget.xls", App: Excel, PID: 0})
	doc := &fakeDoc{path: "/home/j/Budget.xls", modified: true, rev: 4}
	s := NewSaver(Excel, func(f func()) { f() }, doc.snapshot)
	s.Dir = t.TempDir() // the window's own dir; Adopt keeps the copy where it is
	s.Adopt(e, 4)
	s.Wait()
	if o := Orphans(dir); len(o) != 0 {
		t.Errorf("adopted entry listed: %+v", o)
	}
	s.Tick()
	s.Wait()
	if doc.writes != 0 {
		t.Error("adopted copy written again unchanged")
	}
	doc.rev = 5
	s.Tick()
	s.Wait()
	if b, _ := os.ReadFile(e.Copy); string(b) != "rev 5" || len(files(t, dir)) != 2 {
		t.Errorf("adopted copy not replaced: %q %v", b, files(t, dir))
	}
	s.Close()
	s.Wait()
	if f := files(t, dir); len(f) != 0 {
		t.Errorf("closing the window left %v", f)
	}
}

// TestSettings checks the default interval, reading it from the settings
// file, and SetMinutes saving it while keeping other keys.
func TestSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "OfficeLite", "settings.json")
	old := SettingsPath
	SettingsPath = func() string { return path }
	reset := func() { settings.loaded = false }
	t.Cleanup(func() { SettingsPath = old; reset() })
	reset()
	if m := Minutes(); m != DefaultMinutes {
		t.Errorf("default %d", m)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"other": "kept", "autoRecoverMinutes": 2}`), 0o644); err != nil {
		t.Fatal(err)
	}
	reset()
	if m := Minutes(); m != 2 || UserInterval() != 2*time.Minute {
		t.Errorf("read %d", m)
	}
	if err := SetMinutes(0); err != nil {
		t.Fatal(err)
	}
	if Minutes() != 0 || UserInterval() != 0 {
		t.Error("off not taken")
	}
	reset()
	var s map[string]any
	b, _ := os.ReadFile(path)
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if Minutes() != 0 || s["other"] != "kept" {
		t.Errorf("saved %s", b)
	}
}

// TestCrash is the startup scan end to end: a child process writes a
// recovery copy and is killed; while it runs its copy isn't an orphan,
// afterwards it is.
func TestCrash(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(testExe(t), "-test.run=^TestCrashChild$")
	cmd.Env = append(os.Environ(), "AUTORECOVER_CHILD="+dir)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState != nil {
			return
		}
		if err := cmd.Process.Kill(); err != nil {
			t.Errorf("kill child during cleanup: %v", err)
		}
		if err := cmd.Wait(); err == nil {
			t.Error("child exited without being killed during cleanup")
		}
	})
	ready := make(chan string, 1)
	go func() {
		line, err := bufio.NewReader(out).ReadString('\n')
		if err != nil {
			ready <- ""
			return
		}
		ready <- line
	}()
	select {
	case line := <-ready:
		if line != "ready\n" {
			t.Fatalf("child said %q", line)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("child wrote no copy")
	}
	if o := Orphans(dir); len(o) != 0 {
		t.Fatalf("a running process's copy listed: %+v", o)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("child exited without being killed")
	}
	o := Orphans(dir)
	if len(o) != 1 || o[0].Name != "Report.docx" || o[0].App != Word || o[0].PID != cmd.Process.Pid {
		t.Fatalf("after the crash: %+v", o)
	}
	if b, _ := os.ReadFile(o[0].Copy); string(b) != "rev 7" {
		t.Errorf("copy %q", b)
	}
}

// TestCrashChild is TestCrash's child process.
func TestCrashChild(t *testing.T) {
	dir := os.Getenv("AUTORECOVER_CHILD")
	if dir == "" {
		t.Skip("run by TestCrash")
	}
	doc := &fakeDoc{path: "/docs/Report.docx", modified: true, rev: 7}
	ui := make(chan func(), 4)
	s := NewSaver(Word, func(f func()) { ui <- f }, doc.snapshot)
	s.Dir = dir
	s.Interval = func() time.Duration { return 10 * time.Millisecond }
	s.Start()
	(<-ui)()
	s.Wait()
	if _, err := os.Stdout.WriteString("ready\n"); err != nil {
		panic(err)
	}
	for f := range ui { // until killed
		f()
	}
}

// TestSaveSettingUnreadable checks that a settings file that is there but
// can't be read is left alone rather than replaced by one holding only
// the key being saved (which lost every other setting).
func TestSaveSettingUnreadable(t *testing.T) {
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("needs a file permission can deny reading")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	old := SettingsPath
	SettingsPath = func() string { return path }
	t.Cleanup(func() { SettingsPath = old })
	if err := os.WriteFile(path, []byte(`{"other": "kept"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if err := SaveSetting("k", true); err == nil {
		t.Error("no error saving over an unreadable settings file")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "kept") {
		t.Errorf("settings file replaced: %s", b)
	}
	// A missing file is still created.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := SaveSetting("k", true); err != nil {
		t.Fatal(err)
	}
	if Setting("k") != true {
		t.Error("setting not saved to a new file")
	}
}
