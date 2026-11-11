// Package ui is the WordLite window: title bar, menu bar, formatting
// toolbar, the page view where the document is edited, the status bar and
// the File backstage (New, Open, Info, Save, Save As, Print, Export,
// Close).
package ui

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/x/explorer"

	"officelite/common/autorecover"
	"officelite/common/colors"
	"officelite/common/doctype"
	"officelite/common/gui"
	"officelite/common/lockfile"
	"officelite/common/recent"
	"officelite/common/templates"
	"officelite/word/fileio"
	wl "officelite/word/layout"
	"officelite/word/model"
)

const appName = "WordLite"

var (
	accent     = gui.WordAccent
	accentDark = colors.RGB(0x45289A)
)

// blankTiles are the New page's first tiles, before the user's templates.
var blankTiles = []gui.Template{
	{Name: "Blank document", Kind: recent.Document},
	{Name: "Blank workbook", Kind: recent.Spreadsheet},
}

// templateExts are the template files the New page lists.
var templateExts = []string{".dotx", ".dotm", ".dot"}

// Extensions offered in the Open dialog. A template opened from it is
// edited itself, not used to start a new document.
var openExtensions = []string{".docx", ".docm", ".doc", ".dotx", ".dotm", ".dot", ".txt", ".pages", ".xlsx", ".xls", ".csv", ".numbers"}

// docSeq numbers new documents (Document1, Document2, …).
var docSeq atomic.Int32

// Config configures a document window.
type Config struct {
	Path string // file to open; empty for a blank document
	// Template is a template to start a new untitled document from,
	// instead of Path (the template file itself is not edited).
	Template string
	Fonts    *gui.FontLibrary // shared between windows; nil to create one
	// Launch opens a new window for a file (or a blank one for ""). Nil
	// disables New and Open.
	Launch func(kind doctype.Kind, path string)
	// LaunchTemplate opens a new window with a new document based on the
	// template at path. Nil disables templates on the New page, except in a
	// window holding an untouched new document.
	LaunchTemplate func(path string)
	Recent         *recent.List // nil uses recent.Default()
	// Recovered is a recovery copy to open (Document Recovery), instead
	// of Path.
	Recovered *autorecover.Entry
}

// loadResult is what a loader goroutine hands the UI goroutine through
// d.results.
type loadResult struct {
	path   string // file the document came from; "" if unknown
	doc    *model.Document
	shaper *text.Shaper
	fams   []string
	err    error
	lock   lockfile.Status    // what openPath found about the file (lock.go)
	rec    *autorecover.Entry // the recovery copy loaded (recover.go)
	// template is the template a new document was made from; the
	// document is untitled and named name.
	template, name string
}

// window is a document window's whole state; everything in it belongs to
// the UI goroutine except the atomics and the channels.
type window struct {
	win  *app.Window
	th   *gui.Theme
	expl *explorer.Explorer
	// files is the native Open / Save As dialog, with Word and Excel
	// file-type filters.
	files gui.FileDialog
	drop  gui.FileDrop // files dragged onto the window (drop.go)
	cfg   Config
	gtx   layout.Context // the frame being laid out, for backstage pages

	doc      *model.Document
	ed       *model.Editor
	eng      *wl.Engine
	fams     []string // font families loaded into the engine's shaper
	path     string   // file the document was loaded from or saved to
	fileInfo os.FileInfo
	saved    int          // editor version at the last load or save
	lk       lockState    // the file's registration, owner file, read-only (lock.go)
	ar       autoRecover  // the AutoRecover copy of unsaved changes (recover.go)
	as       gui.AutoSave // AutoSave into the file itself (autosave.go)
	laidOut  int          // editor version the layout reflects (-1: stale)
	title    string
	loading  atomic.Bool
	saving   atomic.Bool
	// fontLoad: what is loading is only a new document's fonts (newBlank),
	// which may be typed into meanwhile.
	fontLoad bool
	errMsg   string
	notice   string

	allowClose     bool // the next close request goes through
	closeAfterSave bool
	confirm        confirmState
	lossy          gui.Dialog // what a save loses (.doc, macros); the macro notice
	lossyGo        func()     // the save waiting for the lossy dialog

	menu    gui.MenuBar
	back    gui.Backstage
	bs      backstageState
	tb      toolbar
	v       docView
	dlg     styleDialog
	ins     insertState // Insert menu dialogs, pictures and links (insert.go)
	results chan loadResult
	calls   *gui.CallQueue

	clip     *model.Fragment // last copied content, for pasting with formatting
	clipText string          // its plain text, to recognize it on the clipboard
	stats    statsCache

	// Home commands (find.go, homecmds.go, hdrftr.go, styles_pane.go).
	find       findBar
	hf         hfEdit
	painter    painterState
	fmtClip    *model.FormatCopy // copied formatting (Format Painter, Ctrl+Shift+C)
	pasteSpec  pasteSpecial
	pastePlain bool // the next clipboard read pastes plain text
	pane       stylesPane
	cm         commentsPane // comments.go
	newStyle   newStyleDialog
	showMarks  bool // Show/Hide ¶ (per window, not saved)
	showCodes  bool // field codes shown in place of results (Alt+F9; per window, not saved)
}

