//go:build !windows

package lockfile

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCheckAge: outside Windows an owner file is live for a day.
func TestCheckAge(t *testing.T) {
	useRegistry(t)
	dir := t.TempDir()
	doc := filepath.Join(dir, "Quarterly.docx")
	theirs := filepath.Join(dir, "~$arterly.docx")
	os.WriteFile(theirs, Encode(Word, "Alice"), 0o644)
	if st := Check(doc); st.Owner != "Alice" || st.InUse || st.Elsewhere {
		t.Errorf("fresh owner file: %+v", st)
	}
	old := time.Now().Add(-25 * time.Hour)
	os.Chtimes(theirs, old, old)
	if st := Check(doc); st.Locked() {
		t.Errorf("day-old owner file: %+v", st)
	}
	if _, err := os.Stat(theirs); err != nil {
		t.Error("another user's stale owner file deleted")
	}
	// This user's own leftover is cleaned up.
	os.WriteFile(theirs, Encode(Word, UserName()), 0o644)
	os.Chtimes(theirs, old, old)
	if st := Check(doc); st.Locked() {
		t.Errorf("own stale owner file: %+v", st)
	}
	if _, err := os.Stat(theirs); !os.IsNotExist(err) {
		t.Error("own stale owner file kept")
	}
	// Excel's name, for a workbook.
	os.WriteFile(filepath.Join(dir, "~$Budget.xlsx"), Encode(Excel, "Bob"), 0o644)
	if st := Check(filepath.Join(dir, "Budget.xlsx")); st.Owner != "Bob" {
		t.Errorf("Excel owner file: %+v", st)
	}
	// Owner files are only written on Windows.
	d := NewDoc(func() {})
	d.Set(doc, true)
	if d.own != nil {
		t.Error("owner file taken")
	}
	d.Close()
}
