package gui

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"officelite/common/autorecover"
)

func TestRecoverySaveName(t *testing.T) {
	dir := filepath.Join("home", "j")
	for _, c := range []struct {
		e     autorecover.Entry
		name  string
		first string
	}{
		{autorecover.Entry{Path: filepath.Join(dir, "Budget.xls"), Name: "Budget.xls", App: autorecover.Excel, Format: ".xls"},
			filepath.Join(dir, "Budget.xls"), ".xls"},
		{autorecover.Entry{Path: filepath.Join(dir, "Sales.numbers"), Name: "Sales.numbers", App: autorecover.Excel, Format: ".numbers"},
			filepath.Join(dir, "Sales.xlsx"), ".xlsx"},
		{autorecover.Entry{Name: "Document3", App: autorecover.Word}, "Document3.docx", ".docx"},
		{autorecover.Entry{Path: filepath.Join(dir, "Notes.TXT"), Name: "Notes.TXT", App: autorecover.Word, Format: ".TXT"},
			filepath.Join(dir, "Notes.txt"), ".txt"},
	} {
		name, filters := RecoverySaveName(c.e)
		if name != c.name || filters[0].Exts[0] != c.first || filters[len(filters)-1].Name != AllFilter.Name {
			t.Errorf("%s: %q, first filter %v", c.e.Name, name, filters[0].Exts)
		}
	}
}

// fakeEntry writes a recovery copy and its sidecar.
func fakeEntry(t *testing.T, dir, base string, e autorecover.Entry) autorecover.Entry {
	e.Copy = filepath.Join(dir, base+autorecover.Ext(e.App))
	e.Sidecar = filepath.Join(dir, base+".json")
	os.WriteFile(e.Copy, []byte("copy"), 0o600)
	if err := autorecover.Release(e); err != nil { // writes the sidecar
		t.Fatal(err)
	}
	return e
}

func TestRecoveryWindow(t *testing.T) {
	dir := t.TempDir()
	a := fakeEntry(t, dir, "Budget~1", autorecover.Entry{Path: "/x/Budget.xls", Name: "Budget.xls", App: autorecover.Excel, Saved: time.Now()})
	b := fakeEntry(t, dir, "Document1~2", autorecover.Entry{Name: "Document1", App: autorecover.Word, Saved: time.Now()})
	c := fakeEntry(t, dir, "Notes~3", autorecover.Entry{Name: "Notes.docx", App: autorecover.Word, Saved: time.Now()})
	var opened []autorecover.Entry
	closed := 0
	r := NewRecovery(RecoveryConfig{Fonts: NewFontLibrary(), Entries: []autorecover.Entry{a, b, c},
		Open:  func(e autorecover.Entry) { opened = append(opened, e) },
		Close: func() { closed++ }})
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(640, 520)), Source: router.Source()}
		r.Run()
		r.Layout(gtx)
		router.Frame(gtx.Ops)
	}
	frame()

	r.Open(0)
	if len(opened) != 1 || opened[0].Name != "Budget.xls" || len(r.Entries()) != 2 {
		t.Fatalf("Open: opened %v, left %v", opened, r.Entries())
	}
	if _, err := os.Stat(a.Copy); err != nil {
		t.Error("Open deleted the copy (the window takes it over)")
	}
	r.Discard(0)
	if _, err := os.Stat(b.Copy); err == nil || len(r.Entries()) != 1 || closed != 0 {
		t.Errorf("Discard: copy left (%v), entries %v, closed %d", err, r.Entries(), closed)
	}
	frame()
	// Close keeps the rest, let go for the next start.
	r.Close()
	r.Release()
	if closed != 1 {
		t.Errorf("closed %d", closed)
	}
	if o := autorecover.Orphans(dir); len(o) != 2 { // Budget (no window adopted it here) and Notes
		t.Errorf("after Close: %+v", o)
	}
	// The last one dealt with closes the window.
	r = NewRecovery(RecoveryConfig{Fonts: NewFontLibrary(), Entries: []autorecover.Entry{c}, Close: func() { closed++ }})
	r.Discard(0)
	if closed != 2 {
		t.Error("window not closed when empty")
	}
}