// Run drives a document window until it is closed.
func Run(w *app.Window, cfg Config) error {
	d := newWindow(w, cfg)
	if w != nil {
		w.Option(app.Size(unit.Dp(1100), unit.Dp(800)))
	}
	d.lk.doc = lockfile.NewDoc(gui.Raiser(d.calls, w))
	defer d.lk.doc.Close()
	d.startAutoRecover()
	defer d.ar.saver.Close() // a normal close: the copy goes
	d.startAutoSave()
	defer d.as.Saver.Close()
	if cfg.Recovered != nil {
		d.openRecovered(*cfg.Recovered)
	} else if cfg.Template != "" {
		d.newFromTemplate(cfg.Template)
	} else if cfg.Path != "" {
		d.lk.closeOnCancel = true
		d.setDoc(model.New(filepath.Base(cfg.Path)), d.th.Shaper, nil)
		d.openPath(cfg.Path)
	} else {
		d.newBlank()
	}

	var ops op.Ops
	for {
		e := w.Event()
		d.expl.ListenEvents(e)
		d.files.ListenEvents(e)
		d.drop.Event(e)
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
			d.calls.Run()
			d.takeResult()
			gtx := app.NewContext(&ops, e)
			d.layout(gtx)
			gui.KeepKeyboard(gtx) // keys keep coming in the browser
			e.Frame(gtx.Ops)
		}
	}
}

// newWindow makes a window's state for w (nil in tests), filling in
// cfg's defaults.
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
	if cfg.LaunchTemplate == nil {
		cfg.LaunchTemplate = func(string) {}
	}
	d := &window{
		win:     w,
		th:      gui.NewTheme(cfg.Fonts, accent, accentDark),
		cfg:     cfg,
		results: make(chan loadResult, 1),
	}
	d.calls = gui.NewCallQueue(d.invalidate)
	d.bs.tmpl = templates.Lister{Dir: templates.Dir(), Exts: templateExts, Post: d.calls.Post}
	if w != nil {
		d.expl = explorer.NewExplorer(w)
		d.drop.Window = w
	}
	d.v.zoom = 1
	d.v.vbar.Vertical = true
	return d
}

// newBlank starts a new document; its fonts load in the background. The
// families are listed here, on the UI goroutine: the user may type into
// doc while the loader runs.
func (d *window) newBlank() {
	doc := model.New(newDocName())
	d.setDoc(doc, d.th.Shaper, nil)
	d.loading.Store(true)
	d.fontLoad = true
	fams := DocFonts(doc)
	go func() {
		sh := d.cfg.Fonts.Shaper(fams...)
		d.calls.Post(func() {
			d.loading.Store(false)
			d.fontLoad = false
			if d.doc == doc {
				picked := d.fams // fonts chosen while these loaded (ensureFont)
				d.fams = fams
				d.eng.SetShaper(sh)
				d.laidOut = -1
				for _, f := range picked {
					d.ensureFont(f)
				}
			}
		})
	}()
}

