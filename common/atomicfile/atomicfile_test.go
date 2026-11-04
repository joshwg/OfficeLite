// atomicfile_test.go tests WriteFile's and Write's replacement and cleanup.

package atomicfile

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestWriteFile checks that WriteFile creates and replaces a file, leaves no
// temporary file, and fails cleanly when a directory is in the way.
func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "data.json")
	for _, s := range []string{"first", "second"} {
		if err := WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		if b, err := os.ReadFile(p); err != nil || string(b) != s {
			t.Fatalf("read %q, %v; want %q", b, err, s)
		}
	}
	if es, _ := os.ReadDir(filepath.Dir(p)); len(es) != 1 {
		t.Fatalf("left temporary files: %v", es)
	}
	// A directory in the way: an error, and no temporary file left.
	bad := filepath.Join(dir, "dir")
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(bad, []byte("x"), 0o644); err == nil {
		t.Fatal("no error replacing a directory")
	}
	if es, _ := os.ReadDir(dir); len(es) != 2 {
		t.Fatalf("dir holds %v, want sub and dir only", es)
	}
}

// TestWrite checks the streaming Write: content replaces the file, an
// existing file's permissions are kept, a failed fill leaves the old file
// and no temporary file, and a directory is refused.
func TestWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.docx")
	fill := func(s string) func(io.Writer) error {
		return func(w io.Writer) error { _, err := io.WriteString(w, s); return err }
	}
	if err := Write(path, 0o644, fill("one")); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Write(path, 0o644, fill("two")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "two" {
		t.Errorf("content %q", b)
	}
	if fi, _ := os.Stat(path); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want the existing 0600 kept", fi.Mode().Perm())
	}
	boom := errors.New("boom")
	err := Write(path, 0o644, func(w io.Writer) error {
		if _, err := io.WriteString(w, "partial"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "two" {
		t.Errorf("a failed write changed the file to %q", b)
	}
	if des, _ := os.ReadDir(dir); len(des) != 1 {
		t.Errorf("left behind: %v", des)
	}
	if err := Write(dir, 0o644, fill("x")); err == nil {
		t.Error("writing over a directory succeeded")
	}
}
