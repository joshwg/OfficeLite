// save_test.go checks SaveFile: a failed save leaves the old file as it
// was and no temporary file behind, and a save keeps the file's mode.

package fileio

import (
	"os"
	"path/filepath"
	"testing"

	"officelite/word/model"
)

// TestSaveFileFailureKeepsOld checks that a save that fails part way
// (here an unknown format) leaves the target untouched and cleans up.
func TestSaveFileFailureKeepsOld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Notes.txt")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	doc := model.New("Notes.txt")
	if err := SaveFile(doc, path, Format(99)); err == nil {
		t.Fatal("no error for an unknown format")
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != "old" {
		t.Errorf("file now %q, %v; want old", b, err)
	}
	if es, _ := os.ReadDir(dir); len(es) != 1 {
		t.Errorf("left %v", es)
	}
	if err := SaveFile(doc, path, TXT); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode after save %v, %v; want 0600", fi.Mode().Perm(), err)
	}
}