// newDocName numbers the next new document: Document1, Document2, …
func newDocName() string {
	return "Document" + strconv.Itoa(int(docSeq.Add(1)))
}

// invalidate asks for a frame, when there is a window.
func (d *window) invalidate() {
	if d.win != nil {
		d.win.Invalidate()
	}
}

// DocFonts lists the font families a document uses, for loading (the window
// and wordshot both load these).
func DocFonts(doc *model.Document) []string {
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

// openPath loads the file at path on a goroutine, checking its lock
// first; takeResult shows it when done.
func (d *window) openPath(path string) {
	if !d.startLoad() {
		d.errMsg = busyLoading
		return
	}
	d.lk.doc.Set(path, false) // opening it again raises this window
	go func() {
		st := lockfile.Check(path)
		doc, err := fileio.Open(path)
		if err == nil && !st.Locked() {
			d.lk.doc.Set(path, true) // an owner file, on a network share
		}
		d.finishLoadLock(path, doc, err, st)
	}()
}

// busyLoading is the message for a load refused because another is under
// way.
const busyLoading = "Another file is still opening in this window."

// startLoad marks a load into the window (one whose result goes through
// d.results) as begun, on the UI goroutine. It refuses (false) while one
// is under way, so a second loader can't block on the results channel.
func (d *window) startLoad() bool { return d.loading.CompareAndSwap(false, true) }

// openDialog asks for a file and opens it like a recent file (openFile).
// Where the platform gives no path, the data is read into this window if
// it is pristine (openReader).
func (d *window) openDialog() {
	if d.expl == nil {
		return
	}
	gui.ChooseOpen{
		Files: &d.files,
		// The native dialog lists Word documents, then Excel files (which
		// openFile routes to an ExcelLite window).
		Filters:  []gui.FileFilter{gui.WordFilter, gui.ExcelFilter, gui.AllFilter},
		Choose:   d.expl.ChooseFile,
		Exts:     openExtensions,
		Declined: explorer.ErrUserDecline,
		Calls:    d.calls,
		Open:     d.openFile,
		Read:     d.openReader,
		Fail:     func(err error) { d.errMsg = "Can't open the file: " + err.Error() },
	}.Start()
}

// openReader reads a file the Open dialog gave without a path into the
// window, which then has no file of its own (Save goes to Save As). Only
// a pristine window takes it; otherwise it is closed unread and the
// status line says why.
func (d *window) openReader(rc io.ReadCloser) {
	d.back.Hide()
	if !d.pristine() || !d.startLoad() {
		gui.CloseChosen(rc)
		d.errMsg = gui.OpenHereOnlyEmpty
		return
	}
	d.lk.doc.Set("", false) // no file of its own to register
	go func() {
		data, err := io.ReadAll(rc)
		if closeErr := rc.Close(); err == nil {
			err = closeErr
		}
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
	d.finishLoadLock(path, doc, err, lockfile.Status{})
}

// finishLoadLock is finishLoad for a file lockfile.Check found st about.
func (d *window) finishLoadLock(path string, doc *model.Document, err error, st lockfile.Status) {
	d.finishLoadResult(loadResult{path: path, doc: doc, err: err, lock: st})
}

// finishLoadResult hands a loaded document to the UI goroutine.
func (d *window) finishLoadResult(res loadResult) {
	if doc := res.doc; doc != nil {
		res.fams = DocFonts(doc)
		res.shaper = d.cfg.Fonts.Shaper(res.fams...)
	}
	d.results <- res
	d.invalidate()
}

// takeResult applies a finished load, if any: it shows the error, asks
// about a locked file, or shows the document.
func (d *window) takeResult() {
	select {
	case r := <-d.results:
		d.loading.Store(false)
		if r.err != nil {
			file := r.path
			if r.template != "" {
				file = r.template
			}
			d.errMsg = "Can't open " + gui.OrNone(filepath.Base(file), "the file") + ": " + r.err.Error()
			d.lk.doc.Set(d.path, !d.lk.readOnly) // back to the file shown
			return
		}
		if r.lock.Locked() {
			d.askLocked(r) // lock.go
			return
		}
		d.applyLoad(r, false)
	default:
	}
}

// applyLoad shows a loaded document, read-only or not.
func (d *window) applyLoad(r loadResult, readOnly bool) {
	d.errMsg = ""
	d.path, d.fileInfo = r.path, nil
	d.lk.readOnly, d.lk.closeOnCancel = readOnly, false
	if r.path != "" {
		d.fileInfo, _ = os.Stat(r.path)
		if err := d.cfg.Recent.Add(r.path, recent.Document); err != nil {
			log.Printf("add %s to recent documents: %v", r.path, err)
		}
	}
	if r.template != "" {
		r.doc.Name = r.name
	}
	d.setDoc(r.doc, r.shaper, r.fams)
	if r.template != "" {
		d.adoptTemplate(r.template)
	}
	if r.rec != nil {
		d.adoptRecovered(*r.rec) // recover.go
	}
	d.macroNotice(r.template != "")               // macros.go
	d.as.Elsewhere = r.lock.Locked() && !readOnly // Open Anyway
	d.autoSaveLoaded()                            // autosave.go
}

// setDoc makes doc the window's document.
func (d *window) setDoc(doc *model.Document, shaper *text.Shaper, fams []string) {
	d.doc = doc
	d.ed = model.NewEditor(doc)
	d.eng = wl.New(doc, shaper)
	d.eng.SetShowHidden(d.showMarks)
	d.eng.SetShowFieldCodes(d.showCodes)
	d.fams = fams
	d.saved = d.ed.Version
	d.laidOut = -1
	d.notice = ""
	d.stats = statsCache{}
	d.hf = hfEdit{}
	d.resetComments()
	d.find.matches, d.find.key = nil, findKey{}
	d.painter = painterState{}
	d.bs.info = infoCache{}
	d.v.scrollY, d.v.scrollX = 0, 0
	d.v.focusPending = true
	d.updateTitle()
}

// modified reports whether the document has unsaved changes.
func (d *window) modified() bool { return d.ed != nil && d.bodyEd().Version != d.saved }

// updateTitle sets the OS window title when the name or modified state
// changes.
func (d *window) updateTitle() {
	t := d.docName() + " - " + appName
	if d.modified() {
		t = "• " + t
	}
	if t != d.title && d.win != nil {
		d.win.Option(app.Title(t))
	}
	d.title = t
}

// opening reports whether a file, template or recovery copy is loading
// into the window: until it arrives the document shown is a placeholder
// that it replaces, so editing input and commands are ignored (the window
// still scrolls, zooms, shows its menus and closes). A new document's
// fonts loading (newBlank) don't count: it can be typed into at once.
func (d *window) opening() bool { return d.loading.Load() && !d.fontLoad }

// pristine reports whether the window holds an untouched new document,
// which an opened file replaces. Not while a Save As or export runs: its
// result would land on the document that replaced this one.
func (d *window) pristine() bool {
	return d.path == "" && !d.modified() && !d.loading.Load() && !d.saving.Load()
}

// openFile opens path in this window if it is pristine, otherwise in a new
// window; spreadsheets go to an ExcelLite window (gui.OpenFile).
func (d *window) openFile(path string) {
	d.back.Hide()
	msg := gui.OpenFile(gui.OpenRequest{
		Path:     path,
		Recent:   d.cfg.Recent,
		Own:      doctype.Document,
		Takes:    func(k doctype.Kind) bool { return k == doctype.Document },
		Pristine: d.pristine,
		OpenHere: d.openPath,
		Launch:   d.cfg.Launch,
	})
	if msg != "" {
		d.errMsg = msg
	}
}
