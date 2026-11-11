// Tests of the document store: names found without regard to case,
// replacing, the quota, renames, deletes and removing a user's folder.

package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	docs, used, err := s.List("ANN")
	if err != nil || len(docs) != 2 || used != 7 || docs[0].Name != "a.docx" {
		t.Fatalf("List = %+v, %d, %v", docs, used, err)
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
	if docs, _, _ := s.List("ann"); len(docs) != 1 {
		t.Fatalf("List = %+v", docs)
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
	if _, err := s.Rename("ann", "a.docx", "B.DOCX"); !errors.Is(err, ErrExists) {
		t.Fatalf("rename onto another: %v", err)
	}
	if info, err := s.Rename("ann", "a.docx", "A.docx"); err != nil || info.Name != "A.docx" {
		t.Fatalf("case-only rename = %+v, %v", info, err)
	}
	if _, err := s.Rename("ann", "a.docx", "c.docx"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := s.Delete("ann", "C.docx"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete("ann", "c.docx"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete again: %v", err)
	}
	if docs, _, _ := s.List("ann"); len(docs) != 1 || docs[0].Name != "b.docx" {
		t.Fatalf("List = %+v", docs)
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
