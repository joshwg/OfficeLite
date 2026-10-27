// Package ui is the WordLite window: title bar, menu bar, formatting
// toolbar, the page view where the document is edited, the status bar and
// the File backstage (New, Open, Info, Save, Save As, Print, Export,
// Close).
package ui

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/x/explorer"

	"officelite/common/colors"
	"officelite/common/doctype"
	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/word/fileio"
	wl "officelite/word/layout"
	"officelite/word/model"
)

const appName = "WordLite"

var (
	accent     = gui.WordAccent
	accentDark = colors.RGB(0x10418A)
)

var templates = []gui.Template{
	{Name: "Blank document", Kind: recent.Document},
	{Name: "Blank workbook", Kind: recent.Spreadsheet},
}

// Extensions offered in the Open dialog.
var openExtensions = []string{".docx", ".docm", ".doc", ".txt", ".xlsx", ".xls", ".csv"}

// docSeq numbers new documents (Document1, Document2, ...).
var docSeq atomic.Int32

// Config configures a document window.
type Config struct {
	Path  string           // file to open; empty for a blank document
	Fonts *gui.FontLibrary // shared between windows; nil to create one
	// Launch opens a new window for a file (or a blank one for ""). Nil
	// disables New and Open.
	Launch func(kind doctype.Kind, path string)
	Recent *recent.List // nil uses recent.Default()
}

type loadResult struct {
	path   string // file the document came from; "" if unknown
	doc    *model.Document
	shaper *text.Shaper
	fams   []string
	err    error
}

type window struct {
	win  *app.Window
	th   *gui.Theme
	expl *explorer.Explorer
	cfg  Config
	gtx  layout.Context // the frame being laid out, for backstage pages

	doc      *model.Document
	ed       *model.Editor
	eng      *wl.Engine
	fams     []string // font families loaded into the engine's shaper
	path     string   // file the document was loaded from or saved to
	fileInfo os.FileInfo
	saved    int // editor version at the last load or save
	laidOut  int // editor version the layout reflects (-1: stale)
	title    string
	loading  atomic.Bool
	saving   atomic.Bool
	errMsg   string
	notice   string

	allowClose     bool // the next close request goes through
	closeAfterSave bool
	confirm        confirmState
	lossy          gui.Dialog // what a .doc save loses
	lossyGo        func()     // the save waiting for the lossy dialog

	menu    gui.MenuBar
	back    gui.Backstage
	bs      backstageState
	tb      toolbar
	v       docView
	dlg     styleDialog
	results chan loadResult
	calls   chan func()

	clip     *model.Fragment // last copied content, for pasting with formatting
	clipText string          // its plain text, to recognise it on the clipboard
	stats    statsCache
}

