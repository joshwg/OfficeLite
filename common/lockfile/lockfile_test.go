// Tests of owner-file names and encoding, lock dialog text, cloud-folder
// detection and the open-document registry, in and across processes.

package lockfile

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestOwnerName checks Word's and Excel's owner file names and the
// candidates ownerPaths tries.
func TestOwnerName(t *testing.T) {
	for name, want := range map[string]string{
		"Report2024.docx":        "~$port2024.docx", // eight or more: two dropped
		"Reports1.docx":          "~$ports1.docx",
		"Report1.docx":           "~$eport1.docx", // seven: one dropped
		"Report.docx":            "~$Report.docx", // six or fewer: kept
		"Memo.doc":               "~$Memo.doc",
		"Überblick.docx":         "~$erblick.docx", // characters, not bytes
		"Book1.xlsx":             "~$Book1.xlsx",   // Excel: always the whole name
		"Quarterly results.xlsx": "~$Quarterly results.xlsx",
		"notes.txt":              "~$notes.txt",
	} {
		if got := OwnerName(name); got != want {
			t.Errorf("OwnerName(%q) = %q, want %q", name, got, want)
		}
	}
	dir := string(filepath.Separator) + "d"
	got := ownerPaths(filepath.Join(dir, "Longnotes.txt"))
	want := []string{filepath.Join(dir, "~$Longnotes.txt"), filepath.Join(dir, "~$ngnotes.txt")}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("ownerPaths = %q, want %q", got, want)
	}
	got = ownerPaths(filepath.Join(dir, "Report2024.docx"))
	want = []string{filepath.Join(dir, "~$port2024.docx"), filepath.Join(dir, "~$Report2024.docx")}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("ownerPaths = %q, want %q", got, want)
	}
}

// layout builds an owner file byte by byte from the documented layout,
// independently of Encode.
func layout(f Format, ansi []byte, utf []uint16) []byte {
	size, at, pad := 162, 54, byte(0)
	if f == Excel {
		size, at, pad = 165, 55, 0x20
	}
	var b bytes.Buffer
	b.WriteByte(byte(len(ansi)))
	b.Write(ansi)
	for b.Len() < at {
		b.WriteByte(pad)
	}
	b.WriteByte(byte(len(utf)))
	b.WriteByte(0)
	for _, c := range utf {
		b.WriteByte(byte(c))
		b.WriteByte(byte(c >> 8))
	}
	for b.Len() < size {
		if f == Excel && (b.Len()-at)%2 == 0 {
			b.WriteByte(0x20)
		} else {
			b.WriteByte(0)
		}
	}
	return b.Bytes()
}

// TestEncode checks both formats' byte layout, the Windows-1252 ANSI copy
// and the 52-character limit.
func TestEncode(t *testing.T) {
	josh := []uint16{'J', 'o', 's', 'h'}
	word := Encode(Word, "Josh")
	if want := layout(Word, []byte("Josh"), josh); !bytes.Equal(word, want) {
		t.Errorf("Word:\n% x\nwant\n% x", word, want)
	}
	if len(word) != 162 || word[0] != 4 || string(word[1:5]) != "Josh" || word[54] != 4 || word[56] != 'J' || word[57] != 0 {
		t.Errorf("Word layout: % x", word)
	}
	excel := Encode(Excel, "Josh")
	if want := layout(Excel, []byte("Josh"), josh); !bytes.Equal(excel, want) {
		t.Errorf("Excel:\n% x\nwant\n% x", excel, want)
	}
	if len(excel) != 165 || excel[5] != ' ' || excel[54] != ' ' || excel[55] != 4 || excel[57] != 'J' ||
		excel[65] != ' ' || excel[66] != 0 || excel[164] != 0 {
		t.Errorf("Excel layout: % x", excel)
	}
	// Windows-1252 in the ANSI copy, "?" for what it lacks.
	b := Encode(Word, "José 東")
	if want := layout(Word, []byte{'J', 'o', 's', 0xE9, ' ', '?'}, []uint16{'J', 'o', 's', 0xE9, ' ', 0x6771}); !bytes.Equal(b, want) {
		t.Errorf("accents:\n% x\nwant\n% x", b, want)
	}
	// Office keeps 52 characters.
	long := strings.Repeat("abcdefghij", 6)
	if got, _ := Decode(Encode(Excel, long)); got != long[:52] {
		t.Errorf("long name decodes as %q", got)
	}
}

