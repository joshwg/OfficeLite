// Command excelshot renders a sheet's grid to a PNG without opening a
// window, for checking rendering changes:
//
//	excelshot -o out.png [-sheet N] [-w 1280] [-h 800] [-zoom 1] file.xlsx
//
// With -page (a File page: home, new, open, info, "save as", print,
// export), -menu N (open drop-down N), -edit TEXT (typing TEXT into the
// active cell), -ask (the "save changes?" prompt), -ribbon data, -filter
// B1 (an AutoFilter drop-down; -filtersub, -dialog custom|top10), -data
// ttc|dups|validation|list|circles|... (Data Tools states) or
// group|subtotal|hide|show|rows2|cols1|... (Outline states), -ribbon
// analyze|design and -pivot create|value|field|menu|chip (PivotTable
// states, with -select in a report), -ribbon chartdesign|chartformat and
// -chartsel N -chartui insert|type|combo|data|series|move|format|ctx|
// drag|resize|title|series1|alt|f11 (chart states), -undo undo|redo
// (the Home ribbon's Undo or Redo list after a few changes), -autosave
// on|saved (the title bar's AutoSave switch) or -window,
// the whole window is drawn instead of just the grid.
package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"

	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/excel/calc"
	"officelite/excel/fileio"
	"officelite/excel/formula/functions"
	"officelite/excel/render"
	"officelite/excel/ui"
)

func main() {
	out := flag.String("o", "excelshot.png", "output PNG")
	sheet := flag.Int("sheet", 0, "sheet index")
	w := flag.Int("w", 1280, "width in pixels")
	h := flag.Int("h", 800, "height in pixels")
	zoom := flag.Float64("zoom", 1, "zoom factor")
	page := flag.String("page", "", "draw the window showing this File page")
	menu := flag.Int("menu", -1, "draw the window with this menu open")
	window := flag.Bool("window", false, "draw the whole window, not just the grid")
	typed := flag.String("edit", "", "draw the window editing the active cell with this text")
	ask := flag.Bool("ask", false, "draw the window asking to save changes")
	sel := flag.String("select", "", "select these ranges first, e.g. B2:C4,E6 (window views)")
	ctx := flag.String("ctx", "", "draw the window with a right-click menu open: cell, row, col or tab")
	drag := flag.String("drag", "", "draw the window dragging the selection by its border to this cell")
	ribbon := flag.String("ribbon", "", "draw the window showing this ribbon: home, data, analyze, design (PivotTable), chartdesign or chartformat")
	filter := flag.String("filter", "", "draw the window with the AutoFilter drop-down of this header cell open, e.g. B1")
	filterSub := flag.Bool("filtersub", false, "with -filter, also open its Text/Number/Date Filters submenu")
	dialog := flag.String("dialog", "", "with -filter, open this dialog for the column instead: custom or top10")
	data := flag.String("data", "", "draw the window with a Data Tools state for the selection: ttc, ttc-fixed, dups, validation, validation-input, validation-error, list, circles or alert (with -edit); or an Outline state: group, subtotal, hide, show, rowsN or colsN (level button N)")
	chart := flag.Int("chart", -1, "draw only the sheet's chart N (from 0) at its size times -zoom")
	chartSel := flag.Int("chartsel", 0, "draw the window with the sheet's chart N (from 1) selected")
	chartUI := flag.String("chartui", "", "with -chartsel, draw a chart state: insert, type, combo, data, series, move, format (dialogs), ctx (its menu), drag, resize (a drag's outline), title (editing it), series1 (its first series selected), alt (Alt+F1 on the selection), f11 (F11 on the selection)")
	pivot := flag.String("pivot", "", "draw the window with a PivotTable state for the active cell's report: create, value, field, menu or chip")
	undo := flag.String("undo", "", "draw the window after a few changes with the Home ribbon's undo (or redo) list open")
	autoSave := flag.String("autosave", "", "draw the window with the title bar's AutoSave switch on, or on and just saved: on or saved")
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	wb, err := fileio.Open(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	calc.Recalculate(wb, functions.Default(), calc.Missing, time.Now())
	for _, warn := range wb.Warnings {
		log.Println("warning:", warn)
	}
	sh := wb.Sheets[*sheet]
	fams := []string{"Segoe UI", "Tahoma"}
	for _, st := range wb.Styles {
		fams = append(fams, st.Font.Family)
	}
	fonts := gui.NewFontLibrary(gui.DefaultFontDirs()...)
	shaper := fonts.Shaper(fams...)
	mdw := render.DigitWidth(shaper, wb.Styles[0].Font)
	log.Printf("default font %s %vpt: digit width %vpx", wb.Styles[0].Font.Family, wb.Styles[0].Font.SizePt, mdw)
	v := render.View{
		Sheet:      sh,
		Styles:     wb.Styles,
		Geo:        render.NewGeometry(sh, mdw),
		DigitWidth: mdw,
		Shaper:     shaper,
		Book:       wb,
		TopRow:     max(sh.TopRow, sh.FreezeRows),
		LeftCol:    max(sh.LeftCol, sh.FreezeCols),
		Zoom:       float32(*zoom),
	}
	v.Sel.Active = render.Pos{Row: sh.ActiveRow, Col: sh.ActiveCol}
	v.Sel.Range.R1, v.Sel.Range.C1 = v.Sel.Active.Row, v.Sel.Active.Col
	v.Sel.Range.R2, v.Sel.Range.C2 = v.Sel.Active.Row, v.Sel.Active.Col

	win, err := headless.NewWindow(*w, *h)
	if err != nil {
		log.Fatal(err)
	}
	defer win.Release()
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(*w, *h)),
	}
	if *page != "" || *menu >= 0 || *window || *typed != "" || *ask || *sel != "" || *ctx != "" || *drag != "" || *ribbon != "" || *filter != "" || *data != "" || *pivot != "" || *chartSel > 0 || *chartUI != "" || *undo != "" || *autoSave != "" {
		list := recent.Open("")
		for _, s := range []string{"Samples/leasing.xls", flag.Arg(0)} {
			if abs, err := filepath.Abs(s); err == nil {
				list.Add(abs, recent.Spreadsheet)
			}
		}
		path, _ := filepath.Abs(flag.Arg(0))
		var router input.Router
		gtx.Source = router.Source() // enabled widgets, as in a window
		ui.Preview(gtx, fonts, list, wb, path, ui.PreviewOptions{Page: *page, Menu: *menu, Edit: *typed, Ask: *ask,
			Select: *sel, Context: *ctx, Drag: *drag, Ribbon: *ribbon, Filter: *filter, FilterSub: *filterSub, Dialog: *dialog, Data: *data, Sheet: *sheet, Pivot: *pivot, Chart: *chartSel, ChartUI: *chartUI, Undo: *undo, AutoSave: *autoSave})
	} else if *chart >= 0 {
		if *chart >= len(sh.Charts) {
			log.Fatalf("the sheet has %d charts", len(sh.Charts))
		}
		c := sh.Charts[*chart]
		x0, y0, x1, y1 := render.ChartBox(c, v.Geo)
		z := float32(*zoom)
		rc := image.Rect(0, 0, int(float32(x1-x0)*z), int(float32(y1-y0)*z))
		gui.FillRect(gtx.Ops, image.Rect(0, 0, *w, *h), color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		render.PaintChart(gtx, shaper, wb, sh.Name, c, rc, z)
	} else {
		v.Draw(gtx)
	}
	if err := win.Frame(&ops); err != nil {
		log.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, *w, *h))
	if err := win.Screenshot(img); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