// Run drives a document window until it is closed.
func Run(w *app.Window, cfg Config) error {
	d := newWindow(w, cfg)
	if w != nil {
		w.Option(app.Size(unit.Dp(1100), unit.Dp(800)))
	}
	if cfg.Path != "" {
		d.setDoc(model.New(filepath.Base(cfg.Path)), d.th.Shaper, nil)
		d.openPath(cfg.Path)
	} else {
		d.newBlank()
	}

	var ops op.Ops
	for {
		e := w.Event()
		d.expl.ListenEvents(e)
		switch e := e.(type) {
		case app.DestroyEvent:
			return e.Err
		case *app.ClosingEvent:
			if d.modified() && !d.allowClose {
				e.Abort()
				d.back.Hide()
				d.confirm.show = true
				w.Invalidate()
			}
		case app.FrameEvent:
			d.runCalls()
			d.takeResult()
			gtx := app.NewContext(&ops, e)
			d.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func newWindow(w *app.Window, cfg Config) *window {
	if cfg.Fonts == nil {
		cfg.Fonts = gui.NewFontLibrary(gui.DefaultFontDirs()...)
	}
	if cfg.Recent == nil {
		cfg.Recent = recent.Default()
	}
	if cfg.Launch == nil {
		cfg.Launch = func(doctype.Kind, string) {}
	}
	d := &window{
		win:     w,
		th:      gui.NewTheme(cfg.Fonts, accent, accentDark),
		cfg:     cfg,
		results: make(chan loadResult, 1),
		calls:   make(chan func(), 16),
	}
	if w != nil {
		d.expl = explorer.NewExplorer(w)
	}
	d.v.zoom = 1
	d.v.vbar.Vertical = true
	return d
}

// newBlank starts a new document; its fonts load in the background.
func (d *window) newBlank() {
	doc := model.New("Document" + itoa(int(docSeq.Add(1))))
	d.setDoc(doc, d.th.Shaper, nil)
	d.loading.Store(true)
	go func() {
		fams := docFonts(doc)
		sh := d.cfg.Fonts.Shaper(fams...)
		d.post(func() {
			d.loading.Store(false)
			if d.doc == doc {
				d.fams = fams
				d.eng.SetShaper(sh)
				d.laidOut = -1
			}
		})
	}()
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// post runs f on the UI goroutine at the next frame.
func (d *window) post(f func()) {
	d.calls <- f
	d.invalidate()
}

func (d *window) invalidate() {
	if d.win != nil {
		d.win.Invalidate()
	}
}

func (d *window) runCalls() {
	for {
		select {
		case f := <-d.calls:
			f()
		default:
			return
		}
	}
}

// docFonts lists the font families a document uses, for loading.
func docFonts(doc *model.Document) []string {
	fams := []string{"Calibri", "Calibri Light", "Aptos", "Times New Roman", "Arial", "Symbol"}
	add := func(f string) {
		if f != "" && !slices.Contains(fams, f) {
			fams = append(fams, f)
		}
	}
	add(doc.Styles.DefaultRun.Font)
	for _, st := range doc.Styles.Styles {
		add(st.Run.Font)
	}
	for _, l := range doc.Lists {
		for _, lv := range l.Levels {
			add(lv.Font)
		}
	}
	doc.Paragraphs(func(p *model.Paragraph) bool {
		add(p.Mark.Font)
		for i := range p.Runs {
			add(p.Runs[i].Props.Font)
		}
		return true
	})
	return fams
}

// ensureFont loads a family into the document's shaper if needed.
func (d *window) ensureFont(name string) {
	if name == "" || slices.Contains(d.fams, name) {
		return
	}
	d.fams = append(d.fams, name)
	d.eng.SetShaper(d.cfg.Fonts.Shaper(d.fams...))
	d.laidOut = -1
}

func (d *window) openPath(path string) {
	d.loading.Store(true)
	go func() {
		doc, err := fileio.Open(path)
		d.finishLoad(path, doc, err)
	}()
}

// openDialog asks for a file and opens it like a recent file. Where the
// platform gives no path, the data is read into this window.
func (d *window) openDialog() {
	if d.expl == nil {
		return
	}
	go func() {
		rc, err := d.expl.ChooseFile(openExtensions...)
		if errors.Is(err, explorer.ErrUserDecline) {
			return
		}
		if err != nil {
			d.finishLoad("", nil, err)
			return
		}
		defer rc.Close()
		if f, ok := rc.(*os.File); ok && filepath.IsAbs(f.Name()) {
			path := f.Name()
			d.post(func() { d.openFile(path) })
			return
		}
		d.loading.Store(true)
		d.invalidate()
		data, err := io.ReadAll(rc)
		if err != nil {
			d.finishLoad("", nil, err)
			return
		}
		doc, err := fileio.Read("Document.docx", data)
		d.finishLoad("", doc, err)
	}()
}

// finishLoad runs on the loader goroutine; fonts are loaded here so the
// UI thread never blocks on disk.
func (d *window) finishLoad(path string, doc *model.Document, err error) {
	res := loadResult{path: path, doc: doc, err: err}
	if doc != nil {
		res.fams = docFonts(doc)
		res.shaper = d.cfg.Fonts.Shaper(res.fams...)
	}
	d.results <- res
	d.invalidate()
}

func (d *window) takeResult() {
	select {
	case r := <-d.results:
		d.loading.Store(false)
		if r.err != nil {
			d.errMsg = "Can't open " + orNone(filepath.Base(r.path), "the file") + ": " + r.err.Error()
			return
		}
		d.errMsg = ""
		d.path, d.fileInfo = r.path, nil
		if r.path != "" {
			d.fileInfo, _ = os.Stat(r.path)
			d.cfg.Recent.Add(r.path, recent.Document)
		}
		d.setDoc(r.doc, r.shaper, r.fams)
	default:
	}
}

// setDoc makes doc the window's document.
func (d *window) setDoc(doc *model.Document, shaper *text.Shaper, fams []string) {
	d.doc = doc
	d.ed = model.NewEditor(doc)
	d.eng = wl.New(doc, shaper)
	d.fams = fams
	d.saved = d.ed.Version
	d.laidOut = -1
	d.notice = ""
	d.stats = statsCache{}
	d.bs.info = infoCache{}
	d.v.scrollY, d.v.scrollX = 0, 0
	d.v.focusPending = true
	d.updateTitle()
}

func (d *window) modified() bool { return d.ed != nil && d.ed.Version != d.saved }

// updateTitle sets the OS window title when the name or modified state
// changes.
func (d *window) updateTitle() {
	t := d.doc.Name + " - " + appName
	if d.modified() {
		t = "• " + t
	}
	if t != d.title && d.win != nil {
		d.win.Option(app.Title(t))
	}
	d.title = t
}

// pristine reports whether the window holds an untouched new document,
// which an opened file replaces.
func (d *window) pristine() bool {
	return d.path == "" && !d.modified() && !d.loading.Load()
}

// openFile opens path in this window if it is pristine, otherwise in a new
// window; spreadsheets go to an ExcelLite window.
func (d *window) openFile(path string) {
	d.back.Hide()
	if _, err := os.Stat(path); err != nil {
		d.cfg.Recent.Remove(path)
		d.errMsg = "Can't open " + filepath.Base(path) + ": " + err.Error()
		return
	}
	kind, _ := doctype.DetectFile(path)
	switch {
	case kind != doctype.Document:
		d.cfg.Launch(kind, path)
	case d.pristine():
		d.openPath(path)
	default:
		d.cfg.Launch(doctype.Document, path)
	}
}

func orNone(s, none string) string {
	if s == "" || s == "." {
		return none
	}
	return s
}