// TestDecode checks Encode round trips, the ANSI fallback and rejection of
// malformed owner files.
func TestDecode(t *testing.T) {
	for _, name := range []string{"Josh", "José Müller", "東京", strings.Repeat("x", 32), strings.Repeat("y", 52), "A"} {
		for _, f := range []Format{Word, Excel} {
			if got, ok := Decode(Encode(f, name)); !ok || got != name {
				t.Errorf("format %d: %q decodes as %q, %v", f, name, got, ok)
			}
		}
	}
	// No UTF-16 copy: the ANSI name.
	b := layout(Word, []byte{'R', 0xE9, 'a'}, nil)
	if got, ok := Decode(b); !ok || got != "Réa" {
		t.Errorf("ANSI only: %q, %v", got, ok)
	}
	for _, bad := range [][]byte{nil, make([]byte, 162), make([]byte, 100), append([]byte{60}, make([]byte, 170)...)} {
		if got, ok := Decode(bad); ok {
			t.Errorf("Decode(% x...) = %q", bad[:min(len(bad), 4)], got)
		}
	}
}

// TestStatus checks Status's lock state, messages, buttons and choices.
func TestStatus(t *testing.T) {
	s := Status{Owner: "Alice"}
	if !s.Locked() || !strings.Contains(s.Message("a.docx"), "locked for editing by Alice") || len(s.Buttons()) != 3 {
		t.Errorf("owner: %q %q", s.Message("a.docx"), s.Buttons())
	}
	if s.Choice(0) != ReadOnly || s.Choice(1) != Anyway || s.Choice(2) != Cancel || s.Choice(-1) != Cancel {
		t.Error("three-button choices")
	}
	s = Status{Elsewhere: true, Owner: "Alice"}
	if !strings.Contains(s.Message("a.docx"), "another OfficeLite window") || len(s.Buttons()) != 2 || s.Choice(1) != Cancel {
		t.Errorf("elsewhere: %q %q", s.Message("a.docx"), s.Buttons())
	}
	if (Status{}).Locked() {
		t.Error("empty status locked")
	}
	if m := (Status{InUse: true}).Message("b.xlsx"); !strings.Contains(m, "in use by another program") {
		t.Errorf("in use: %q", m)
	}
}

// TestSaveError checks the plain message for a save that failed for
// another reason than a lock.
func TestSaveError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.docx")
	if got := SaveError(p, errors.New("disk full")); got != "Save failed: disk full" {
		t.Errorf("plain error: %q", got)
	}
}

// TestCloudFolder checks cloud-sync folders by environment variable and by
// path component.
func TestCloudFolder(t *testing.T) {
	t.Setenv("OneDrive", filepath.Join(string(filepath.Separator), "users", "josh", "Stuff"))
	for path, want := range map[string]bool{
		`\\server\share\Dropbox\a.docx`:                                               true,
		`C:\Users\josh\OneDrive - Acme\a`:                                             true,
		"/home/josh/Google Drive/a.xlsx":                                              true,
		filepath.Join(string(filepath.Separator), "users", "josh", "Stuff", "a.docx"): true,
		`\\server\share\Team\a.docx`:                                                  false,
		"/home/josh/docs/a.docx":                                                      false,
	} {
		if got := cloudFolder(path); got != want {
			t.Errorf("cloudFolder(%q) = %v", path, got)
		}
	}
}

