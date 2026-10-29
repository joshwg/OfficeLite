// Windows lock tests: network paths, owner-file liveness and share-mode
// locks.

package lockfile

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// TestNetworkPath checks isNetwork on UNC, extended-length and local paths.
func TestNetworkPath(t *testing.T) {
	for path, want := range map[string]bool{
		`\\server\share\a.docx`:        true,
		`\\?\UNC\server\share\a.docx`:  true,
		`\\?\C:\Windows\a.docx`:        false,
		`\\.\C:\Windows\a.docx`:        false,
		os.Getenv("SystemRoot") + `\a`: false,
	} {
		if got := isNetwork(path); got != want {
			t.Errorf("isNetwork(%q) = %v", path, got)
		}
	}
}

// TestOwnerFileLive checks that a held owner file is hidden and live, a
// closed one stale, and that only this user's stale one is deleted.
func TestOwnerFileLive(t *testing.T) {
	useRegistry(t)
	dir := t.TempDir()
	doc := filepath.Join(dir, "Quarterly.docx")
	os.WriteFile(doc, []byte("doc"), 0o644)
	op := filepath.Join(dir, "~$arterly.docx")
	f, err := createOwner(op, Encode(Word, "Alice"))
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(op); err != nil || fi.Sys().(*syscall.Win32FileAttributeData).FileAttributes&fileAttributeHidden == 0 {
		t.Errorf("owner file not hidden: %v", err)
	}
	if !ownerLive(op) {
		t.Error("held owner file not live")
	}
	if st := Check(doc); st.Owner != "Alice" || st.InUse {
		t.Errorf("Check with a live owner file = %+v", st)
	}
	f.Close()
	if ownerLive(op) {
		t.Error("closed owner file live")
	}
	if st := Check(doc); st.Locked() {
		t.Errorf("Check with a stale owner file = %+v", st)
	}
	if _, err := os.Stat(op); err != nil {
		t.Error("another user's stale owner file deleted")
	}
	// This user's own stale one goes.
	os.Remove(op)
	f, _ = createOwner(op, Encode(Word, UserName()))
	f.Close()
	Check(doc)
	if _, err := os.Stat(op); !os.IsNotExist(err) {
		t.Error("own stale owner file kept")
	}
	// Local folders get no owner file.
	d := NewDoc(func() {})
	d.Set(doc, true)
	if d.own != nil {
		t.Error("owner file taken in a local folder")
	}
	d.Close()
}

// TestInUseAndSaveError checks InUse against shared readers and Excel's
// lock, and SaveError's message for a locked file.
func TestInUseAndSaveError(t *testing.T) {
	doc := filepath.Join(t.TempDir(), "Budget.xlsx")
	os.WriteFile(doc, []byte("x"), 0o644)
	if InUse(doc) {
		t.Fatal("closed file in use")
	}
	// A reader that shares everything doesn't count.
	r, _ := createFile(doc, syscall.GENERIC_READ, shareAll, syscall.OPEN_EXISTING, 0)
	if InUse(doc) {
		t.Error("shared reader counts as in use")
	}
	syscall.CloseHandle(r)
	// Excel's lock: open, sharing reading only.
	h, err := createFile(doc, syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ, syscall.OPEN_EXISTING, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !InUse(doc) {
		t.Error("locked file not in use")
	}
	err = os.Rename(doc+".tmp", doc) // what a save would hit
	if msg := SaveError(doc, err); !strings.Contains(msg, "Budget.xlsx is open in another program") {
		t.Errorf("SaveError = %q", msg)
	}
	syscall.CloseHandle(h)
	if InUse(doc) {
		t.Error("released file in use")
	}
}
