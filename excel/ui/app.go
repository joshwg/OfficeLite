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

	"officelite/common/doctype"
	"officelite/common/gui"
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
var openExtensions = []string{".xlsx", ".xlsm", ".xls", ".csv", ".tsv", ".txt"}

type sheetState struct {
	view   render.View
	selEnd render.Pos // moving corner when extending the selection
}

type loadResult struct {
	path   string // file the workbook came from; "" if unknown
	wb     *model.Workbook
	shaper *text.Shaper
	err    error
}

type App struct {
	win    *app.Window
	ui     *gui.Theme
	th     *material.Theme // ui.Theme
	expl   *explorer.Explorer
	fonts  *gui.FontLibrary
	recent *recent.List
	launch func(kind doctype.Kind, path string)
	gtx    layout.Context // the frame being laid out, for backstage pages

	wb       *model.Workbook
	path     string      // file the workbook was loaded from or saved to
	fileInfo os.FileInfo // of path, when loaded
	modified bool        // unsaved changes
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
	calls   chan func() // work posted to the UI goroutine
	stats   statsCache
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
		calls:   make(chan func(), 8),
		states:  map[int]*sheetState{},
	}
	a.th = a.ui.Theme
	a.vbar.Vertical = true
	a.shaper = a.th.Shaper
	if cfg.Path != "" {
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
		switch e := e.(type) {
		case *app.ClosingEvent:
			// Gio lets a window refuse to close (WM_CLOSE on Windows):
			// ask about unsaved changes first.
			if a.modified && !a.mayClose {
				e.Abort()
				a.askSaveBeforeClose()
				w.Invalidate()
			}
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			a.runCalls()
			a.takeResult()
			gtx := app.NewContext(&ops, e)
			a.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// post runs f on the UI goroutine at the next frame.
func (a *App) post(f func()) {
	a.calls <- f
	a.win.Invalidate()
}

func (a *App) runCalls() {
	for {
		select {
		case f := <-a.calls:
			f()
		default:
			return
		}
	}
}

func emptyWorkbook() *model.Workbook {
	return &model.Workbook{
		Name:   fmt.Sprintf("Book%d", bookSeq.Add(1)),
		Sheets: []*model.Sheet{model.NewSheet("Sheet1")},
		Styles: []model.Style{model.DefaultStyle()},
	}
}

func (a *App) openPath(path string) {
	a.loading.Store(true)
	go func() {
		wb, err := fileio.Open(path)
		a.finishLoad(path, wb, err)
	}()
}

// openDialog asks for a file and opens it like a recent file. Where the
// platform gives no path, the data is read into this window.
func (a *App) openDialog() {
	go func() {
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
			a.post(func() { a.openFile(path) })
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
	res := loadResult{path: path, wb: wb, err: err}
	if wb != nil {
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
	a.win.Invalidate()
}

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

func (a *App) takeResult() {
	select {
	case r := <-a.results:
		a.loading.Store(false)
		if r.err != nil {
			a.errMsg = r.err.Error()
			return
		}
		a.errMsg = ""
		a.shaper = r.shaper
		a.th.Shaper = r.shaper
		a.path, a.fileInfo, a.modified = r.path, nil, false
		if r.path != "" {
			a.fileInfo, _ = os.Stat(r.path)
			a.recent.Add(r.path, recent.Spreadsheet)
		}
		a.setWorkbook(r.wb)
	default:
	}
}

func (a *App) setWorkbook(wb *model.Workbook) {
	a.wb = wb
	if a.book != nil {
		a.book.Close()
	}
	a.book = edit.New(wb)
	a.ed.active = false
	a.rename.active = false
	a.clip = nil
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
	if a.win != nil {
		a.win.Option(windowTitle(wb.Name))
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

func (a *App) setZoom(z float32) {
	a.zoom = min(max(z, 0.1), 4)
}

func requestFocus(gtx layout.Context, tag any) {
	gtx.Execute(key.FocusCmd{Tag: tag})
}