// testExe is this test binary, to run again as another process. Not on
// Windows, where a test exe spawning itself trips antivirus heuristics;
// the same-process handle tests in lock_windows_test.go cover locks
// there.
func testExe(t *testing.T) string {
	if runtime.GOOS == "windows" {
		t.Skip("no self-spawning test processes on Windows")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	return exe
}

// useRegistry points the registry at a fresh temporary directory for the
// test and returns it.
func useRegistry(t *testing.T) string {
	dir := t.TempDir()
	old := registryDir
	registryDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { registryDir = old })
	return dir
}

// TestRegistryInProcess checks Activate, re-registration on Save As,
// cleanup on Close and a nil Doc within one process.
func TestRegistryInProcess(t *testing.T) {
	dir := useRegistry(t)
	path := filepath.Join(t.TempDir(), "Book.xlsx")
	var raised atomic.Int32
	d := NewDoc(func() { raised.Add(1) })
	d.Set(path, false)
	if !Activate(path) || raised.Load() != 1 {
		t.Fatalf("Activate: raised %d", raised.Load())
	}
	if es, _ := os.ReadDir(dir); len(es) != 1 {
		t.Fatalf("registry holds %v", es)
	}
	if Check(path).Elsewhere {
		t.Error("this process counts as elsewhere")
	}
	// Save As: the new name is registered instead.
	other := filepath.Join(filepath.Dir(path), "Copy.xlsx")
	d.Set(other, true)
	if Activate(path) || !Activate(other) || raised.Load() != 2 {
		t.Errorf("after Save As: raised %d", raised.Load())
	}
	d.Close()
	if Activate(other) {
		t.Error("closed document activated")
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Errorf("registry left %v", es)
	}
	var nilDoc *Doc
	nilDoc.Set(path, true)
	nilDoc.Close()
}

// TestRegistryTwoWindows checks that a file open in two windows of this
// process stays reachable until the last one closes, whichever closes
// first, and that Activate raises the oldest.
func TestRegistryTwoWindows(t *testing.T) {
	for _, firstCloses := range []bool{true, false} {
		dir := useRegistry(t)
		path := filepath.Join(t.TempDir(), "Twice.xlsx")
		var r1, r2 atomic.Int32
		d1 := NewDoc(func() { r1.Add(1) })
		d2 := NewDoc(func() { r2.Add(1) })
		d1.Set(path, false)
		d2.Set(path, false) // Open Read-Only, say
		if !Activate(path) || r1.Load() != 1 || r2.Load() != 0 {
			t.Fatalf("both open: raised %d, %d", r1.Load(), r2.Load())
		}
		gone, left, rLeft := d1, d2, &r2
		if !firstCloses {
			gone, left, rLeft = d2, d1, &r1
		}
		gone.Close()
		before := rLeft.Load()
		if !Activate(path) || rLeft.Load() != before+1 {
			t.Errorf("firstCloses=%v: remaining window not raised", firstCloses)
		}
		if es, _ := os.ReadDir(dir); len(es) != 1 {
			t.Errorf("firstCloses=%v: registry holds %v", firstCloses, es)
		}
		left.Close()
		if Activate(path) {
			t.Errorf("firstCloses=%v: closed file activated", firstCloses)
		}
		if es, _ := os.ReadDir(dir); len(es) != 0 {
			t.Errorf("firstCloses=%v: registry left %v", firstCloses, es)
		}
	}
}

