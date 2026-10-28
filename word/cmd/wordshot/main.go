// Command wordshot renders a document page to a PNG without opening a
// window, for checking rendering changes:
//
//	wordshot -o out.png [-page N] [-zoom 1] [file.docx]
//
// Without a file it renders a built-in sample with headings, lists, tabs,
// a table and character formatting. With -window the whole WordLite window
// is drawn (-w, -h; -backstage names a File page, -menu N opens a menu,
// -dialog style|close|context|colresize|dragtext|... shows a dialog or
// a mouse interaction).
package main

import (
	"flag"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"

	"gioui.org/gpu/headless"
	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/word/fileio"
	wl "officelite/word/layout"
	"officelite/word/model"
	"officelite/word/ui"
)

func main() {
	out := flag.String("o", "wordshot.png", "output PNG")
	page := flag.Int("page", 1, "page number to draw")
	zoom := flag.Float64("zoom", 1, "zoom factor")
	window := flag.Bool("window", false, "draw the whole window")
	w := flag.Int("w", 1280, "window width in pixels (-window)")
	h := flag.Int("h", 860, "window height in pixels (-window)")
	backstage := flag.String("backstage", "", "File page to show (-window)")
	menu := flag.Int("menu", -1, "menu to open (-window)")
	dialog := flag.String("dialog", "", "dialog to show: style or close (-window)")
	flag.Parse()

	var doc *model.Document
	path := ""
	if flag.NArg() > 0 {
		var err error
		path, _ = filepath.Abs(flag.Arg(0))
		doc, err = fileio.Open(flag.Arg(0))
		if err != nil {
			log.Fatal(err)
		}
	} else {
		doc = Sample()
	}
	for _, warn := range doc.Warnings {
		log.Println("warning:", warn)
	}
	fonts := gui.NewFontLibrary(gui.DefaultFontDirs()...)

	var ops op.Ops
	var width, height int
	if *window || *backstage != "" || *menu >= 0 || *dialog != "" {
		width, height = *w, *h
		gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(width, height))}
		var router input.Router
		gtx.Source = router.Source()
		ui.Preview(gtx, fonts, recent.Open(""), doc, path, *backstage, *menu, *dialog)
	} else {
		fams := []string{"Calibri", "Calibri Light", "Aptos", "Times New Roman", "Arial", "Symbol", "Courier New"}
		doc.Paragraphs(func(p *model.Paragraph) bool {
			for _, r := range p.Runs {
				if r.Props.Font != "" {
					fams = append(fams, r.Props.Font)
				}
			}
			return true
		})
		for _, st := range doc.Styles.Styles {
			if st.Run.Font != "" {
				fams = append(fams, st.Run.Font)
			}
		}
		eng := wl.New(doc, fonts.Shaper(fams...))
		eng.Layout()
		log.Printf("%d pages, laid out in %v", len(eng.Pages), eng.LastDuration)
		if *page < 1 || *page > len(eng.Pages) {
			log.Fatalf("page %d of %d", *page, len(eng.Pages))
		}
		pg := eng.Pages[*page-1]
		scale := *zoom * 96 / 72
		width, height = int(math.Ceil(pg.Width*scale)), int(math.Ceil(pg.Height*scale))
		eng.DrawPage(&ops, pg, wl.DrawOptions{Scale: scale})
	}

	win, err := headless.NewWindow(width, height)
	if err != nil {
		log.Fatal(err)
	}
	defer win.Release()
	if err := win.Frame(&ops); err != nil {
		log.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
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
