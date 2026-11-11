// Tests for the SaveAs lifecycle: the saving flag, a canceled dialog, a
// failing dialog, a written file, a failed write and a target open
// elsewhere.

package gui

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"officelite/common/lockfile"
)

// errDeclined stands for explorer.ErrUserDecline in these tests.
var errDeclined = errors.New("declined")

// runUntil runs q's posted calls until done reports true, failing t after
// a few seconds.
func runUntil(t *testing.T, q *CallQueue, done func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); !done(); {
		if time.Now().After(end) {
			t.Fatal("timed out")
		}
		q.Run()
		time.Sleep(time.Millisecond)
	}
}

// testJob is a SaveAs of the string "content" whose dialog is create;
// the outcome lands in *got.
func testJob(busy *atomic.Bool, create func(string) (io.WriteCloser, error), got *[]string) (SaveAs[string], *CallQueue) {
	q := NewCallQueue(nil)
	return SaveAs[string]{
		Name:     "Doc.txt",
		Create:   create,
		Declined: errDeclined,
		Calls:    q,
		Busy:     busy,
		Snapshot: func() string { return "content" },
		Write: func(s string, w io.Writer) error {
			_, err := io.WriteString(w, s)
			return err
		},
		Done: func(s string, r SaveAsResult) {
			if busy.Load() {
				*got = append(*got, "busy")
			}
			msg := "ok"
			if r.Err != nil {
				msg = r.Err.Error()
			}
			if r.Shared {
				msg += "|shared"
			}
			*got = append(*got, s+"|"+filepath.Base(r.Path)+"|"+msg)
		},
		Cancel: func() { *got = append(*got, "cancel") },
	}, q
}

