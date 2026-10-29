package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/io/key"

	"officelite/common/gui"
	"officelite/common/lockfile"
)

// TestLockedOpen: a workbook another user has open asks first; Open
// Read-Only marks the window and sends Save to Save As; Esc cancels.
func TestLockedOpen(t *testing.T) {
	h := newHarness(t)
	a := h.a
	a.results = make(chan loadResult, 1)
	path := filepath.Join(t.TempDir(), "Budget.xlsx")
	wb := emptyWorkbook()
	wb.Name = "Budget.xlsx"
	a.results <- loadResult{path: path, wb: wb, shaper: a.shaper, lock: lockfile.Status{Owner: "Alice"}}
	a.takeResult()
	h.frame()
	if !a.dialog.Visible || !strings.Contains(a.dialog.Message, "locked for editing by Alice") || a.path != "" {
		t.Fatalf("dialog %v %q, path %q", a.dialog.Visible, a.dialog.Message, a.path)
	}
	a.dialog.Hide()
	a.answer(0) // Open Read-Only
	if a.path != path || !a.lk.readOnly || a.docName() != "Budget.xlsx (Read-Only)" {
		t.Fatalf("read-only: path %q, %v, %q", a.path, a.lk.readOnly, a.docName())
	}
	a.modified = true
	a.save()
	if !a.back.Visible || a.back.Page != gui.PageSaveAs {
		t.Errorf("Save in read-only mode: backstage %v page %v", a.back.Visible, a.back.Page)
	}
	a.back.Hide()

	// Another locked file: Esc cancels and the window keeps its workbook.
	other := emptyWorkbook()
	a.results <- loadResult{path: filepath.Join(filepath.Dir(path), "Other.xlsx"), wb: other, shaper: a.shaper,
		lock: lockfile.Status{InUse: true}}
	a.takeResult()
	h.frame()
	if !strings.Contains(a.dialog.Message, "in use by another program") {
		t.Fatalf("dialog %q", a.dialog.Message)
	}
	h.press(key.NameEscape, 0)
	if a.dialog.Visible || a.wb == other || a.path != path {
		t.Errorf("after Esc: dialog %v, path %q", a.dialog.Visible, a.path)
	}
}
