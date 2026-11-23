// Tests of the document store: names found without regard to case,
// replacing, the quota, renames, deletes, folders (making, listing,
// moving, deleting, the depth and count limits) and removing a user's
// folder.

package files

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"officelite/web"
)

// put stores content as name for user ann within quota, failing the test
// on error.
func put(t *testing.T, s *Store, name, content string, quota int64, replace bool) Info {
	t.Helper()
	info, err := s.Put("Ann", name, strings.NewReader(content), quota, replace)
	if err != nil {
		t.Fatalf("Put(%q): %v", name, err)
	}
	return info
}

// TestPutListOpen checks the round trip and the folder's place and name.
func TestPutListOpen(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	info := put(t, s, "  Budget 2026.xlsx ", "hello", 100, false)
	if info.Name != "Budget 2026.xlsx" || info.Size != 5 {
		t.Fatalf("Put = %+v", info)
	}
	if _, err := os.Stat(filepath.Join(root, "ann", "Budget 2026.xlsx")); err != nil {
		t.Fatalf("not stored under the lowercase username: %v", err)
	}
	put(t, s, "a.docx", "xy", 100, false)
	l, err := s.List("ANN", "")
	if err != nil || len(l.Files) != 2 || l.Used != 7 || l.Files[0].Name != "a.docx" {
		t.Fatalf("List = %+v, %v", l, err)
	}
	f, d, err := s.Open("ann", "BUDGET 2026.XLSX")
	if err != nil || d.Name != "Budget 2026.xlsx" {
		t.Fatalf("Open = %+v, %v", d, err)
	}
	b := make([]byte, 10)
	n, _ := f.Read(b)
	_ = f.Close()
	if string(b[:n]) != "hello" {
		t.Fatalf("read %q", b[:n])
	}
	if _, _, err := s.Open("ann", "missing.docx"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open missing: %v", err)
	}
}

// TestPutReplace checks that a name in another case is the same file.
func TestPutReplace(t *testing.T) {
	s := New(t.TempDir())
	put(t, s, "Budget.xlsx", "one", 100, false)
	if _, err := s.Put("ann", "budget.XLSX", strings.NewReader("two"), 100, false); !errors.Is(err, ErrExists) {
		t.Fatalf("Put without replace: %v", err)
	}
	info := put(t, s, "budget.XLSX", "three", 100, true)
	if info.Name != "Budget.xlsx" || info.Size != 5 {
		t.Fatalf("replace = %+v", info)
	}
	if l, _ := s.List("ann", ""); len(l.Files) != 1 {
		t.Fatalf("List = %+v", l)
	}
}

// TestQuota checks that a write past the quota fails and changes
// nothing, and that replacing a file counts only the new size.
func TestQuota(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	put(t, s, "a.docx", "12345", 10, false)
	if _, err := s.Put("ann", "b.docx", strings.NewReader("123456"), 10, false); !errors.Is(err, ErrQuota) {
		t.Fatalf("over quota: %v", err)
	}
	put(t, s, "b.docx", "12345", 10, false) // exactly full
	put(t, s, "a.docx", "1234", 10, true)   // replacing frees its old size
	if _, err := s.Put("ann", "a.docx", strings.NewReader("123456"), 10, true); !errors.Is(err, ErrQuota) {
		t.Fatalf("replace over quota: %v", err)
	}
	if d, _ := s.Stat("ann", "a.docx"); d.Size != 4 {
		t.Fatalf("failed write changed the file: %+v", d)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "ann"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tempPrefix) {
			t.Fatalf("temporary file left: %s", e.Name())
		}
	}
}

// TestBadNames checks that rule-breaking names are refused as *NameError.
func TestBadNames(t *testing.T) {
	s := New(t.TempDir())
	var ne *NameError
	if _, err := s.Put("ann", "../x.docx", strings.NewReader(""), 10, false); !errors.As(err, &ne) {
		t.Fatalf("bad file name: %v", err)
	}
	if _, err := s.Put("../ann", "x.docx", strings.NewReader(""), 10, false); !errors.As(err, &ne) {
		t.Fatalf("bad username: %v", err)
	}
	if _, err := s.Put("ann", "x.exe", strings.NewReader(""), 10, false); !errors.As(err, &ne) {
		t.Fatalf("bad extension: %v", err)
	}
}