// TestSaveAsLifecycle checks Busy and the callbacks for a canceled
// dialog, a dialog that fails and a file written.
func TestSaveAsLifecycle(t *testing.T) {
	var busy atomic.Bool
	var got []string
	j, q := testJob(&busy, func(string) (io.WriteCloser, error) { return nil, errDeclined }, &got)
	j.Start()
	if !busy.Load() {
		t.Fatal("not busy after Start")
	}
	runUntil(t, q, func() bool { return !busy.Load() })
	if len(got) != 1 || got[0] != "cancel" {
		t.Errorf("canceled: %q", got)
	}

	got = nil
	j, q = testJob(&busy, func(string) (io.WriteCloser, error) { return nil, errors.New("no disk") }, &got)
	j.Start()
	runUntil(t, q, func() bool { return !busy.Load() })
	if len(got) != 1 || got[0] != "|Doc.txt|no disk" {
		t.Errorf("failed dialog: %q", got)
	}

	got = nil
	path := filepath.Join(t.TempDir(), "Chosen.txt")
	j, q = testJob(&busy, func(string) (io.WriteCloser, error) { return os.Create(path) }, &got)
	j.Start()
	runUntil(t, q, func() bool { return !busy.Load() })
	if len(got) != 1 || got[0] != "content|Chosen.txt|ok" {
		t.Errorf("written: %q", got)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "content" {
		t.Errorf("file: %q, %v", b, err)
	}
}

// failingWrite is a SaveAs Write that writes part of the document, then
// fails.
func failingWrite(_ string, w io.Writer) error {
	if _, err := io.WriteString(w, "half"); err != nil {
		return err
	}
	return errors.New("disk full")
}

// assertUnchanged fails t unless path still holds want and its folder
// holds nothing else (no temporary file left).
func assertUnchanged(t *testing.T, path, want string) {
	t.Helper()
	if b, err := os.ReadFile(path); err != nil || string(b) != want {
		t.Errorf("target now %q, %v; want %q", b, err, want)
	}
	if ents, err := os.ReadDir(filepath.Dir(path)); err != nil || len(ents) != 1 {
		t.Errorf("folder holds %v, %v", ents, err)
	}
}

// TestSaveAsFailedWriteKeepsTarget: a write that fails leaves a file the
// dialog chose as it was, with no temporary file, whether the native
// dialog gave its path or the fallback dialog created (opened) it.
func TestSaveAsFailedWriteKeepsTarget(t *testing.T) {
	var busy atomic.Bool
	var got []string
	path := filepath.Join(t.TempDir(), "Old.txt")
	if err := os.WriteFile(path, []byte("old content"), 0o644); err != nil {
		t.Fatal(err)
	}
	j, q := testJob(&busy, nil, &got)
	j.Save = func(string, ...FileFilter) (string, error) { return path, nil }
	j.Write = failingWrite
	j.Start()
	runUntil(t, q, func() bool { return !busy.Load() })
	if len(got) != 1 || got[0] != "content|Old.txt|disk full" {
		t.Errorf("native dialog: %q", got)
	}
	assertUnchanged(t, path, "old content")

	got = nil
	j, q = testJob(&busy, func(string) (io.WriteCloser, error) { return os.OpenFile(path, os.O_WRONLY, 0) }, &got)
	j.Save = func(string, ...FileFilter) (string, error) { return "", ErrNoFileDialog }
	j.Write = failingWrite
	j.Start()
	runUntil(t, q, func() bool { return !busy.Load() })
	if len(got) != 1 || got[0] != "content|Old.txt|disk full" {
		t.Errorf("fallback dialog: %q", got)
	}
	assertUnchanged(t, path, "old content")
}

// TestSaveAsLockedTarget: a Save As onto a file open elsewhere asks
// first, before writing where the dialog gives the path; declined,
// nothing is written and Cancel runs; accepted, the file is written and
// the result says it is shared. An export (no Ask) doesn't check.
func TestSaveAsLockedTarget(t *testing.T) {
	var busy atomic.Bool
	var got []string
	path := filepath.Join(t.TempDir(), "Theirs.txt")
	if err := os.WriteFile(path, []byte("theirs"), 0o644); err != nil {
		t.Fatal(err)
	}
	locked := func(string) lockfile.Status { return lockfile.Status{Owner: "Alice"} }
	for _, ok := range []bool{false, true} {
		got = nil
		j, q := testJob(&busy, nil, &got)
		j.Save = func(string, ...FileFilter) (string, error) { return path, nil }
		j.Check = locked
		asked := ""
		j.Ask = func(st lockfile.Status, p string, answer func(bool)) {
			asked = st.Owner + "@" + filepath.Base(p)
			if b, _ := os.ReadFile(path); string(b) != "theirs" {
				t.Errorf("written before asking: %q", b)
			}
			answer(ok)
		}
		j.Start()
		runUntil(t, q, func() bool { return !busy.Load() })
		if asked != "Alice@Theirs.txt" {
			t.Errorf("asked %q", asked)
		}
		b, _ := os.ReadFile(path)
		switch {
		case !ok && (len(got) != 1 || got[0] != "cancel" || string(b) != "theirs"):
			t.Errorf("declined: %q, file %q", got, b)
		case ok && (len(got) != 1 || got[0] != "content|Theirs.txt|ok|shared" || string(b) != "content"):
			t.Errorf("accepted: %q, file %q", got, b)
		}
	}

	got = nil
	j, q := testJob(&busy, nil, &got)
	j.Save = func(string, ...FileFilter) (string, error) { return path, nil }
	j.Check = func(string) lockfile.Status { t.Error("an export checked the lock"); return lockfile.Status{} }
	j.Start()
	runUntil(t, q, func() bool { return !busy.Load() })
	if len(got) != 1 || got[0] != "content|Theirs.txt|ok" {
		t.Errorf("export: %q", got)
	}
}

// TestAskSaveLocked checks the Save As File In Use dialog's buttons: Save
// Anyway or Cancel, and only OK for a file in another OfficeLite window.
func TestAskSaveLocked(t *testing.T) {
	var dlg Dialog
	var got []bool
	answer := AskSaveLocked(&dlg, lockfile.Status{InUse: true}, "/x/a.docx", func(ok bool) { got = append(got, ok) })
	if len(dlg.Buttons) != 2 || !strings.Contains(dlg.Message, "a.docx is in use") {
		t.Errorf("in use: %q %q", dlg.Buttons, dlg.Message)
	}
	answer(0)
	answer(1)
	answer(-1)
	answer = AskSaveLocked(&dlg, lockfile.Status{Elsewhere: true}, "/x/a.docx", func(ok bool) { got = append(got, ok) })
	if len(dlg.Buttons) != 1 {
		t.Errorf("elsewhere: %q", dlg.Buttons)
	}
	answer(0)
	if want := []bool{true, false, false, false}; len(got) != 4 || got[0] != want[0] || got[1] || got[2] || got[3] {
		t.Errorf("answers %v, want %v", got, want)
	}
}
