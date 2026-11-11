// Tests for the shared AutoSave rules and AutoRecover glue: why AutoSave
// is blocked, an AutoSave's end, the recovery origin, Save As names and
// format choice.

package gui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"officelite/common/autorecover"
)

// TestAutoSaveBlocked checks each reason, in order, and the file kinds
// AutoSave works with.
func TestAutoSaveBlocked(t *testing.T) {
	f := AutoSaveFile{Noun: "workbook", Ext: ".xlsx", Path: "/x/B.XLSX"}
	if got := AutoSaveBlocked(f); got != "" {
		t.Errorf("a saved .xlsx: %q", got)
	}
	for _, c := range []struct {
		set  func(*AutoSaveFile)
		want string
	}{
		{func(f *AutoSaveFile) { f.Loading, f.Recovered = true, true }, "Opening the workbook…"},
		{func(f *AutoSaveFile) { f.Recovered, f.ReadOnly = true, true }, "recovered workbook"},
		{func(f *AutoSaveFile) { f.ReadOnly = true }, "is read-only"},
		{func(f *AutoSaveFile) { f.Elsewhere = true }, "Someone else"},
		{func(f *AutoSaveFile) { f.Path = "/x/B.xls" }, AutoSaveWorksWith(".xlsx")},
		{func(f *AutoSaveFile) { f.Path = "" }, AutoSaveWorksWith(".xlsx")},
	} {
		g := f
		c.set(&g)
		if got := AutoSaveBlocked(g); !strings.Contains(got, c.want) {
			t.Errorf("%+v: %q, want %q", g, got, c.want)
		}
	}
}

// TestAutoSaveEnded checks an error's message, a save of a file the
// window no longer has, and a save that counts.
func TestAutoSaveEnded(t *testing.T) {
	s := AutoSave{Path: "/x/a.docx"}
	if msg, ok := s.Ended(errors.New("disk full"), "/x/a.docx"); ok || !strings.HasPrefix(msg, "AutoSave turned off: ") || !s.SavedAt.IsZero() {
		t.Errorf("error: %q %v", msg, ok)
	}
	if msg, ok := s.Ended(nil, "/x/b.docx"); ok || msg != "" {
		t.Errorf("saved as another file: %q %v", msg, ok)
	}
	if msg, ok := s.Ended(nil, "/x/a.docx"); !ok || msg != "" || s.SavedAt.IsZero() {
		t.Errorf("saved: %q %v %v", msg, ok, s.SavedAt)
	}
}

// TestRecoveryGlue checks RecoveryOrigin, SaveAsName and FormatIndex for
// a document with a file and one opened from a recovery copy.
func TestRecoveryGlue(t *testing.T) {
	rec := &autorecover.Entry{Path: filepath.Join("/docs", "Old.doc"), Format: ".doc"}
	if p, f := RecoveryOrigin("/x/A.DOCX", nil); p != "/x/A.DOCX" || f != ".docx" {
		t.Errorf("own file: %q %q", p, f)
	}
	if p, f := RecoveryOrigin("", rec); p != rec.Path || f != ".doc" {
		t.Errorf("recovered: %q %q", p, f)
	}
	if got := SaveAsName("Old", ".docx", nil); got != "Old.docx" {
		t.Errorf("SaveAsName: %q", got)
	}
	if got := SaveAsName("Old", ".docx", rec); got != filepath.Join("/docs", "Old.docx") {
		t.Errorf("SaveAsName recovered: %q", got)
	}
	formats := []string{"docx", "doc", "txt"}
	if FormatIndex(formats, "doc", true) != 1 || FormatIndex(formats, "doc", false) != 0 || FormatIndex(formats, "pages", true) != 0 {
		t.Error("FormatIndex")
	}
}
