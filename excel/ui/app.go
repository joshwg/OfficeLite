// Package ui is the ExcelLite window: title bar, menu bar, formula bar,
// grid, sheet tabs and status bar, and the File backstage (New, Open,
// Info, Save, Save As, Print, Export, Close).
package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gioui.org/x/explorer"

	"officelite/common/autorecover"
	"officelite/common/doctype"
	"officelite/common/gui"
	"officelite/common/lockfile"
	"officelite/common/recent"
	"officelite/excel/calc"
	"officelite/excel/edit"
	"officelite/excel/fileio"
	"officelite/excel/formula/functions"
	"officelite/excel/model"
	"officelite/excel/render"
)

const appName = "ExcelLite"

// Extensions offered in the Open dialog.
var openExtensions = []string{".xlsx", ".xlsm", ".xls", ".csv", ".tsv", ".txt", ".numbers"}

type sheetState struct {
	view   render.View
	selEnd render.Pos // moving corner when extending the selection
	extent extent     // cells with content, for the scrollbars (scroll.go)
	// circles are the cells Circle Invalid Data marks (validation.go);
	// nil when it is off.
	circles [][2]int
}

type loadResult struct {
	path   string // file the workbook came from; "" if unknown
	wb     *model.Workbook
	shaper *text.Shaper
	err    error
	lock   lockfile.Status    // what openPath found about the file (lock.go)
	rec    *autorecover.Entry // the recovery copy loaded (recover.go)
}

type App struct {
	win    *app.Window
	ui     *gui.Theme
	th     *material.Theme // ui.Theme
	expl   *explorer.Explorer
	files  gui.FileDialog // Open dialog with Excel/Word file-type filters
	drop   gui.FileDrop   // files dragged onto the window (drop.go)
	fonts  *gui.FontLibrary
	recent *recent.List
	launch func(kind doctype.Kind, path string)
	gtx    layout.Context // the frame being laid out, for backstage pages

	wb       *model.Workbook
	path     string      // file the workbook was loaded from or saved to
	fileInfo os.FileInfo // of path, when loaded
	modified bool        // unsaved changes
	lk       lockState   // the file's registration, owner file, read-only (lock.go)
	ar       autoRecover // the AutoRecover copy of unsaved changes (recover.go)
	as       autoSave    // AutoSave into the file itself (autosave.go)
	shaper   *text.Shaper
	active   int // index into wb.Sheets
	states   map[int]*sheetState
	zoom     float32
	loading  atomic.Bool // set by loader goroutines
	saving   atomic.Bool // a Save As or export is writing
	errMsg   string
	notice   string // transient status message (e.g. after F9)

	menu                           gui.MenuBar
	back                           gui.Backstage
	bs                             backstageState
	zoomInBtn, zoomOutBtn, zoomBtn widget.Clickable
	tabBtns                        []widget.Clickable
	newSheetBtn                    widget.Clickable // the + after the sheet tabs
	tabList                        layout.List
	vbar                           gui.Scrollbar
	hbar                           gui.Scrollbar

	grid     gridInput
	book     *edit.Book // the workbook's editing state: undo, recalculation
	ed       cellEditor
	clip     *edit.Clip // cells copied or cut in this window
	clipText string     // the text put on the clipboard with clip
	rename   renameState
	dialog   gui.Dialog
	// closing is set while the window waits for the answer to "Save
	// changes?" or for the save that answer started, to close afterwards.
	closing  bool
	mayClose bool      // the user chose Don't Save
	warned   string    // path whose format-loss warning was accepted
	answer   func(int) // handles the dialog's button

	results chan loadResult
	calls   *gui.CallQueue // work posted to the UI goroutine
	stats   statsCache

	home   homeRibbon   // the Home ribbon (ribbon.go)
	data   dataRibbon   // the Data ribbon (data.go)
	tab    int          // the ribbon showing: tabHome or tabData
	filt   filterMenu   // an AutoFilter drop-down (filter.go)
	vlist  validList    // a data validation list's drop-down (validation.go)
	fmtDlg formatDialog // Format Cells (formatdialog.go)
	askDlg askDialog    // Row Height, Column Width, ... prompts
	ins    insertState  // Insert, Find & Select and Sort dialogs, fill handle (dialogs.go)
	cm     ctxMenus     // right-click menus (ctxmenu.go)
	pv     pivotUI      // PivotTables: field list, drop-downs, dialogs (pivot.go)
	ch     chartUI      // charts: dragging, title editing, ribbons, dialogs (chart.go)
	// fmtRange is the range a formatting command is working on: each of
	// a multiple selection's in turn (format.go).
	fmtRange model.Range
}