// TestRegistryStale checks that a dead process's entry is dropped and an
// unreachable live one counts as open elsewhere.
func TestRegistryStale(t *testing.T) {
	dir := useRegistry(t)
	path := filepath.Join(t.TempDir(), "a.docx")
	k := key(path)
	write := func(pid, port int) string {
		name := filepath.Join(dir, hashKey(k)+"-"+strconv.Itoa(pid)+"-1")
		b, _ := json.Marshal(entry{PID: pid, Path: path, Port: port})
		if err := os.WriteFile(name, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return name
	}
	// A process that is gone: its entry is dropped.
	cmd := exec.Command(testExe(t), "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	dead := write(cmd.Process.Pid, 1)
	if openElsewhere(path) || Activate(path) {
		t.Error("dead process's entry counts")
	}
	if _, err := os.Stat(dead); !os.IsNotExist(err) {
		t.Error("dead process's entry kept")
	}
	// A live one that can't be reached: open elsewhere, not activated.
	live := write(os.Getppid(), 0)
	if Activate(path) {
		t.Error("unreachable process activated")
	}
	if st := Check(path); !st.Elsewhere || !st.Locked() {
		t.Errorf("Check = %+v", st)
	}
	os.Remove(live)
}

// TestHandOff checks the hand-off protocol: raising, a wrong token and a
// file not open.
func TestHandOff(t *testing.T) {
	useRegistry(t)
	path := filepath.Join(t.TempDir(), "a.docx")
	raised := make(chan bool, 1)
	d := NewDoc(func() { raised <- true })
	d.Set(path, false)
	defer d.Close()
	port, token := listen()
	if port == 0 {
		t.Skip("can't listen on localhost")
	}
	e := entry{PID: os.Getpid(), Path: path, Port: port, Token: token}
	if !handOff(e, path) {
		t.Fatal("hand-off refused")
	}
	select {
	case <-raised:
	case <-time.After(time.Second):
		t.Fatal("window not raised")
	}
	if handOff(entry{Port: port, Token: "wrong"}, path) {
		t.Error("wrong token accepted")
	}
	if handOff(e, filepath.Join(filepath.Dir(path), "other.docx")) {
		t.Error("hand-off of a file not open")
	}
}

// TestHelperProcess is the "other OfficeLite" of TestCrossProcess: it
// registers a document, says "ready", says "raised" when asked to show
// it, and exits without cleaning up when its stdin closes, as a crash
// would.
func TestHelperProcess(t *testing.T) {
	path := os.Getenv("LOCKFILE_HELPER_PATH")
	if path == "" {
		t.Skip("helper only")
	}
	dir := os.Getenv("LOCKFILE_HELPER_REG")
	registryDir = func() (string, error) { return dir, nil }
	d := NewDoc(func() { fmt.Println("raised") })
	d.Set(path, false)
	fmt.Println("ready")
	bufio.NewReader(os.Stdin).ReadString('\n')
	os.Exit(0)
}

// TestCrossProcess checks Check and Activate against a helper process, and
// cleanup of its entry once it exits.
func TestCrossProcess(t *testing.T) {
	dir := useRegistry(t)
	path := filepath.Join(t.TempDir(), "Shared.docx")
	if err := os.WriteFile(path, []byte("doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(testExe(t), "-test.run=^TestHelperProcess$")
	cmd.Env = append(os.Environ(), "LOCKFILE_HELPER_PATH="+path, "LOCKFILE_HELPER_REG="+dir)
	// A directory deeper than this process's, so that a relative path
	// resolves to something else there instead of climbing to the root.
	cmd.Dir = filepath.Join(t.TempDir(), strings.Repeat("d/", 12))
	if err := os.MkdirAll(cmd.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	out := bufio.NewReader(stdout)
	if line, _ := out.ReadString('\n'); line != "ready\n" {
		t.Fatalf("helper said %q", line)
	}
	if st := Check(path); !st.Elsewhere || st.InUse || st.Owner != "" {
		t.Errorf("Check while open elsewhere = %+v", st)
	}
	if !Activate(path) {
		t.Fatal("not handed off")
	}
	if line, _ := out.ReadString('\n'); line != "raised\n" {
		t.Errorf("helper said %q", line)
	}
	// A relative path means something else in the helper's directory.
	wd, _ := os.Getwd()
	if rel, err := filepath.Rel(wd, path); err == nil && !filepath.IsAbs(rel) {
		if !Activate(rel) {
			t.Error("relative path not handed off")
		} else if line, _ := out.ReadString('\n'); line != "raised\n" {
			t.Errorf("helper said %q after a relative path", line)
		}
	}
	stdin.Close()
	cmd.Wait()
	if st := Check(path); st.Locked() {
		t.Errorf("Check after the helper exited = %+v", st)
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Errorf("crashed helper's entry kept: %v", es)
	}
}