// TestRenameDelete checks renames (with a case-only one) and deletes.
func TestRenameDelete(t *testing.T) {
	s := New(t.TempDir())
	put(t, s, "a.docx", "1", 100, false)
	put(t, s, "b.docx", "2", 100, false)
	if _, err := s.Move("ann", "a.docx", "B.DOCX"); !errors.Is(err, ErrExists) {
		t.Fatalf("rename onto another: %v", err)
	}
	if got, err := s.Move("ann", "a.docx", "A.docx"); err != nil || got != "A.docx" {
		t.Fatalf("case-only rename = %q, %v", got, err)
	}
	if _, err := s.Move("ann", "a.docx", "c.docx"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := s.Delete("ann", "C.docx"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete("ann", "c.docx"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete again: %v", err)
	}
	if l, _ := s.List("ann", ""); len(l.Files) != 1 || l.Files[0].Name != "b.docx" {
		t.Fatalf("List = %+v", l)
	}
}

// TestRemoveUser checks that a user's folder goes with them.
func TestRemoveUser(t *testing.T) {
	s := New(t.TempDir())
	if has, err := s.HasFolder("ann"); has || err != nil {
		t.Fatalf("HasFolder before = %v, %v", has, err)
	}
	put(t, s, "a.docx", "1", 100, false)
	if has, _ := s.HasFolder("ANN"); !has {
		t.Fatal("HasFolder after Put = false")
	}
	if err := s.RemoveUser("Ann"); err != nil {
		t.Fatal(err)
	}
	if has, _ := s.HasFolder("ann"); has {
		t.Fatal("folder left after RemoveUser")
	}
}

// mkdir makes folder path for user ann, failing the test on error.
func mkdir(t *testing.T, s *Store, path string) Folder {
	t.Helper()
	f, err := s.MakeFolder("ann", path)
	if err != nil {
		t.Fatalf("MakeFolder(%q): %v", path, err)
	}
	return f
}

// TestFolders checks making, listing (folders first, case kept) and
// using folders: documents in them, found in any case, counted in the
// quota, and real folders on disk.
func TestFolders(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	if f := mkdir(t, s, " Reports "); f.Name != "Reports" || f.Path != "Reports" {
		t.Fatalf("MakeFolder = %+v", f)
	}
	mkdir(t, s, "reports/2026")
	if _, err := s.MakeFolder("ann", "REPORTS"); !errors.Is(err, ErrExists) {
		t.Fatalf("same folder in another case: %v", err)
	}
	if _, err := s.MakeFolder("ann", "Missing/x"); !errors.Is(err, ErrNoFolder) {
		t.Fatalf("folder in a missing one: %v", err)
	}
	var ne *NameError
	for _, bad := range []string{"", ".hidden", "Reports/.x", "Old.docx", "a/b/c/d/e"} {
		if _, err := s.MakeFolder("ann", bad); !errors.As(err, &ne) {
			t.Errorf("MakeFolder(%q): %v", bad, err)
		}
	}
	info := put(t, s, "REPORTS/2026/Budget.xlsx", "12345", 100, false)
	if info.Path != "Reports/2026/Budget.xlsx" {
		t.Fatalf("Put path = %q", info.Path)
	}
	if _, err := os.Stat(filepath.Join(root, "ann", "Reports", "2026", "Budget.xlsx")); err != nil {
		t.Fatalf("not a real folder on disk: %v", err)
	}
	if _, err := s.Put("ann", "Nowhere/x.docx", strings.NewReader("x"), 100, false); !errors.Is(err, ErrNoFolder) {
		t.Fatalf("Put in a missing folder: %v", err)
	}
	put(t, s, "top.docx", "12", 100, false)
	l, err := s.List("ann", "")
	if err != nil || len(l.Folders) != 1 || l.Folders[0].Name != "Reports" || len(l.Files) != 1 || l.Used != 7 {
		t.Fatalf("List top = %+v, %v", l, err)
	}
	l, err = s.List("ann", "reports/2026")
	if err != nil || l.Dir != "Reports/2026" || len(l.Files) != 1 || l.Files[0].Path != "Reports/2026/Budget.xlsx" {
		t.Fatalf("List 2026 = %+v, %v", l, err)
	}
	if _, err := s.List("ann", "Missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("List missing: %v", err)
	}
	if _, err := s.Put("ann", "Reports/b.docx", strings.NewReader("1234"), 10, false); !errors.Is(err, ErrQuota) {
		t.Fatalf("quota counts every folder: %v", err)
	}
	if d, err := s.Stat("ann", "reports/2026/BUDGET.xlsx"); err != nil || d.Size != 5 {
		t.Fatalf("Stat = %+v, %v", d, err)
	}
	if all, err := s.AllFolders("ann"); err != nil || strings.Join(all, "|") != "Reports|Reports/2026" {
		t.Fatalf("AllFolders = %q, %v", all, err)
	}
}

// TestFolderDepth checks web.FolderMaxDepth for new folders, documents
// and moved folders.
func TestFolderDepth(t *testing.T) {
	s := New(t.TempDir())
	path := ""
	for i := 1; i <= web.FolderMaxDepth; i++ {
		path = strings.TrimPrefix(path+"/"+fmt.Sprint("L", i), "/")
		mkdir(t, s, path)
	}
	put(t, s, path+"/deep.docx", "x", 100, false)
	var ne *NameError
	if _, err := s.MakeFolder("ann", path+"/more"); !errors.As(err, &ne) {
		t.Fatalf("folder past the depth: %v", err)
	}
	mkdir(t, s, "Other")
	// L1 holds folders FolderMaxDepth-1 deep, so it can't go into Other.
	if _, err := s.Move("ann", "L1", "Other/L1"); !errors.Is(err, ErrTooDeep) {
		t.Fatalf("move past the depth: %v", err)
	}
	if got, err := s.Move("ann", "Other", "L1/Other"); err != nil || got != "L1/Other" {
		t.Fatalf("move an empty folder down = %q, %v", got, err)
	}
}

// TestFolderMoveDelete checks moving documents and folders between
// folders, renaming folders, refusing a move into itself, and deleting
// folders with and without what they hold.
func TestFolderMoveDelete(t *testing.T) {
	s := New(t.TempDir())
	mkdir(t, s, "A")
	mkdir(t, s, "A/B")
	mkdir(t, s, "C")
	put(t, s, "x.docx", "1", 100, false)
	if got, err := s.Move("ann", "X.docx", "a/b/x.docx"); err != nil || got != "A/B/x.docx" {
		t.Fatalf("move a document down = %q, %v", got, err)
	}
	put(t, s, "x.docx", "2", 100, false)
	if _, err := s.Move("ann", "x.docx", "A/B/X.DOCX"); !errors.Is(err, ErrExists) {
		t.Fatalf("move onto a document: %v", err)
	}
	if _, err := s.Move("ann", "A", "A/B/A"); !errors.Is(err, ErrIntoItself) {
		t.Fatalf("move into itself: %v", err)
	}
	if _, err := s.Move("ann", "Gone.docx", "A/Gone.docx"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("move of a missing document: %v", err)
	}
	if _, err := s.Move("ann", "x.docx", "Nowhere/x.docx"); !errors.Is(err, ErrNoFolder) {
		t.Fatalf("move into a missing folder: %v", err)
	}
	if _, err := s.Move("ann", "A", "c"); !errors.Is(err, ErrExists) {
		t.Fatalf("rename onto a folder: %v", err)
	}
	if got, err := s.Move("ann", "a", "a"); err != nil || got != "a" {
		t.Fatalf("case-only folder rename = %q, %v", got, err)
	}
	if got, err := s.Move("ann", "A", "C/Moved"); err != nil || got != "C/Moved" {
		t.Fatalf("move a folder = %q, %v", got, err)
	}
	if d, err := s.Stat("ann", "C/Moved/B/x.docx"); err != nil || d.Size != 1 {
		t.Fatalf("document after its folder moved = %+v, %v", d, err)
	}
	var ne *NameError
	if _, err := s.Move("ann", "C", "C.docx"); !errors.As(err, &ne) {
		t.Fatalf("folder renamed to a file name: %v", err)
	}
	if _, err := s.Move("ann", "x.docx", "Folder"); !errors.As(err, &ne) {
		t.Fatalf("file renamed to a folder name: %v", err)
	}
	if err := s.DeleteFolder("ann", "C", false); !errors.Is(err, ErrNotEmpty) {
		t.Fatalf("delete a full folder: %v", err)
	}
	mkdir(t, s, "Empty")
	if err := s.DeleteFolder("ann", "empty", false); err != nil {
		t.Fatalf("delete an empty folder: %v", err)
	}
	if err := s.DeleteFolder("ann", "C", true); err != nil {
		t.Fatalf("delete a folder and all in it: %v", err)
	}
	if l, _ := s.List("ann", ""); len(l.Folders) != 0 || l.Used != 1 {
		t.Fatalf("List after deletes = %+v", l)
	}
	if err := s.DeleteFolder("ann", "", true); !errors.As(err, &ne) {
		t.Fatalf("delete the top folder: %v", err)
	}
}
