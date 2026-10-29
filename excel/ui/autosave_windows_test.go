// Windows-only AutoSave test: a save into a file another program holds open.

package ui

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"gioui.org/io/key"

	"officelite/common/autosave"
)

// TestAutoSaveLocked: when another program (Excel) holds the file open,
// the AutoSave fails, turns itself off and says why; the edits stay, and
// closing asks about them.
func TestAutoSaveLocked(t *testing.T) {
	h := newHarness(t)
	a := h.a
	path := filepath.Join(t.TempDir(), "Budget.xlsx")
	clock := h.startAutoSave(path)
	a.setAutoSave(true)
	// Open as Excel does: others may read, not write or replace it.
	p, _ := syscall.UTF16PtrFromString(path)
	hf, err := syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ, nil,
		syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(hf)
	h.typeText("42")
	h.press(key.NameReturn, 0)
	clock.Advance(autosave.Delay)
	h.settle()
	if a.as.saver.On() || !strings.Contains(a.errMsg, "Budget.xlsx is open in another program") {
		t.Errorf("on %v, error %q", a.as.saver.On(), a.errMsg)
	}
	if !a.modified || h.cell("A1").Text != "42" || !a.askBeforeClose() {
		t.Errorf("edits lost: modified %v, A1 %q", a.modified, h.cell("A1").Text)
	}
}