// bookSeq numbers new workbooks (Book1, Book2, ...) across windows.
var bookSeq atomic.Int32

// Config configures a spreadsheet window.
type Config struct {
	Path  string           // file to open; empty for a blank workbook
	Fonts *gui.FontLibrary // shared between windows; nil to create one
	// Launch opens a new window for a file (or a blank one for ""). Nil
	// opens spreadsheet windows in this process without tracking them.
	Launch func(kind doctype.Kind, path string)
	Recent *recent.List // nil uses recent.Default()
	// Recovered is a recovery copy to open (Document Recovery), instead
	// of Path.
	Recovered *autorecover.Entry
}

// Run drives a spreadsheet window until it is closed.
func Run(w *app.Window, cfg Config) error {
	if cfg.Fonts == nil {
		cfg.Fonts = gui.NewFontLibrary(gui.DefaultFontDirs()...)
	}
	if cfg.Recent == nil {
		cfg.Recent = recent.Default()
	}
	if cfg.Launch == nil {
		fonts := cfg.Fonts
		cfg.Launch = func(_ doctype.Kind, path string) {
			go Run(new(app.Window), Config{Path: path, Fonts: fonts})
		}
	}
	w.Option(app.Title(appName), app.Size(unit.Dp(1280), unit.Dp(800)))
	a := &App{
		win:     w,
		ui:      gui.NewTheme(cfg.Fonts, accent, accentDark),
		expl:    explorer.NewExplorer(w),
		fonts:   cfg.Fonts,
		recent:  cfg.Recent,
		launch:  cfg.Launch,
		zoom:    1,
		results: make(chan loadResult, 1),
		calls:   gui.NewCallQueue(w.Invalidate),
		states:  map[int]*sheetState{},
	}
	a.th = a.ui.Theme
	a.drop.Window = w
	a.vbar.Vertical = true
	a.shaper = a.th.Shaper
	a.lk.doc = lockfile.NewDoc(a.raise)
	defer a.lk.doc.Close()
	a.startAutoRecover()
	defer a.ar.saver.Close() // a normal close: the copy goes
	a.startAutoSave()
	defer a.as.saver.Close()
	if cfg.Recovered != nil {
		a.openRecovered(*cfg.Recovered)
	} else if cfg.Path != "" {
		a.lk.closeOnCancel = true
		a.setWorkbook(&model.Workbook{
			Name:   filepath.Base(cfg.Path),
			Sheets: []*model.Sheet{model.NewSheet("Sheet1")},
			Styles: []model.Style{model.DefaultStyle()},
		})
		a.openPath(cfg.Path)
	} else {
		a.setWorkbook(emptyWorkbook())
	}

	var ops op.Ops
	for {
		e := w.Event()
		a.expl.ListenEvents(e)
		a.files.ListenEvents(e)
		a.drop.Event(e)
		switch e := e.(type) {
		case *app.ClosingEvent:
			// Gio lets a window refuse to close (WM_CLOSE on Windows):
			// ask about unsaved changes first.
			if a.askBeforeClose() {
				e.Abort()
				a.askSaveBeforeClose()
				w.Invalidate()
			}
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			a.calls.Run()
			a.takeResult()
			gtx := app.NewContext(&ops, e)
			a.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// emptyWorkbook returns a new one-sheet workbook named BookN.
func emptyWorkbook() *model.Workbook {
	return &model.Workbook{
		Name:   fmt.Sprintf("Book%d", bookSeq.Add(1)),
		Sheets: []*model.Sheet{model.NewSheet("Sheet1")},
		Styles: []model.Style{model.DefaultStyle()},
	}
}

// openPath loads the file at path on a goroutine, checking its lock
// first; takeResult shows it when done.
func (a *App) openPath(path string) {
	a.loading.Store(true)
	a.lk.doc.Set(path, false) // opening it again raises this window
	go func() {
		st := lockfile.Check(path)
		wb, err := fileio.Open(path)
		if err == nil && !st.Locked() {
			a.lk.doc.Set(path, true) // an owner file, on a network share
		}
		a.finishLoadLock(path, wb, err, st)
	}()
}

// openDialog asks for a file and opens it like a recent file. Where the
// platform gives no path, the data is read into this window.
func (a *App) openDialog() {
	go func() {
		// The native dialog lists Excel files, then Word documents (which
		// openFile routes to a WordLite window).
		path, err := a.files.Open(gui.ExcelFilter, gui.WordFilter, gui.AllFilter)
		switch {
		case err == nil:
			a.calls.Post(func() { a.openFile(path) })
			return
		case errors.Is(err, gui.ErrCancelled):
			return
		}
		rc, err := a.expl.ChooseFile(openExtensions...)
		if errors.Is(err, explorer.ErrUserDecline) {
			return
		}
		if err != nil {
			a.finishLoad("", nil, err)
			return
		}
		defer rc.Close()
		if f, ok := rc.(*os.File); ok && filepath.IsAbs(f.Name()) {
			path := f.Name()
			a.calls.Post(func() { a.openFile(path) })
			return
		}
		a.loading.Store(true)
		a.win.Invalidate()
		data, err := io.ReadAll(rc)
		if err != nil {
			a.finishLoad("", nil, err)
			return
		}
		wb, err := fileio.Read("Workbook", data)
		a.finishLoad("", wb, err)
	}()
}

// finishLoad runs on the loader goroutine. Fonts for the workbook are
// resolved here so the UI thread never blocks on disk.
func (a *App) finishLoad(path string, wb *model.Workbook, err error) {
	a.finishLoadLock(path, wb, err, lockfile.Status{})
}

// finishLoadLock is finishLoad for a file lockfile.Check found st about.
func (a *App) finishLoadLock(path string, wb *model.Workbook, err error, st lockfile.Status) {
	a.finishLoadResult(loadResult{path: path, wb: wb, err: err, lock: st})
}

// finishLoadResult hands a loaded workbook to the UI goroutine.
func (a *App) finishLoadResult(res loadResult) {
	if wb := res.wb; wb != nil {
		// Show stored results as Excel does, calculating only formulas
		// that have none (CSV formulas, files from non-calculating tools).
		r := calc.Recalculate(wb, functions.Default(), calc.Missing, time.Now())
		wb.Warnings = append(wb.Warnings, circularWarning(r)...)
		fams := []string{"Segoe UI", "Tahoma"}
		for _, st := range wb.Styles {
			if !slices.Contains(fams, st.Font.Family) {
				fams = append(fams, st.Font.Family)
			}
		}
		res.shaper = a.fonts.Shaper(fams...)
	}
	a.results <- res
	if a.win != nil {
		a.win.Invalidate()
	}
}

// circularWarning returns a warning naming up to three of r's circular
// references, or nil when it has none.
func circularWarning(r calc.Result) []string {
	if len(r.Circular) == 0 {
		return nil
	}
	return []string{"Circular reference: " + strings.Join(r.Circular[:min(len(r.Circular), 3)], ", ")}
}

// recalculate recalculates every formula (F9).
func (a *App) recalculate() {
	if a.saving.Load() {
		return // the workbook is being written
	}
	if a.ed.active && !a.commitEdit(0, 0) {
		return
	}
	start := time.Now()
	r := a.book.Recalculate()
	a.stats = statsCache{}
	a.notice = fmt.Sprintf("Calculated %d formulas in %v", r.Calculated, time.Since(start).Round(time.Millisecond))
	if w := circularWarning(r); w != nil {
		a.notice = w[0]
	}
}

// takeResult applies a finished load, if any: it shows the error, asks
// about a locked file, or shows the workbook.
func (a *App) takeResult() {
	select {
	case r := <-a.results:
		a.loading.Store(false)
		if r.err != nil {
			a.errMsg = r.err.Error()
			a.lk.doc.Set(a.path, !a.lk.readOnly) // back to the file shown
			return
		}
		if r.lock.Locked() {
			a.askLocked(r) // lock.go
			return
		}
		a.applyLoad(r, false)
	default:
	}
}

// applyLoad shows a loaded workbook, read-only or not.
func (a *App) applyLoad(r loadResult, readOnly bool) {
	a.errMsg = ""
	a.shaper = r.shaper
	a.th.Shaper = r.shaper
	a.path, a.fileInfo, a.modified = r.path, nil, false
	a.lk.readOnly, a.lk.closeOnCancel = readOnly, false
	if r.path != "" {
		a.fileInfo, _ = os.Stat(r.path)
		a.recent.Add(r.path, recent.Spreadsheet)
	}
	a.setWorkbook(r.wb)
	if r.rec != nil {
		a.adoptRecovered(*r.rec) // recover.go
	}
	a.as.elsewhere = r.lock.Locked() && !readOnly // Open Anyway
	a.autoSaveLoaded()                            // autosave.go
}

// setWorkbook makes wb the window's workbook, resetting the editing
// state, sheet views, ribbons and dialogs and activating the first
// visible sheet.
func (a *App) setWorkbook(wb *model.Workbook) {
	a.wb = wb
	if a.book != nil {
		a.book.Close()
	}
	a.book = edit.New(wb)
	a.ed.active = false
	a.rename.active = false
	a.clip = nil
	a.cm.grid.Close()
	a.cm.tabs.Close()
	a.grid.move, a.grid.pointing = nil, false
	a.states = map[int]*sheetState{}
	a.tabBtns = make([]widget.Clickable, len(wb.Sheets))
	a.active = 0
	for i, sh := range wb.Sheets {
		if !sh.Hidden {
			a.active = i
			break
		}
	}
	a.stats = statsCache{}
	a.notice = ""
	a.bs.info = infoCache{}
	a.home = homeRibbon{}
	a.data = dataRibbon{}
	a.filt = filterMenu{}
	a.vlist = validList{}
	a.pv = pivotUI{}
	a.ch = chartUI{}
	a.fmtDlg.open, a.askDlg.open = false, false
	a.ins = insertState{}
	if a.win != nil {
		a.win.Option(windowTitle(a.docName()))
	}
	a.grid.focusPending = true
}

// state returns the view state of the active sheet, creating it on first use.
func (a *App) state() *sheetState {
	st, ok := a.states[a.active]
	if !ok {
		sh := a.wb.Sheets[a.active]
		st = &sheetState{}
		mdw := render.DigitWidth(a.shaper, a.wb.Styles[0].Font)
		st.view = render.View{
			Sheet:      sh,
			Styles:     a.wb.Styles,
			Geo:        render.NewGeometry(sh, mdw),
			DigitWidth: mdw,
			Book:       a.wb,
			TopRow:     max(sh.TopRow, sh.FreezeRows),
			LeftCol:    max(sh.LeftCol, sh.FreezeCols),
		}
		start := render.Pos{Row: sh.ActiveRow, Col: sh.ActiveCol}
		st.view.Sel = render.Selection{Active: start, Range: model.Range{R1: start.Row, C1: start.Col, R2: start.Row, C2: start.Col}}
		st.selEnd = start
		a.states[a.active] = st
	}
	st.view.Shaper = a.shaper
	st.view.Zoom = a.zoom
	st.view.Styles = a.wb.Styles // formatting adds styles
	return st
}

// switchSheet activates the visible sheet delta tabs away from the current.
func (a *App) switchSheet(delta int) {
	n := len(a.wb.Sheets)
	for i, idx := 0, a.active; i < n; i++ {
		idx = (idx + delta + n) % n
		if !a.wb.Sheets[idx].Hidden {
			a.active = idx
			a.grid.focusPending = true
			return
		}
	}
}

// setZoom sets the zoom factor, clamped to 10%–400%.
func (a *App) setZoom(z float32) {
	a.zoom = min(max(z, 0.1), 4)
}

// requestFocus gives the keyboard focus to tag.
func requestFocus(gtx layout.Context, tag any) {
	gtx.Execute(key.FocusCmd{Tag: tag})
}
