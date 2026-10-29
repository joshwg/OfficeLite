// Tests for FileDrop: opening dropped files, the drop queue and path
// clean-up.

package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestOpenDropped: documents and workbooks open once each, the first
// MaxDrop of them; folders, programs, pictures and missing files are named
// in the message and never opened.
func TestOpenDropped(t *testing.T) {
	dir := t.TempDir()
	write := func(name, data string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	csv := write("Données ✓.csv", "a,b\n1,2\n")
	txt := write("notes.txt", "hello")
	exe := write("setup.exe", "MZ\x90\x00")
	png := write("photo.png", "\x89PNG\r\n")
	sub := filepath.Join(dir, "Folder")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	var opened []string
	msg := OpenDropped([]string{csv, exe, txt, csv, sub, png, filepath.Join(dir, "gone.xlsx")},
		func(p string) { opened = append(opened, p) })
	if !slices.Equal(opened, []string{csv, txt}) {
		t.Errorf("opened %q", opened)
	}
	if want := "OfficeLite can't open setup.exe, Folder, photo.png, 1 more"; msg != want {
		t.Errorf("message %q, want %q", msg, want)
	}

	// Too many: the first MaxDrop open.
	var many []string
	for i := range MaxDrop + 5 {
		many = append(many, write(fmt.Sprintf("Book%02d.csv", i), "1\n"))
	}
	opened = nil
	msg = OpenDropped(many, func(p string) { opened = append(opened, p) })
	if len(opened) != MaxDrop || opened[0] != many[0] || !strings.Contains(msg, fmt.Sprintf("first %d of %d", MaxDrop, MaxDrop+5)) {
		t.Errorf("%d opened, message %q", len(opened), msg)
	}
	if msg := OpenDropped(nil, func(string) { t.Error("opened from an empty drop") }); msg != "" {
		t.Errorf("empty drop: %q", msg)
	}
}

// TestFileDropQueue checks that Dropped returns the added paths in order,
// once, and that an empty Add queues nothing.
func TestFileDropQueue(t *testing.T) {
	var f FileDrop
	f.Add()
	f.Add(`C:\a.xlsx`, `C:\b.docx`)
	f.Add(`C:\c.xls`)
	if got := f.Dropped(); !slices.Equal(got, []string{`C:\a.xlsx`, `C:\b.docx`, `C:\c.xls`}) {
		t.Errorf("dropped %q", got)
	}
	if got := f.Dropped(); got != nil {
		t.Errorf("dropped again %q", got)
	}
}

// TestDropPaths checks trimLongPrefix on long, UNC, volume and ordinary
// paths, and dropName on files and folders.
func TestDropPaths(t *testing.T) {
	for in, want := range map[string]string{
		`\\?\C:\very\long\Book.xlsx`:       `C:\very\long\Book.xlsx`,
		`\\?\UNC\server\share\Report.docx`: `\\server\share\Report.docx`,
		`\\?\Volume{1234}\x.xlsx`:          `\\?\Volume{1234}\x.xlsx`,
		`C:\Users\josh\a.docx`:             `C:\Users\josh\a.docx`,
	} {
		if got := trimLongPrefix(in); got != want {
			t.Errorf("trimLongPrefix(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		`C:\Users\josh\Budget.xlsx`: "Budget.xlsx",
		`C:\Users\josh\Folder\`:     "Folder",
		"/home/josh/日本語.docx":       "日本語.docx",
	} {
		if got := dropName(in); got != want {
			t.Errorf("dropName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestFileDropTake: a drop the window isn't ready for is ignored; the
// message waits until the window is done loading.
func TestFileDropTake(t *testing.T) {
	dir := t.TempDir()
	csv, exe := filepath.Join(dir, "a.csv"), filepath.Join(dir, "x.exe")
	for _, p := range []string{csv, exe} {
		if err := os.WriteFile(p, []byte("1"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var f FileDrop
	var opened []string
	open := func(p string) { opened = append(opened, p) }
	busy := false
	f.Add(csv, exe)
	if msg := f.Take(func() bool { return false }, open, func() bool { return busy }); msg != "" || opened != nil {
		t.Errorf("not ready: opened %q, message %q", opened, msg)
	}
	if f.Dropped() != nil {
		t.Error("an ignored drop stays queued")
	}
	f.Add(csv, exe)
	busy = true
	if msg := f.Take(func() bool { return true }, open, func() bool { return busy }); msg != "" || len(opened) != 1 {
		t.Errorf("loading: opened %q, message %q", opened, msg)
	}
	busy = false
	if msg := f.Take(func() bool { return true }, open, func() bool { return busy }); msg != "OfficeLite can't open x.exe" {
		t.Errorf("after loading: message %q", msg)
	}
	if msg := f.Take(func() bool { return true }, open, func() bool { return busy }); msg != "" {
		t.Errorf("message again: %q", msg)
	}
}
