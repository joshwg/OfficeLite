package ui

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"officelite/common/autosave"
)

// TestAutoSaveLocked: when another program (Word) holds the file open,
// the AutoSave fails, turns itself off and says why; the edits stay, and
// closing asks about them.
func TestAutoSaveLocked(t *testing.T) {
	h := newHarness(t)
	d := h.d
	path := filepath.Join(t.TempDir(), "Letter.docx")
	clock := h.startAutoSave(path)
	d.setAutoSave(true)
	// Open as Word does: others may read, not write or replace it.
	p, _ := syscall.UTF16PtrFromString(path)
	hf, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ, nil,
		syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(hf)
	h.typeText("Dear")
	clock.Advance(autosave.Delay)
	h.settle()
	if d.as.saver.On() || !strings.Contains(d.errMsg, "Letter.docx is open in another program") {
		t.Errorf("on %v, error %q", d.as.saver.On(), d.errMsg)
	}
	if !d.modified() || h.text() != "Dear" {
		t.Errorf("edits lost: modified %v, %q", d.modified(), h.text())
	}
}
