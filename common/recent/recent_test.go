// recent_test.go tests the recent-files list: ordering, persistence, the
// size limit and sharing one file between processes.

package recent

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"officelite/common/limits"
)

func testAdd(t *testing.T, l *List, path string, kind Kind) {
	t.Helper()
	if err := l.Add(path, kind); err != nil {
		t.Fatal(err)
	}
}

func testRemove(t *testing.T, l *List, path string) {
	t.Helper()
	if err := l.Remove(path); err != nil {
		t.Fatal(err)
	}
}

// TestRemoveRelative checks that Remove matches the absolute path Add
// stored for a relative or uncleaned one, and that an empty path does
// nothing.
func TestRemoveRelative(t *testing.T) {
	l := Open(filepath.Join(t.TempDir(), "recent.json"))
	testAdd(t, l, "docs/a.xlsx", Spreadsheet)
	testAdd(t, l, "b.docx", Document)
	testRemove(t, l, "")
	if len(l.Entries()) != 2 {
		t.Fatalf("Remove of an empty path changed %+v", l.Entries())
	}
	testRemove(t, l, "docs/../docs/a.xlsx")
	if e := l.Entries(); len(e) != 1 || e[0].Name() != "b.docx" {
		t.Fatalf("after removing an uncleaned path: %+v", e)
	}
	testRemove(t, l, "./b.docx")
	if e := l.Entries(); len(e) != 0 {
		t.Errorf("after removing a relative path: %+v", e)
	}
}

// TestList checks that Add moves a file to the top without duplicates, the
// list reloads from its file, Remove drops a file, and the list stays
// within limits.RecentFiles.
func TestList(t *testing.T) {
	store := filepath.Join(t.TempDir(), "sub", "recent.json")
	l := Open(store)
	testAdd(t, l, "/a/one.xlsx", Spreadsheet)
	testAdd(t, l, "/a/two.docx", Document)
	testAdd(t, l, "/a/one.xlsx", Spreadsheet) // moves to the top, no duplicate
	got := l.Entries()
	if len(got) != 2 || got[0].Name() != "one.xlsx" || got[1].Kind != Document {
		t.Fatalf("entries = %+v", got)
	}

	reloaded := Open(store)
	if e := reloaded.Entries(); len(e) != 2 || e[0].Path != got[0].Path {
		t.Fatalf("reloaded = %+v", e)
	}
	testRemove(t, reloaded, "/a/one.xlsx")
	if e := reloaded.Entries(); len(e) != 1 || e[0].Name() != "two.docx" {
		t.Fatalf("after remove = %+v", e)
	}
	for i := 0; i < limits.RecentFiles+5; i++ {
		testAdd(t, l, filepath.Join("/f", string(rune('a'+i%26)), "x.csv"), Spreadsheet)
	}
	if n := len(l.Entries()); n > limits.RecentFiles {
		t.Errorf("len = %d, want <= %d", n, limits.RecentFiles)
	}
}

// TestListShared checks that two lists on one file (two OfficeLite
// processes) keep each other's changes.
func TestListShared(t *testing.T) {
	store := filepath.Join(t.TempDir(), "recent.json")
	a, b := Open(store), Open(store)
	if err := a.Add("/a/one.xlsx", Spreadsheet); err != nil {
		t.Fatal(err)
	}
	if err := b.Add("/b/two.docx", Document); err != nil { // b hasn't seen one.xlsx
		t.Fatal(err)
	}
	if err := a.Add("/a/three.csv", Spreadsheet); err != nil {
		t.Fatal(err)
	}
	names := func(es []Entry) []string {
		var s []string
		for _, e := range es {
			s = append(s, e.Name())
		}
		return s
	}
	want := []string{"three.csv", "two.docx", "one.xlsx"}
	if got := names(Open(store).Entries()); !slices.Equal(got, want) {
		t.Fatalf("file holds %q, want %q", got, want)
	}
	if got := names(a.Entries()); !slices.Equal(got, want) {
		t.Fatalf("a holds %q, want %q", got, want)
	}
	if err := b.Remove("/a/one.xlsx"); err != nil {
		t.Fatal(err)
	}
	want = []string{"three.csv", "two.docx"}
	if got := names(Open(store).Entries()); !slices.Equal(got, want) {
		t.Fatalf("after remove the file holds %q, want %q", got, want)
	}
	if es, _ := os.ReadDir(filepath.Dir(store)); len(es) != 1 {
		t.Errorf("left temporary files: %v", es)
	}

	// A save that fails is reported; the list in memory still changes.
	bad := Open(filepath.Join(store, "recent.json")) // under a file
	if err := bad.Add("/x.csv", Spreadsheet); err == nil {
		t.Error("no error saving under a file")
	}
	if len(bad.Entries()) != 1 {
		t.Error("entry not kept in memory")
	}
}
