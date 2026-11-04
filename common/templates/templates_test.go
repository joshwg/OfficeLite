package templates

import (
	"officelite/common/limits"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

// touch creates an empty file at dir/name.
func touch(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

// names lists the entries' names.
func names(es []Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Name
	}
	return out
}

// TestList checks the filtering and order of a templates folder listing.
func TestList(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"memo.dotx", "Agenda.DOTX", "old.dot", "macro.dotm",
		"report.docx", ".hidden.dotx", "~$memo.dotx", "budget.xltx"} {
		touch(t, dir, n)
	}
	if err := os.Mkdir(filepath.Join(dir, "folder.dotx"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := List(dir, ".dotx", ".dotm", ".dot")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Agenda", "macro", "memo", "old"}; !slices.Equal(names(got), want) {
		t.Fatalf("names = %q, want %q", names(got), want)
	}
	if got[0].Path != filepath.Join(dir, "Agenda.DOTX") {
		t.Errorf("path = %q", got[0].Path)
	}
	if got, err := List(dir, ".xltx"); err != nil || !slices.Equal(names(got), []string{"budget"}) {
		t.Errorf("xltx = %q, %v", names(got), err)
	}
}

// TestListMissing checks that a missing or unnamed folder lists nothing
// without an error, while a file in the folder's place is an error.
func TestListMissing(t *testing.T) {
	dir := t.TempDir()
	if got, err := List(filepath.Join(dir, "nope"), ".dotx"); err != nil || got != nil {
		t.Errorf("missing dir: %v, %v", got, err)
	}
	if got, err := List("", ".dotx"); err != nil || got != nil {
		t.Errorf("empty dir: %v, %v", got, err)
	}
	touch(t, dir, "file")
	if _, err := List(filepath.Join(dir, "file"), ".dotx"); err == nil {
		t.Error("listing a file: no error")
	}
}

// TestDir checks that the templates folder is under the home folder's
// Documents.
func TestDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Documents is a Known Folder on Windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got, want := Dir(), filepath.Join(home, "Documents", FolderName); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

// TestLister checks that a Lister reads in the background, delivers the
// list through Post, and reads again only once its list is stale.
func TestLister(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "a.dotx")
	posted := make(chan func(), 4)
	l := &Lister{Dir: dir, Exts: []string{".dotx"}, Post: func(f func()) { posted <- f }}
	if got := l.Entries(); got != nil {
		t.Fatalf("before the first read: %v", got)
	}
	run := func() {
		t.Helper()
		select {
		case f := <-posted:
			f()
		case <-time.After(5 * time.Second):
			t.Fatal("no read was posted")
		}
	}
	run()
	if got := names(l.Entries()); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("after the read: %q", got)
	}
	// A fresh list isn't read again.
	touch(t, dir, "b.dotx")
	l.Entries()
	select {
	case <-posted:
		t.Fatal("read again while fresh")
	case <-time.After(50 * time.Millisecond):
	}
	// A stale one is, and the old list is shown meanwhile.
	l.read = time.Now().Add(-limits.TemplatesRefreshEvery)
	if got := names(l.Entries()); !slices.Equal(got, []string{"a"}) {
		t.Fatalf("while reading: %q", got)
	}
	l.Entries() // a read is running: no second one
	run()
	if got := names(l.Entries()); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("after the second read: %q", got)
	}
	select {
	case <-posted:
		t.Fatal("two reads ran at once")
	default:
	}
}
