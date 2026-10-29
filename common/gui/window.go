// Window helpers shared by the apps: the UI call queue, Info page and
// drop-down formatting, PDF export and printing through a PDF viewer.

package gui

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// CallQueue hands work from other goroutines (file dialogs, loading,
// saving) to the UI goroutine, which runs it at the next frame. The queue
// is unbounded, so Post never blocks, even on the UI goroutine itself.
type CallQueue struct {
	mu    sync.Mutex
	calls []func()
	wake  func() // asks for a frame; nil in tests
}

// NewCallQueue makes a queue that calls wake (the window's Invalidate)
// when work is posted.
func NewCallQueue(wake func()) *CallQueue {
	return &CallQueue{wake: wake}
}

// Post queues f for the UI goroutine.
func (q *CallQueue) Post(f func()) {
	q.mu.Lock()
	q.calls = append(q.calls, f)
	q.mu.Unlock()
	if q.wake != nil {
		q.wake()
	}
}

// Run runs the queued calls, including any they post, in order; the UI
// goroutine calls it every frame.
func (q *CallQueue) Run() {
	for {
		q.mu.Lock()
		calls := q.calls
		q.calls = nil
		q.mu.Unlock()
		if len(calls) == 0 {
			return
		}
		for _, f := range calls {
			f()
		}
	}
}

// ByteSize is a file size for the Info page: "512 bytes", "1.5 KB".
func ByteSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d bytes", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

// OrNone is s, or none when s is empty (or the "." filepath gives for no
// path).
func OrNone(s, none string) string {
	if s == "" || s == "." {
		return none
	}
	return s
}

// FormatPt is a font size or spacing in points for a drop-down: "11",
// "10.5". Rounding to hundredths hides float32 noise (10.1 stored as
// 10.100000381) and covers Office's twentieths of a point.
func FormatPt(v float64) string { return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64) }

// MenuItems turns labels into drop-down items; an empty label is a
// separator.
func MenuItems(labels []string) []DropItem {
	items := make([]DropItem, len(labels))
	for i, l := range labels {
		if l == "" {
			items[i] = DropItem{Separator: true}
		} else {
			items[i] = DropItem{Label: l}
		}
	}
	return items
}

// PDFSave is a PDF export for SavePDF.
type PDFSave struct {
	Name  string // the document's name, without ".pdf"
	Files *FileDialog
	// Create is the Save dialog used when there is no native one
	// (explorer's CreateFile); Declined is its error for a canceled
	// dialog.
	Create   func(name string) (io.WriteCloser, error)
	Declined error
	Calls    *CallQueue
	Busy     *atomic.Bool // set until the export is done
	// Build renders the PDF; it runs on the UI goroutine, so the document
	// can't change meanwhile.
	Build func() (b []byte, pages int, err error)
	Open  bool // open the PDF in the default viewer afterwards
	// Report gets the status notice or the error message on the UI
	// goroutine; it isn't called when the user cancels.
	Report func(notice, errMsg string)
}

// SavePDF asks for a file name and writes the PDF there.
func SavePDF(j PDFSave) {
	j.Busy.Store(true)
	name := j.Name + ".pdf"
	go func() {
		path, err := j.Files.Save(name, PDFFilter, AllFilter)
		switch {
		case err == nil:
			j.Calls.Post(func() {
				defer j.Busy.Store(false)
				b, pages, err := j.Build()
				if err == nil {
					err = os.WriteFile(path, b, 0o644)
				}
				j.Report(pdfDone(path, pages, err, j.Open))
			})
			return
		case errors.Is(err, ErrCancelled):
			j.Busy.Store(false)
			return
		}
		// No native dialog: the fallback Save dialog.
		wc, err := j.Create(name)
		j.Calls.Post(func() {
			defer j.Busy.Store(false)
			if errors.Is(err, j.Declined) {
				return
			}
			var pages int
			path := name
			if err == nil {
				var b []byte
				b, pages, err = j.Build()
				if err == nil {
					_, err = wc.Write(b)
				}
				if cerr := wc.Close(); err == nil {
					err = cerr
				}
				if f, ok := wc.(*os.File); ok {
					path = f.Name()
				}
			}
			j.Report(pdfDone(path, pages, err, j.Open && filepath.IsAbs(path)))
		})
	}()
}

// pdfDone is the outcome of an export, as a notice or an error message.
func pdfDone(path string, pages int, err error, open bool) (notice, errMsg string) {
	if err != nil {
		return "", "PDF export failed: " + err.Error()
	}
	notice = fmt.Sprintf("Exported %s (%d page(s))", filepath.Base(path), pages)
	if open {
		if err := OpenWithDefault(path); err != nil {
			notice += "; can't open it: " + err.Error()
		}
	}
	return notice, ""
}

// PrintViaPDF writes the PDF of a document called name to a temporary
// file and opens it in the default PDF viewer, to be printed from there.
// It returns the status notice, or an error message.
func PrintViaPDF(name string, build func() ([]byte, int, error)) (notice, errMsg string) {
	b, pages, err := build()
	path := ""
	if err == nil {
		dir := filepath.Join(os.TempDir(), "OfficeLite")
		err = os.MkdirAll(dir, 0o755)
		// A new name each time: the viewer may still hold the last one.
		path = filepath.Join(dir, fmt.Sprintf("%s (print %s).pdf", name, time.Now().Format("150405")))
		if err == nil {
			err = os.WriteFile(path, b, 0o644)
		}
	}
	if err != nil {
		return "", "Print failed: " + err.Error()
	}
	if err := OpenWithDefault(path); err != nil {
		return "", "Can't open the PDF viewer: " + err.Error() + " (the PDF is " + path + ")"
	}
	return fmt.Sprintf("Opened %d page(s) in your PDF viewer; print from there", pages), ""
}
