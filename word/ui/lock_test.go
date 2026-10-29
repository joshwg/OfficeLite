package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"gioui.org/io/key"

	"officelite/common/gui"
	"officelite/common/lockfile"
	"officelite/word/model"
)

// TestLockedOpen: a document Word has open asks first; Open Read-Only
// marks the window and sends Save to Save As; Esc cancels.
func TestLockedOpen(t *testing.T) {
	h := newHarness(t)
	d := h.d
	path := filepath.Join(t.TempDir(), "Report.docx")
	doc := model.New("Report.docx")
	d.results <- loadResult{path: path, doc: doc, shaper: d.eng.Shaper(), lock: lockfile.Status{InUse: true}}
	d.takeResult()
	h.frame()
	if !d.lk.dlg.Visible || !strings.Contains(d.lk.dlg.Message, "in use by another program") || d.path != "" {
		t.Fatalf("dialog %v %q, path %q", d.lk.dlg.Visible, d.lk.dlg.Message, d.path)
	}
	d.lk.dlg.Hide()
	d.lockAnswer(0) // Open Read-Only
	h.frame()
	if d.path != path || !d.lk.readOnly || d.title != "Report.docx (Read-Only) - WordLite" {
		t.Fatalf("read-only: path %q, %v, title %q", d.path, d.lk.readOnly, d.title)
	}
	d.save()
	if !d.back.Visible || d.back.Page != gui.PageSaveAs {
		t.Errorf("Save in read-only mode: backstage %v page %v", d.back.Visible, d.back.Page)
	}
	d.back.Hide()

	other := model.New("Other.docx")
	d.results <- loadResult{path: filepath.Join(filepath.Dir(path), "Other.docx"), doc: other, shaper: d.eng.Shaper(),
		lock: lockfile.Status{Owner: "Alice"}}
	d.takeResult()
	h.frame()
	if !strings.Contains(d.lk.dlg.Message, "locked for editing by Alice") {
		t.Fatalf("dialog %q", d.lk.dlg.Message)
	}
	h.press(key.NameEscape, 0)
	if d.lk.dlg.Visible || d.doc == other || d.path != path {
		t.Errorf("after Esc: dialog %v, path %q", d.lk.dlg.Visible, d.path)
	}
}
