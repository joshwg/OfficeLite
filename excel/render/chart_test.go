package render

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"officelite/excel/internal/chartsample"
	"officelite/excel/model"
	"officelite/excel/numfmt"
)

func layoutSample(t *testing.T, wb *model.Workbook, c *model.Chart) []prim {
	t.Helper()
	nf := numfmt.New(false)
	t.Cleanup(nf.Close)
	return layoutChart(c, &chartEnv{wb: wb, home: "Data", format: nf.Format}, 480, 288)
}

func texts(prims []prim) []string {
	var out []string
	for _, p := range prims {
		if p.kind == primText {
			out = append(out, p.text)
		}
	}
	return out
}

func hasText(prims []prim, s string) bool {
	for _, t := range texts(prims) {
		if t == s {
			return true
		}
	}
	return false
}

// Every sample chart lays out: finite coordinates, its title, and
// something drawn in its series' colours.
func TestLayoutChartTypes(t *testing.T) {
	wb := chartsample.Workbook()
	th := model.DefaultTheme()
	for i, c := range wb.Sheets[0].Charts {
		prims := layoutSample(t, wb, c)
		if len(prims) < 10 {
			t.Errorf("chart %d (%s): %d primitives", i, c.Title.Text, len(prims))
			continue
		}
		for _, p := range prims {
			for _, q := range p.pts {
				if math.IsNaN(float64(q.X)) || math.IsNaN(float64(q.Y)) || math.IsInf(float64(q.X), 0) {
					t.Fatalf("chart %d: bad point %v", i, q)
				}
			}
		}
		if !hasText(prims, c.Title.Text) {
			t.Errorf("chart %d: no title %q in %v", i, c.Title.Text, texts(prims))
		}
		accent := false
		for _, p := range prims {
			if p.color == th.SeriesColor(0) {
				accent = true
			}
		}
		if !accent {
			t.Errorf("chart %d (%s): nothing in the first series' colour", i, c.Title.Text)
		}
	}
	// The value axis of the clustered column chart: $0 to $250 by $50
	// ($205 the largest value), formatted as the cells are.
	prims := layoutSample(t, wb, wb.Sheets[0].Charts[0])
	for _, want := range []string{"$0", "$50", "$250", "Jan", "Jun", "North", "West"} {
		if !hasText(prims, want) {
			t.Errorf("column chart: no %q in %v", want, texts(prims))
		}
	}
	if hasText(prims, "$300") {
		t.Error("column chart: axis too long")
	}
	// The 100% stacked chart's axis is in percent.
	if prims := layoutSample(t, wb, wb.Sheets[0].Charts[2]); !hasText(prims, "100%") || !hasText(prims, "50%") {
		t.Errorf("percent axis: %v", texts(prims))
	}
	// The combo chart's secondary axis.
	if prims := layoutSample(t, wb, wb.Sheets[0].Charts[10]); !hasText(prims, "25%") || !hasText(prims, "$250") {
		t.Errorf("combo axes: %v", texts(prims))
	}
	// Negative values: the axis goes below zero, the labels show them.
	if prims := layoutSample(t, wb, wb.Sheets[0].Charts[11]); !hasText(prims, "-40") || !hasText(prims, "-30") {
		t.Errorf("negative chart: %v", texts(prims))
	}
}

// Pie slices start at 12 o'clock and go clockwise, in the data's order.
func TestPieSliceOrder(t *testing.T) {
	wb := chartsample.Workbook()
	pie := wb.Sheets[0].Charts[5]
	th := model.DefaultTheme()
	prims := layoutSample(t, wb, pie)
	var first *prim
	for i := range prims {
		if prims[i].kind == primFill && prims[i].color == th.SeriesColor(0) && len(prims[i].pts) > 10 {
			first = &prims[i]
			break
		}
	}
	if first == nil {
		t.Fatal("no first slice")
	}
	// The slice's points run from 12 o'clock clockwise, the centre last.
	c := first.pts[len(first.pts)-1]
	p0, p1 := first.pts[0], first.pts[1]
	if math.Abs(float64(p0.X-c.X)) > 1 || p0.Y >= c.Y {
		t.Errorf("first slice doesn't start at 12 o'clock: %v centre %v", p0, c)
	}
	if p1.X <= p0.X {
		t.Errorf("first slice not clockwise: %v then %v", p0, p1)
	}
	// 38% of the pie: it ends past 3 o'clock, before 6.
	end := first.pts[len(first.pts)-2]
	if end.Y <= c.Y || end.X <= c.X {
		t.Errorf("first slice ends at %v (centre %v)", end, c)
	}
	if !hasText(prims, "Laptops, 38%") {
		t.Errorf("labels %v", texts(prims))
	}
}

// Charts draw from the cells as they are now.
func TestChartFollowsCells(t *testing.T) {
	wb := chartsample.Workbook()
	c := wb.Sheets[0].Charts[0]
	before := layoutSample(t, wb, c)
	wb.Sheets[0].Set(6, 1, model.Cell{Kind: model.KindNumber, Num: 480, Text: "$480"})
	after := layoutSample(t, wb, c)
	if !hasText(after, "$600") || hasText(before, "$600") {
		t.Errorf("axis didn't follow the cell: before %v after %v", texts(before), texts(after))
	}
	// Renamed category.
	wb.Sheets[0].Set(1, 0, model.Cell{Kind: model.KindText, Text: "January"})
	if !hasText(layoutSample(t, wb, c), "January") {
		t.Error("category label didn't follow the cell")
	}
	// Without the workbook (or with a broken reference): the cache.
	c.Plots[0].Series[0].Cat = model.ChartData{Ref: "#REF!", Cache: []model.ChartValue{{Kind: model.ValueText, Text: "Cached"}}}
	if !hasText(layoutSample(t, wb, c), "Cached") {
		t.Error("cache not used")
	}
}

// Blanks: a gap splits the line; zero plots them at 0; span joins over.
func TestChartBlanks(t *testing.T) {
	wb := chartsample.Workbook()
	c := wb.Sheets[0].Charts[9] // the scatter with a blank in Trend
	lines := func() int {
		n := 0
		for _, p := range layoutSample(t, wb, c) {
			if p.kind == primStroke && p.color == model.DefaultTheme().SeriesColor(1) && p.width >= 2 && !p.closed {
				n++
			}
		}
		return n
	}
	// The legend's key is one more.
	if n := lines(); n != 3 {
		t.Errorf("gap: %d line pieces, want 2 and the key", n)
	}
	c.DispBlanks = model.BlanksSpan
	if n := lines(); n != 2 {
		t.Errorf("span: %d line pieces, want 1 and the key", n)
	}
}

func TestGeneralAxisLabels(t *testing.T) {
	if s := strings.Join([]string{generalNum(0.30000000000000004), generalNum(-0.0), generalNum(1e12)}, " "); s != "0.3 0 1E+12" {
		t.Errorf("general: %s", s)
	}
}

// Charts print as vector paths: the sheet's pages take them in, and a
// chart sheet is a page of its own.
func TestChartsPDF(t *testing.T) {
	wb := chartsample.Workbook()
	n, out := exportPages(t, wb, PDFOptions{Sheets: []int{0}})
	if n < 4 {
		t.Errorf("%d pages for the data sheet and its charts", n)
	}
	if !bytes.Contains(out, []byte(" l ")) || !bytes.Contains(out, []byte("h f")) {
		t.Error("no paths in the PDF")
	}
	if n, _ := exportPages(t, wb, PDFOptions{Sheets: []int{1}}); n != 1 {
		t.Errorf("chart sheet: %d pages", n)
	}
}
