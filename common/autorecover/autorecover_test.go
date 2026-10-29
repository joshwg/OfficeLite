package autorecover

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func files(t *testing.T, dir string) []string {
	t.Helper()
	des, _ := os.ReadDir(dir)
	var out []string
	for _, de := range des {
		out = append(out, de.Name())
	}
	return out
}

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

func TestOrphans(t *testing.T) {
	dir := t.TempDir()
	dead := deadPID(t)
	at := time.Now()
	crashed := writeFake(t, dir, "Budget~01", Entry{Path: "/home/j/Budget.xls", Name: "Budget.xls", App: Excel,
		Format: ".xls", Saved: at.Add(-time.Hour), PID: dead})
	released := writeFake(t, dir, "Document1~02", Entry{Name: "Document1", App: Word, Saved: at, PID: 0})
	live := writeFake(t, dir, "Live~03", Entry{Name: "Live.docx", App: Word, Saved: at, PID: os.Getpid(), Started: selfStart})
	expired := writeFake(t, dir, "Old~04", Entry{Name: "Old.xlsx", App: Excel, Saved: at.Add(-MaxAge - time.Hour), PID: dead})
	noCopy := writeFake(t, dir, "Gone~05", Entry{Name: "Gone.xlsx", App: Excel, Saved: at, PID: dead})
	os.Remove(noCopy.Copy)
	// A copy whose sidecar was never written: kept a while, then deleted.
	young := filepath.Join(dir, "Half~06.xlsx")
	stale := filepath.Join(dir, "Half~07.docx")
	tmp := filepath.Join(dir, "Half~08.xlsx.123.tmp")
	for _, p := range []string{young, stale, tmp} {
		os.WriteFile(p, []byte("x"), 0o600)
	}
	day := at.Add(-strayAge - time.Hour)
	os.Chtimes(stale, day, day)
	os.Chtimes(tmp, day, day)

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
	Release(got[1])
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

// A reused pid: alive, but started later than the process that wrote the
// entry (Windows only can tell).
func TestReusedPID(t *testing.T) {
	if selfStart == 0 {
		t.Skip("process start times aren't known here")
	}
	if !alive(os.Getpid(), selfStart) || alive(os.Getpid(), selfStart-1) {
		t.Error("start time not compared")
	}
}

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
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(`{"other": "kept", "autoRecoverMinutes": 2}`), 0o644)
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
	json.Unmarshal(b, &s)
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
	defer cmd.Process.Kill()
	ready := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(out).ReadString('\n')
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
	cmd.Process.Kill() // the crash
	cmd.Wait()
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
	os.Stdout.WriteString("ready\n")
	for f := range ui { // until killed
		f()
	}
}
