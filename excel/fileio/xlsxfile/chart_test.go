package xlsxfile

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"officelite/common/colors"
	"officelite/excel/formula"
	"officelite/excel/internal/chartsample"
	"officelite/excel/model"
)

// chartRoundTrip writes wb and reads it back.
func chartRoundTrip(t *testing.T, wb *model.Workbook) (*model.Workbook, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(wb, &buf); err != nil {
		t.Fatal(err)
	}
	back, err := Read(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return back, buf.Bytes()
}

// parts lists a package's parts and their contents.
func parts(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func TestReadExcelChart(t *testing.T) {
	data, err := os.ReadFile("../../../Samples/Pivot-Tables-and-Charts.xlsx")
	if err != nil {
		t.Skip(err)
	}
	wb, err := Read(data)
	if err != nil {
		t.Fatal(err)
	}
	sh := wb.Sheets[2]
	if len(sh.Charts) != 1 {
		t.Fatalf("%d charts", len(sh.Charts))
	}
	c := sh.Charts[0]
	a := c.Anchor
	if a.Kind != model.AnchorTwoCell || a.From != (model.CellOffset{Row: 1, Col: 4}) || a.To != (model.CellOffset{Row: 15, Col: 11, DX: 304800, DY: 76200}) {
		t.Errorf("anchor %+v", a)
	}
	if len(c.Plots) != 1 || c.Plots[0].Type != model.ChartBar || c.Plots[0].Horizontal || c.Plots[0].GapWidth != 219 || c.Plots[0].Overlap != -27 {
		t.Fatalf("plots %+v", c.Plots)
	}
	s := c.Plots[0].Series[1]
	if s.Name.Ref != "PivotChart!$C$1" || s.Val.Ref != "PivotChart!$C$2:$C$7" || len(s.Val.Cache) != 5 || s.Val.Cache[3].Num != 10 {
		t.Errorf("series %+v", s)
	}
	// accent2 of this (Office 2007) theme.
	if s.Shape.Fill.Kind != model.FillSolid || s.Shape.Fill.Color != colors.RGB(0xC0504D) {
		t.Errorf("series fill %+v", s.Shape.Fill)
	}
	if c.Legend == nil || c.Legend.Pos != "r" || len(c.Axes) != 2 || !c.Axes[1].MajorGrid || c.Axes[0].Kind != model.AxisCat {
		t.Errorf("legend %+v axes %+v", c.Legend, c.Axes)
	}
	if c.PivotSource == "" || c.Raw == nil || len(c.Raw.Refs) != 6 || len(c.Raw.Parts) != 2 {
		t.Errorf("pivot %q raw %+v", c.PivotSource, c.Raw)
	}
	if wb.Theme == nil || wb.Theme.MinorFont != "Calibri" || wb.Theme.Colors[model.ThemeAccent1] != colors.RGB(0x4F81BD) {
		t.Errorf("theme %+v", wb.Theme)
	}

	// Saved as read: the original part with its style and colour parts.
	back, out := chartRoundTrip(t, wb)
	p := parts(t, out)
	if !strings.Contains(p["xl/charts/chart1.xml"], "c:pivotSource") || p["xl/charts/style1.xml"] == "" || p["xl/charts/colors1.xml"] == "" {
		t.Errorf("kept parts: %v", keys(p))
	}
	if p["xl/theme/theme1.xml"] != string(wb.Theme.XML) {
		t.Error("theme not kept")
	}
	if len(back.Sheets[2].Charts) != 1 || back.Sheets[2].Charts[0].Plots[0].Series[1].Val.Ref != "PivotChart!$C$2:$C$7" {
		t.Error("chart lost in round trip")
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestWriteChartTypes(t *testing.T) {
	wb := chartsample.Workbook()
	back, out := chartRoundTrip(t, wb)
	p := parts(t, out)

	// Every part is well-formed XML.
	for name, s := range p {
		if strings.HasSuffix(name, ".xml") || strings.HasSuffix(name, ".rels") {
			d := xml.NewDecoder(strings.NewReader(s))
			for {
				if _, err := d.Token(); err == io.EOF {
					break
				} else if err != nil {
					t.Errorf("%s: %v", name, err)
					break
				}
			}
		}
	}
	src, got := wb.Sheets[0].Charts, back.Sheets[0].Charts
	if len(got) != len(src) {
		t.Fatalf("%d charts read back, want %d", len(got), len(src))
	}
	for i, c := range src {
		g := got[i]
		if g.Anchor != c.Anchor || g.Title == nil || g.Title.Text != c.Title.Text || len(g.Plots) != len(c.Plots) {
			t.Errorf("chart %d: %+v", i, g)
			continue
		}
		for k := range c.Plots {
			cp, gp := &c.Plots[k], &g.Plots[k]
			if gp.Type != cp.Type || gp.Grouping != cp.Grouping || gp.Horizontal != cp.Horizontal || len(gp.Series) != len(cp.Series) {
				t.Errorf("chart %d plot %d: %+v", i, k, gp)
				continue
			}
			for j := range cp.Series {
				cs, gs := &cp.Series[j], &gp.Series[j]
				if gs.Val.Ref != cs.Val.Ref || gs.Cat.Ref != cs.Cat.Ref || gs.Name.Ref != cs.Name.Ref || len(gs.Val.Cache) == 0 {
					t.Errorf("chart %d series %d: %+v", i, j, gs)
				}
			}
		}
		if g.Raw == nil {
			t.Errorf("chart %d: not kept as read", i)
		}
	}
	// The doughnut's hole, the combo's secondary axis, the scatter's
	// markers.
	if p := got[6].Plots[0]; p.Type != model.ChartDoughnut || p.HoleSize != 50 || !p.VaryColors {
		t.Errorf("doughnut %+v", p)
	}
	if c := got[10]; len(c.Axes) != 4 || c.Axis(1004).Pos != "r" || c.Axis(1004).Crosses != "max" || c.Plots[1].AxisIDs[1] != 1004 {
		t.Errorf("combo axes %+v", c.Axes)
	}
	if s := got[8].Plots[0].Series[0]; s.Marker.Symbol != "circle" || s.Shape.Line.Kind != model.FillNone {
		t.Errorf("scatter series %+v", s)
	}
	// The cached values are the cells'.
	if v := got[0].Plots[0].Series[0].Val.Cache; len(v) != 6 || v[5].Num != 205 {
		t.Errorf("cache %+v", v)
	}
	// The chart sheet comes back in its place.
	if len(back.Sheets) != 2 || !back.Sheets[1].IsChartSheet() || back.Sheets[1].Name != "Chart Sheet" {
		t.Fatalf("sheets %+v", back.Sheets)
	}
	if _, ok := p["xl/chartsheets/sheet1.xml"]; !ok || strings.Contains(p["[Content_Types].xml"], "/xl/worksheets/sheet2.xml") {
		t.Errorf("chart sheet parts: %v", keys(p))
	}
	if !strings.Contains(p["xl/_rels/workbook.xml.rels"], `Target="chartsheets/sheet1.xml"`) {
		t.Error("chart sheet relationship")
	}
	// The worksheet's <drawing> comes before its legacyDrawing, if any,
	// and after the merge cells / page margins.
	ws := p["xl/worksheets/sheet1.xml"]
	if i := strings.Index(ws, "<drawing"); i < 0 || i < strings.Index(ws, "</sheetData>") {
		t.Errorf("drawing element misplaced: %s", ws[max(len(ws)-400, 0):])
	}
}

// A chart kept as read follows structural edits: its references (and the
// cache) are rewritten in the saved part.
func TestRawChartRefsFollowEdits(t *testing.T) {
	wb := chartsample.Workbook()
	back, _ := chartRoundTrip(t, wb)
	c := back.Sheets[0].Charts[0]
	if c.Raw == nil {
		t.Fatal("not raw")
	}
	sh := back.Sheets[0]
	// Insert two rows above the data, as edit does.
	c.Refs(func(s *string) { *s = formula.AdjustRows(*s, sh.Name, "Data", 0, 2) })
	sh.InsertRows(0, 2)
	sh.Set(7, 1, model.Cell{Kind: model.KindNumber, Num: 999, Text: "999"}) // Jun's North, now row 8
	_, out := chartRoundTrip(t, back)
	p := parts(t, out)
	x := p["xl/charts/chart1.xml"]
	if !strings.Contains(x, "<c:f>Data!$B$4:$B$9</c:f>") || strings.Contains(x, "Data!$B$2:$B$7") {
		t.Errorf("references not rewritten: %s", x[:min(len(x), 1500)])
	}
	if !strings.Contains(x, "<c:v>999</c:v>") {
		t.Error("cache not refreshed")
	}
}

func TestChartRefSpans(t *testing.T) {
	x := `<c:chartSpace xmlns:c="` + nsChart + `"><c:chart><c:title><c:tx><c:strRef><c:f>S!$A$1</c:f></c:strRef></c:tx></c:title>` +
		`<c:plotArea><c:lineChart><c:ser><c:tx><c:strRef><c:f>S!$B$1</c:f></c:strRef></c:tx>` +
		`<c:cat><c:strRef><c:f>S!$A$2:$A$3</c:f><c:strCache><c:ptCount val="0"/></c:strCache></c:strRef></c:cat>` +
		`<c:val><c:numRef><c:f>S!$B$2:$B$3</c:f><c:numCache><c:ptCount val="0"/></c:numCache></c:numRef></c:val></c:ser></c:lineChart>` +
		`<c:valAx><c:title><c:tx><c:strRef><c:f>S!$C$1</c:f></c:strRef></c:tx></c:title></c:valAx></c:plotArea></c:chart></c:chartSpace>`
	spans, ok := chartRefSpans([]byte(x))
	if !ok || len(spans) != 5 {
		t.Fatalf("%v %+v", ok, spans)
	}
	want := []model.RawRef{{Plot: -1}, {Plot: 0, Field: model.RefName}, {Plot: 0, Field: model.RefCat}, {Plot: 0, Field: model.RefVal}, {Plot: -2, Series: 0}}
	for i, s := range spans {
		w := want[i]
		if s.at.Plot != w.Plot || s.at.Series != w.Series || s.at.Field != w.Field {
			t.Errorf("span %d: %+v, want %+v", i, s.at, w)
		}
		if got := x[s.start:s.end]; got != s.at.Text {
			t.Errorf("span %d text %q vs %q", i, got, s.at.Text)
		}
	}
	if s := spans[3]; s.cacheEnd == 0 || !strings.HasPrefix(x[s.cacheStart:], "<c:numCache>") || !strings.HasSuffix(x[:s.cacheEnd], "</c:numCache>") {
		t.Errorf("cache span %+v", s)
	}
}

// Charts made and changed by the editing commands are written from the
// model and read back the same.
func TestCreatedChartsRoundTrip(t *testing.T) {
	wb := chartsample.Created()
	back, out := chartRoundTrip(t, wb)
	p := parts(t, out)
	for name, s := range p {
		if strings.HasSuffix(name, ".xml") {
			d := xml.NewDecoder(strings.NewReader(s))
			for {
				if _, err := d.Token(); err == io.EOF {
					break
				} else if err != nil {
					t.Errorf("%s: %v", name, err)
					break
				}
			}
		}
	}
	if len(back.Sheets) != 2 || !back.Sheets[1].IsChartSheet() || back.Sheets[1].Name != "Sales Chart" {
		t.Fatalf("sheets: %d", len(back.Sheets))
	}
	src, got := append(slices.Clone(wb.Sheets[0].Charts), wb.Sheets[1].ChartSheet), append(slices.Clone(back.Sheets[0].Charts), back.Sheets[1].ChartSheet)
	if len(got) != len(src) {
		t.Fatalf("%d charts read back, want %d", len(got), len(src))
	}
	for i, c := range src {
		g := got[i]
		if c.Raw != nil {
			t.Errorf("chart %d made here has a part as read", i)
		}
		if i < len(src)-1 && g.Anchor != c.Anchor {
			t.Errorf("chart %d anchor %+v, want %+v", i, g.Anchor, c.Anchor)
		}
		if (g.Title == nil) != (c.Title == nil) || g.Title != nil && g.Title.Text != c.Title.Text || g.AutoTitleDeleted != c.AutoTitleDeleted {
			t.Errorf("chart %d title %+v, want %+v", i, g.Title, c.Title)
		}
		if (g.Legend == nil) != (c.Legend == nil) || g.Legend != nil && g.Legend.Pos != c.Legend.Pos {
			t.Errorf("chart %d legend %+v, want %+v", i, g.Legend, c.Legend)
		}
		if len(g.Plots) != len(c.Plots) || len(g.Axes) != len(c.Axes) {
			t.Errorf("chart %d: %d plots %d axes, want %d %d", i, len(g.Plots), len(g.Axes), len(c.Plots), len(c.Axes))
			continue
		}
		for k := range c.Plots {
			cp, gp := &c.Plots[k], &g.Plots[k]
			if gp.Type != cp.Type || gp.Grouping != cp.Grouping || gp.Horizontal != cp.Horizontal || gp.Marker != cp.Marker ||
				!slices.Equal(gp.AxisIDs, cp.AxisIDs) || len(gp.Series) != len(cp.Series) || cp.Labels.Any() != gp.Labels.Any() {
				t.Errorf("chart %d plot %d: %+v, want %+v", i, k, gp, cp)
				continue
			}
			for j := range cp.Series {
				cs, gs := &cp.Series[j], &gp.Series[j]
				if gs.Val.Ref != cs.Val.Ref || gs.Cat.Ref != cs.Cat.Ref || gs.Name.Ref != cs.Name.Ref ||
					cs.Shape.Fill.Kind == model.FillSolid && gs.Shape.Fill != cs.Shape.Fill {
					t.Errorf("chart %d series %d: %+v, want %+v", i, j, gs, cs)
				}
			}
		}
		for k := range c.Axes {
			ca, ga := &c.Axes[k], &g.Axes[k]
			if ga.ID != ca.ID || ga.Kind != ca.Kind || ga.Pos != ca.Pos || ga.Delete != ca.Delete || ga.MajorGrid != ca.MajorGrid ||
				ga.MinorGrid != ca.MinorGrid || (ga.Title == nil) != (ca.Title == nil) {
				t.Errorf("chart %d axis %d: %+v, want %+v", i, k, ga, ca)
			}
		}
		if g.ChartArea.Fill != c.ChartArea.Fill && c.ChartArea.Fill.Kind != model.FillAuto {
			t.Errorf("chart %d chart area %+v, want %+v", i, g.ChartArea, c.ChartArea)
		}
	}
	// Labels Excel refuses for a kind aren't written (100% stacked
	// columns take ctr, inEnd, inBase).
	for name, s := range p {
		if strings.HasPrefix(name, "xl/charts/") && strings.Contains(s, "percentStacked") && strings.Contains(s, `<c:dLblPos val="outEnd"/>`) {
			t.Errorf("%s: outEnd labels on stacked bars", name)
		}
	}
}
