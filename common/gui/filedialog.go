package gui

import (
	"errors"
	"strings"
	"sync/atomic"

	"gioui.org/io/event"
)

// FileFilter is one entry of an Open dialog's file-type list.
type FileFilter struct {
	Name string   // e.g. "Word Documents"
	Exts []string // e.g. ".docx", ".doc"; empty matches all files
}

// Pattern is the filter's wildcard list, e.g. "*.docx;*.doc".
func (f FileFilter) Pattern() string {
	if len(f.Exts) == 0 {
		return "*.*"
	}
	p := make([]string, len(f.Exts))
	for i, e := range f.Exts {
		p[i] = "*" + e
	}
	return strings.Join(p, ";")
}

// Label is the text shown in the file-type list, e.g.
// "Word Documents (*.docx;*.doc)".
func (f FileFilter) Label() string { return f.Name + " (" + f.Pattern() + ")" }

// File-type filters shared by the spreadsheet and document windows.
var (
	ExcelFilter = FileFilter{Name: "Excel Files", Exts: []string{".xlsx", ".xlsm", ".xls", ".csv", ".tsv", ".txt", ".numbers"}}
	WordFilter  = FileFilter{Name: "Word Documents", Exts: []string{".docx", ".docm", ".dotx", ".doc", ".txt", ".pages"}}
	AllFilter   = FileFilter{Name: "All Files"}
	PDFFilter   = FileFilter{Name: "PDF", Exts: []string{".pdf"}}
	ImageFilter = FileFilter{Name: "Pictures", Exts: []string{".png", ".jpg", ".jpeg", ".jfif", ".gif", ".bmp", ".tif", ".tiff", ".webp"}}
)

// ErrCancelled is returned when the user closes a file dialog without
// choosing a file.
var ErrCancelled = errors.New("file dialog cancelled")

// ErrNoFileDialog is returned where the platform has no native dialog
// with file-type filters; callers fall back to gioui.org/x/explorer.
var ErrNoFileDialog = errors.New("no native file dialog")

// FileDialog shows the platform's Open dialog with a file-type list, which
// gioui.org/x/explorer can't (it offers a single filter).
type FileDialog struct {
	owner atomic.Uintptr // native window handle, for a modal dialog
}

// ListenEvents records the window handle; call it with every window event.
func (d *FileDialog) ListenEvents(e event.Event) { d.listen(e) }

// Open shows the Open dialog with the given filters (the first one
// selected) and returns the chosen path. It blocks, so call it from a
// goroutine.
func (d *FileDialog) Open(filters ...FileFilter) (string, error) { return d.open(filters) }

// Save shows the Save As dialog with the given filters, suggesting name,
// and returns the chosen path, with the first filter's extension added
// when the user typed none. It asks before replacing an existing file.
// It blocks, so call it from a goroutine.
func (d *FileDialog) Save(name string, filters ...FileFilter) (string, error) {
	return d.save(name, filters)
}
