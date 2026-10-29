package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

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
