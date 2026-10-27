// Command excelshot renders a sheet's grid to a PNG without opening a
// window, for checking rendering changes:
//
//	excelshot -o out.png [-sheet N] [-w 1280] [-h 800] [-zoom 1] file.xlsx
//
// With -page (a File page: home, new, open, info, "save as", print,
// export), -menu N (open drop-down N), -edit TEXT (typing TEXT into the
// active cell), -ask (the "save changes?" prompt) or -window, the whole
// window is drawn instead of just the grid.
package main

import (
	"flag"
	"image"
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
	if *page != "" || *menu >= 0 || *window || *typed != "" || *ask {
		list := recent.Open("")
		for _, s := range []string{"Samples/leasing.xls", flag.Arg(0)} {
			if abs, err := filepath.Abs(s); err == nil {
				list.Add(abs, recent.Spreadsheet)
			}
		}
		path, _ := filepath.Abs(flag.Arg(0))
		var router input.Router
		gtx.Source = router.Source() // enabled widgets, as in a window
		ui.Preview(gtx, fonts, list, wb, path, ui.PreviewOptions{Page: *page, Menu: *menu, Edit: *typed, Ask: *ask})
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
