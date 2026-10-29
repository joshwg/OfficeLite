// Charts in PDF exports: laid out as on screen (chart.go) and drawn with
// vector paths and embedded-font text, in pixels scaled to points.

package render

import (
	"math"

	"officelite/common/pdf"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

// chartEnvPDF is the layout environment of the export's charts.
func (x *sheetExport) chartEnvPDF() *chartEnv {
	if x.nf == nil {
		x.nf = numfmt.New(x.wb.Date1904)
	}
	nf := x.nf
	return &chartEnv{
		wb:   x.wb,
		home: x.sh.Name,
		measure: func(f chartFont, s string) (float32, float32) {
			pf := x.font(model.Font{Family: f.Family, Bold: f.Bold, Italic: f.Italic})
			px := float64(f.px())
			return float32(pf.Measure(s, px)), float32(px * 1.22)
		},
		format: func(v float64, id int, code string) string { return nf.Format(v, id, code) },
	}
}

// drawChartPDF draws a chart w × h pixels with its top-left corner at
// ox, oy (points, in the page's current coordinates).
func (x *sheetExport) drawChartPDF(c *model.Chart, ox, oy, w, h float64) {
	prims := layoutChart(c, x.chartEnvPDF(), float32(w), float32(h))
	p := x.p
	p.Save()
	p.Transform(pxToPt, 0, 0, pxToPt, ox, oy)
	p.ClipRect(0, 0, w, h)
	pts := func(in []chartPt) []pdf.Point {
		out := make([]pdf.Point, len(in))
		for i, q := range in {
			out[i] = pdf.Point{X: float64(q.X), Y: float64(q.Y)}
		}
		return out
	}
	for i := range prims {
		pr := &prims[i]
		switch pr.kind {
		case primFill:
			p.FillPolygon(pts(pr.pts), pr.color)
		case primStroke:
			var dash []float64
			for _, d := range pr.dash {
				dash = append(dash, float64(d))
			}
			p.StrokePolyline(pts(pr.pts), pr.closed, float64(pr.width), pr.color, dash...)
		case primText:
			f := pr.font
			pf := x.font(model.Font{Family: f.Family, Bold: f.Bold, Italic: f.Italic})
			size := float64(f.px())
			tw := pf.Measure(pr.text, size)
			th := size * 1.22
			dx := map[int8]float64{-1: 0, 0: -tw / 2, 1: -tw}[pr.ha]
			dy := map[int8]float64{-1: 0, 0: -th / 2, 1: -th}[pr.va]
			base := dy + (th-size)/2 + pf.Ascent(size)
			if pr.rot == 0 {
				p.DrawText(pdf.Run{Font: pf, Size: size, X: float64(pr.x) + dx, Y: float64(pr.y) + base, Text: pr.text, Color: f.Color})
				continue
			}
			rad := float64(pr.rot) * math.Pi / 180
			cs, sn := math.Cos(rad), math.Sin(rad)
			p.Save()
			p.Transform(cs, sn, -sn, cs, float64(pr.x), float64(pr.y))
			p.DrawText(pdf.Run{Font: pf, Size: size, X: dx, Y: base, Text: pr.text, Color: f.Color})
			p.Restore()
		}
	}
	p.Restore()
}

// chartCells is the range of cells a chart covers, for the print area.
func (x *sheetExport) chartCells(c *model.Chart) model.Range {
	x0, y0, x1, y1 := ChartBox(c, x.geo)
	return model.Range{R1: x.geo.Rows.Index(y0), C1: x.geo.Cols.Index(x0), R2: x.geo.Rows.Index(max(y1-1, y0)), C2: x.geo.Cols.Index(max(x1-1, x0))}
}

// pageCharts draws the sheet's charts that reach into the page's band.
func (x *sheetExport) pageCharts() {
	for _, c := range x.sh.Charts {
		cells := x.chartCells(c)
		b := x.band
		if cells.R2 < b.R1 || cells.R1 > b.R2 || cells.C2 < b.C1 || cells.C1 > b.C2 {
			continue
		}
		x0, y0, x1, y1 := ChartBox(c, x.geo)
		ox := (x0 - x.geo.Cols.Start(b.C1)) * pxToPt
		oy := (y0 - x.geo.Rows.Start(b.R1)) * pxToPt
		x.drawChartPDF(c, ox, oy, x1-x0, y1-y0)
	}
}

// chartSheet prints a chart sheet: its chart on a landscape page.
func (x *sheetExport) chartSheet(sh *model.Sheet) {
	paper := pdf.Size{W: max(x.o.Paper.W, x.o.Paper.H), H: min(x.o.Paper.W, x.o.Paper.H)}
	x.p = x.d.AddPage(paper)
	w, h := paper.W-2*marginLR, paper.H-2*marginTB
	x.drawChartPDF(sh.ChartSheet, marginLR, marginTB, w/pxToPt, h/pxToPt)
}
