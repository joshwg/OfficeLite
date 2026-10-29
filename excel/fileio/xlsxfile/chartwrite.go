package xlsxfile

import (
	"bytes"
	"fmt"
	"image/color"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"officelite/excel/model"
)

// Writing charts: each worksheet with charts gets a drawing
// (xl/drawings/drawingN.xml, one anchor per chart) and each chart its
// part (xl/charts/chartN.xml), with the relationships, content types and
// the worksheet's <drawing>. A chart sheet is written by excelize as an
// empty worksheet placeholder (so the sheet order, defined names' sheet
// numbers and the active tab come out right) which patch turns into a
// chartsheet part.
//
// A chart read from a file and not edited is written as it came (with
// the parts it uses: its style, colours, shapes drawn on it), only its
// references and cached values brought up to date; other charts are
// written from the model, every colour explicit, so Excel draws them as
// ExcelLite does whatever the theme.

const (
	ctDrawing      = "application/vnd.openxmlformats-officedocument.drawing+xml"
	ctChart        = "application/vnd.openxmlformats-officedocument.drawingml.chart+xml"
	ctChartsheet   = "application/vnd.openxmlformats-officedocument.spreadsheetml.chartsheet+xml"
	ctWorksheet    = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
	nsDrawingSS    = "http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing"
	nsDrawingMain  = "http://schemas.openxmlformats.org/drawingml/2006/main"
	chartGraphicNS = "http://schemas.openxmlformats.org/drawingml/2006/chart"
)

// pendingCharts is a worksheet's charts, or a chart sheet's chart, to
// write.
type pendingCharts struct {
	sheet   int
	part    string // the worksheet part (for a chart sheet, its placeholder)
	charts  []*model.Chart
	isSheet bool // a chart sheet
}

// collectCharts notes the charts of sheet i.
func (x *writer) collectCharts(i int, sh *model.Sheet, part string) {
	switch {
	case sh.IsChartSheet():
		x.charts = append(x.charts, pendingCharts{sheet: i, part: part, charts: []*model.Chart{sh.ChartSheet}, isSheet: true})
	case len(sh.Charts) > 0:
		x.charts = append(x.charts, pendingCharts{sheet: i, part: part, charts: sh.Charts})
		x.sheetPatch(part).drawing = `<drawing xmlns:r="` + nsRel + `" r:id="rIdDrawing1"/>`
	}
}

// chartParts makes the drawings, charts and chart sheets and adds the
// edits of the parts that list them.
func (x *writer) chartParts(names map[string]bool, edits map[string][]func([]byte) ([]byte, error)) []part {
	if len(x.charts) == 0 && (x.wb.Theme == nil || len(x.wb.Theme.XML) == 0) {
		return nil
	}
	add := func(name string, fn func([]byte) ([]byte, error)) { edits[name] = append(edits[name], fn) }
	if th := x.wb.Theme; th != nil && len(th.XML) > 0 && names["xl/theme/theme1.xml"] {
		// The workbook's own theme, so its charts' theme colours are
		// what they were.
		add("xl/theme/theme1.xml", func([]byte) ([]byte, error) { return th.XML, nil })
	}
	if len(x.charts) == 0 {
		return nil
	}
	used := lowerSet(names)
	unique := func(want string) string {
		if !used[strings.ToLower(want)] {
			used[strings.ToLower(want)] = true
			return want
		}
		dir, file := path.Split(want)
		ext := path.Ext(file)
		stem := strings.TrimRight(strings.TrimSuffix(file, ext), "0123456789")
		for n := 1; ; n++ {
			name := dir + stem + strconv.Itoa(n) + ext
			if !used[strings.ToLower(name)] {
				used[strings.ToLower(name)] = true
				return name
			}
		}
	}
	var parts []part
	var overrides strings.Builder
	override := func(name, ct string) {
		fmt.Fprintf(&overrides, `<Override PartName="/%s" ContentType="%s"/>`, name, ct)
	}
	var wbRels []func([]byte) ([]byte, error)
	for _, pc := range x.charts {
		home := x.wb.Sheets[pc.sheet].Name
		drawing := unique("xl/drawings/drawing1.xml")
		var anchors, drawingRels strings.Builder
		for k, c := range pc.charts {
			chartName := unique("xl/charts/chart1.xml")
			data, rels, extra := x.chartPart(c, home, chartName, unique)
			parts = append(parts, part{chartName, data})
			override(chartName, ctChart)
			if rels != "" {
				parts = append(parts, part{relsName(chartName), []byte(relsXML(rels))})
			}
			for _, e := range extra {
				parts = append(parts, e.part)
				if e.ct != "" {
					override(e.part.name, e.ct)
				}
			}
			rid := "rId" + strconv.Itoa(k+1)
			fmt.Fprintf(&drawingRels, `<Relationship Id="%s" Type="%s" Target="%s"/>`, rid, relChart, relTarget(drawing, chartName))
			a := c.Anchor
			if pc.isSheet {
				// Excel's chart sheet: the chart at the page's size.
				a = model.Anchor{Kind: model.AnchorAbsolute, ExtX: 8666049, ExtY: 6293304}
			}
			anchors.WriteString(anchorXML(a, k+2, c.Name, rid))
		}
		parts = append(parts,
			part{drawing, []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
				`<xdr:wsDr xmlns:xdr="` + nsDrawingSS + `" xmlns:a="` + nsDrawingMain + `">` + anchors.String() + `</xdr:wsDr>`)},
			part{relsName(drawing), []byte(relsXML(drawingRels.String()))})
		override(drawing, ctDrawing)
		drawingRel := fmt.Sprintf(`<Relationship Id="rIdDrawing1" Type="%s" Target="%s"/>`, relDrawing, relTarget(pc.part, drawing))
		if !pc.isSheet {
			add(relsName(pc.part), func(b []byte) ([]byte, error) {
				if b == nil {
					return []byte(relsXML(drawingRel)), nil
				}
				return insertBefore(b, "</Relationships>", drawingRel)
			})
			continue
		}
		// The chart sheet replaces its placeholder worksheet.
		sheetPart := unique("xl/chartsheets/sheet1.xml")
		x.dropParts = append(x.dropParts, pc.part, relsName(pc.part))
		drawingRel = fmt.Sprintf(`<Relationship Id="rId1" Type="%s" Target="%s"/>`, relDrawing, relTarget(sheetPart, drawing))
		parts = append(parts,
			part{sheetPart, []byte(chartsheetXML(x.wb.Sheets[pc.sheet], pc.sheet == x.activeSheet))},
			part{relsName(sheetPart), []byte(relsXML(drawingRel))})
		old, target := strings.TrimPrefix(pc.part, "xl/"), strings.TrimPrefix(sheetPart, "xl/")
		wbRels = append(wbRels, func(b []byte) ([]byte, error) { return retargetSheet(b, old, target) })
		oldPart := "/" + pc.part
		add("[Content_Types].xml", func(b []byte) ([]byte, error) {
			re := regexp.MustCompile(`<Override PartName="` + regexp.QuoteMeta(oldPart) + `"[^>]*?(/>|></Override>)`)
			if !re.Match(b) {
				return nil, fmt.Errorf("no content type for %s", oldPart)
			}
			return re.ReplaceAll(b, []byte(`<Override PartName="/`+sheetPart+`" ContentType="`+ctChartsheet+`"/>`)), nil
		})
	}
	add("[Content_Types].xml", func(b []byte) ([]byte, error) { return insertBefore(b, "</Types>", overrides.String()) })
	for _, fn := range wbRels {
		add("xl/_rels/workbook.xml.rels", fn)
	}
	return parts
}

func lowerSet(names map[string]bool) map[string]bool {
	out := make(map[string]bool, len(names))
	for n := range names {
		out[strings.ToLower(n)] = true
	}
	return out
}

// relsName is the relationships part of a part.
func relsName(name string) string {
	dir, file := path.Split(name)
	return dir + "_rels/" + file + ".rels"
}

// relTarget is target as a relationship of from writes it: relative to
// from's folder.
func relTarget(from, target string) string {
	fd := strings.Split(path.Dir(from), "/")
	td := strings.Split(path.Dir(target), "/")
	i := 0
	for i < len(fd) && i < len(td) && fd[i] == td[i] {
		i++
	}
	rel := strings.Repeat("../", len(fd)-i) + strings.Join(td[i:], "/")
	if rel != "" && !strings.HasSuffix(rel, "/") {
		rel += "/"
	}
	return rel + path.Base(target)
}

// retargetSheet turns a worksheet's relationship in workbook.xml.rels
// into a chart sheet's.
func retargetSheet(b []byte, old, target string) ([]byte, error) {
	re := regexp.MustCompile(`<Relationship [^>]*Target="` + regexp.QuoteMeta(old) + `"[^>]*?(/>|></Relationship>)`)
	m := re.Find(b)
	if m == nil {
		return nil, fmt.Errorf("no relationship for %s", old)
	}
	id := regexp.MustCompile(`Id="([^"]*)"`).FindSubmatch(m)
	if id == nil {
		return nil, fmt.Errorf("relationship for %s has no id", old)
	}
	rel := fmt.Sprintf(`<Relationship Id="%s" Type="%s" Target="%s"/>`, id[1], relChartsheet, target)
	return bytes.Replace(b, m, []byte(rel), 1), nil
}

func chartsheetXML(sh *model.Sheet, active bool) string {
	sel := ""
	if active {
		sel = ` tabSelected="1"`
	}
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<chartsheet xmlns="` + nsMain + `" xmlns:r="` + nsRel + `"><sheetPr/><sheetViews><sheetView` + sel +
		` zoomToFit="1" workbookViewId="0"/></sheetViews>` +
		`<pageMargins left="0.7" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/>` +
		`<drawing r:id="rId1"/></chartsheet>`
}

// anchorXML writes a drawing anchor holding a chart's graphic frame.
func anchorXML(a model.Anchor, id int, name, rid string) string {
	if name == "" {
		name = "Chart " + strconv.Itoa(id-1)
	}
	marker := func(tag string, p model.CellOffset) string {
		return fmt.Sprintf(`<xdr:%s><xdr:col>%d</xdr:col><xdr:colOff>%d</xdr:colOff><xdr:row>%d</xdr:row><xdr:rowOff>%d</xdr:rowOff></xdr:%s>`,
			tag, p.Col, p.DX, p.Row, p.DY, tag)
	}
	frame := fmt.Sprintf(`<xdr:graphicFrame macro=""><xdr:nvGraphicFramePr><xdr:cNvPr id="%d" name="%s"/><xdr:cNvGraphicFramePr/></xdr:nvGraphicFramePr>`+
		`<xdr:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></xdr:xfrm><a:graphic><a:graphicData uri="%s">`+
		`<c:chart xmlns:c="%s" xmlns:r="%s" r:id="%s"/></a:graphicData></a:graphic></xdr:graphicFrame><xdr:clientData/>`,
		id, xmlEscape(name), chartGraphicNS, chartGraphicNS, nsRel, rid)
	ext := fmt.Sprintf(`<xdr:ext cx="%d" cy="%d"/>`, max(a.ExtX, 0), max(a.ExtY, 0))
	switch a.Kind {
	case model.AnchorOneCell:
		return `<xdr:oneCellAnchor>` + marker("from", a.From) + ext + frame + `</xdr:oneCellAnchor>`
	case model.AnchorAbsolute:
		return fmt.Sprintf(`<xdr:absoluteAnchor><xdr:pos x="%d" y="%d"/>`, a.PosX, a.PosY) + ext + frame + `</xdr:absoluteAnchor>`
	}
	edit := ""
	if a.EditAs == "oneCell" || a.EditAs == "absolute" {
		edit = ` editAs="` + a.EditAs + `"`
	}
	return `<xdr:twoCellAnchor` + edit + `>` + marker("from", a.From) + marker("to", a.To) + frame + `</xdr:twoCellAnchor>`
}

// extraPart is a part a chart uses, with its content type.
type extraPart struct {
	part part
	ct   string
}

// chartPart returns a chart part's XML, its relationships and the parts
// they point at.
func (x *writer) chartPart(c *model.Chart, home, name string, unique func(string) string) ([]byte, string, []extraPart) {
	if c.Raw != nil {
		if data, ok := x.rawChart(c, home); ok {
			rels, extra := keptParts(c.Raw, c.Raw.Rels, name, unique, map[string]string{})
			return data, rels, extra
		}
	}
	return []byte(x.chartXML(c, home)), "", nil
}

// keptParts writes a kept chart's related parts under new names and
// returns the relationships of from pointing at them.
func keptParts(raw *model.ChartRaw, rels []model.RawRel, from string, unique func(string) string, done map[string]string) (string, []extraPart) {
	var b strings.Builder
	var out []extraPart
	for _, rel := range rels {
		target := rel.Target
		mode := ""
		if rel.External {
			mode = ` TargetMode="External"`
		} else {
			p := raw.Parts[normPath(rel.Target)]
			if p == nil {
				continue // a part that couldn't be read: drop the link
			}
			name, ok := done[normPath(rel.Target)]
			if !ok {
				name = unique(rel.Target)
				done[normPath(rel.Target)] = name
				sub, more := keptParts(raw, p.Rels, name, unique, done)
				out = append(out, extraPart{part{name, p.Data}, p.ContentType})
				if sub != "" {
					out = append(out, extraPart{part{relsName(name), []byte(relsXML(sub))}, ""})
				}
				out = append(out, more...)
			}
			target = relTarget(from, name)
		}
		fmt.Fprintf(&b, `<Relationship Id="%s" Type="%s" Target="%s"%s/>`, xmlEscape(rel.ID), xmlEscape(rel.Type), xmlEscape(target), mode)
	}
	return b.String(), out
}

// rawChart is a kept chart part with the model's current references and
// freshly cached values spliced in. ok is false when the part no longer
// lines up with the model (it is then written from the model).
func (x *writer) rawChart(c *model.Chart, home string) ([]byte, bool) {
	raw := c.Raw
	spans, ok := chartRefSpans(raw.XML)
	if !ok || len(spans) != len(raw.Refs) {
		return nil, false
	}
	var b bytes.Buffer
	last := 0
	for i, sp := range spans {
		at := raw.Refs[i]
		if sp.at.Plot != at.Plot || sp.at.Series != at.Series || sp.at.Field != at.Field || sp.end < sp.start {
			return nil, false
		}
		ref, ok := c.RefAt(at)
		if !ok {
			return nil, false
		}
		b.Write(raw.XML[last:sp.start])
		b.WriteString(xmlEscape(formula0(ref)))
		last = sp.end
		if sp.cacheEnd > sp.cacheStart && sp.cacheStart >= sp.end {
			if d := c.DataAt(at); d != nil && at.Plot != -3 && c.PivotSource == "" {
				b.Write(raw.XML[last:sp.cacheStart])
				prefix := elemPrefix(raw.XML[sp.cacheStart:])
				b.WriteString(x.chartCacheXML(d, home, c.PlotVisOnly, sp.cacheNum, prefix))
				last = sp.cacheEnd
			}
		}
	}
	b.Write(raw.XML[last:])
	return b.Bytes(), true
}

// formula0 is a reference as a chart part holds it.
func formula0(ref string) string { return strings.TrimPrefix(ref, "=") }

// elemPrefix is the namespace prefix of the element starting b ("c:").
func elemPrefix(b []byte) string {
	if len(b) < 2 || b[0] != '<' {
		return "c:"
	}
	end := bytes.IndexAny(b, " >/")
	if i := bytes.IndexByte(b[:max(end, 0)], ':'); i > 0 {
		return string(b[1 : i+1])
	}
	return ""
}

// chartCacheXML writes the current values of d as a c:numCache or
// c:strCache.
func (x *writer) chartCacheXML(d *model.ChartData, home string, visibleOnly, num bool, p string) string {
	vals, f := d.Cache, d.Format
	if d.Ref != "" {
		if v, _, st, ok := x.wb.ChartValues(d.Ref, home, visibleOnly); ok {
			vals = v
			if int(st) < len(x.wb.Styles) {
				f = numFmtCode(x.wb.Styles[st])
			}
		}
	}
	var b strings.Builder
	if num {
		if f == "" {
			f = "General"
		}
		fmt.Fprintf(&b, `<%snumCache><%sformatCode>%s</%sformatCode>`, p, p, xmlEscape(f), p)
	} else {
		fmt.Fprintf(&b, `<%sstrCache>`, p)
	}
	fmt.Fprintf(&b, `<%sptCount val="%d"/>`, p, len(vals))
	for i, v := range vals {
		switch {
		case num && v.Kind == model.ValueNumber:
			fmt.Fprintf(&b, `<%spt idx="%d"><%sv>%s</%sv></%spt>`, p, i, p, strconv.FormatFloat(v.Num, 'g', -1, 64), p, p)
		case !num && v.Kind != model.ValueBlank:
			fmt.Fprintf(&b, `<%spt idx="%d"><%sv>%s</%sv></%spt>`, p, i, p, xmlEscape(v.Text), p, p)
		}
	}
	if num {
		fmt.Fprintf(&b, `</%snumCache>`, p)
	} else {
		fmt.Fprintf(&b, `</%sstrCache>`, p)
	}
	return b.String()
}

// numFmtCode is a style's number format code.
func numFmtCode(st model.Style) string {
	if st.NumFmtCode != "" {
		return st.NumFmtCode
	}
	if code, ok := builtinNumFmts[st.NumFmtID]; ok {
		return code
	}
	return "General"
}

var builtinNumFmts = map[int]string{
	0: "General", 1: "0", 2: "0.00", 3: "#,##0", 4: "#,##0.00", 9: "0%", 10: "0.00%", 11: "0.00E+00",
	12: "# ?/?", 13: "# ??/??", 14: "m/d/yyyy", 15: "d-mmm-yy", 16: "d-mmm", 17: "mmm-yy", 18: "h:mm AM/PM",
	19: "h:mm:ss AM/PM", 20: "h:mm", 21: "h:mm:ss", 22: "m/d/yyyy h:mm", 37: "#,##0 ;(#,##0)",
	38: "#,##0 ;[Red](#,##0)", 39: "#,##0.00;(#,##0.00)", 40: "#,##0.00;[Red](#,##0.00)", 45: "mm:ss",
	46: "[h]:mm:ss", 47: "mmss.0", 48: "##0.0E+0", 49: "@",
	5: `"$"#,##0_);\("$"#,##0\)`, 6: `"$"#,##0_);[Red]\("$"#,##0\)`,
	7: `"$"#,##0.00_);\("$"#,##0.00\)`, 8: `"$"#,##0.00_);[Red]\("$"#,##0.00\)`,
}

// --- charts written from the model ---

// Excel 2013+'s default look, written out so Excel draws the chart as
// ExcelLite does.
var (
	defTextColor  = color.NRGBA{R: 0x59, G: 0x59, B: 0x59, A: 255}
	defLineColor  = color.NRGBA{R: 0xD9, G: 0xD9, B: 0xD9, A: 255}
	defLabelColor = color.NRGBA{R: 0x40, G: 0x40, B: 0x40, A: 255}
)

func (x *writer) chartXML(c *model.Chart, home string) string {
	th := x.wb.Theme
	if th == nil {
		th = model.DefaultTheme()
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	b.WriteString(`<c:chartSpace xmlns:c="` + chartGraphicNS + `" xmlns:a="` + nsDrawingMain + `" xmlns:r="` + nsRel + `">`)
	date1904 := "0"
	if x.wb.Date1904 {
		date1904 = "1"
	}
	fmt.Fprintf(&b, `<c:date1904 val="%s"/><c:lang val="en-US"/><c:roundedCorners val="%s"/>`, date1904, bit(c.RoundedCorners))
	style := c.Style
	if style < 1 || style > 48 {
		style = 2
	}
	fmt.Fprintf(&b, `<c:style val="%d"/>`, style)
	b.WriteString(`<c:chart>`)
	if c.Title != nil {
		b.WriteString(x.titleXML(c.Title, home, 14, c.PlotVisOnly))
	}
	fmt.Fprintf(&b, `<c:autoTitleDeleted val="%s"/>`, bit(c.AutoTitleDeleted))
	b.WriteString(`<c:plotArea><c:layout/>`)
	written := map[int]bool{}
	for pi := range c.Plots {
		p := &c.Plots[pi]
		if p.Type == model.ChartOther {
			continue
		}
		b.WriteString(x.plotXML(p, th, home, c.PlotVisOnly))
		for _, id := range p.AxisIDs {
			written[id] = true
		}
	}
	for i := range c.Axes {
		if written[c.Axes[i].ID] {
			b.WriteString(x.axisXML(&c.Axes[i], c, home))
		}
	}
	b.WriteString(spPrXML(c.PlotArea, model.Paint{Kind: model.FillNone}, model.Paint{Kind: model.FillNone}, 0))
	b.WriteString(`</c:plotArea>`)
	if lg := c.Legend; lg != nil {
		pos := lg.Pos
		if !slices.Contains([]string{"b", "l", "r", "t", "tr"}, pos) {
			pos = "r"
		}
		fmt.Fprintf(&b, `<c:legend><c:legendPos val="%s"/>`, pos)
		for _, i := range lg.Deleted {
			fmt.Fprintf(&b, `<c:legendEntry><c:idx val="%d"/><c:delete val="1"/></c:legendEntry>`, i)
		}
		fmt.Fprintf(&b, `<c:overlay val="%s"/>`, bit(lg.Overlay))
		b.WriteString(spPrXML(lg.Shape, model.Paint{Kind: model.FillNone}, model.Paint{Kind: model.FillNone}, 0))
		b.WriteString(txPrXML(lg.Font, 9, defTextColor, nil))
		b.WriteString(`</c:legend>`)
	}
	fmt.Fprintf(&b, `<c:plotVisOnly val="%s"/>`, bit(c.PlotVisOnly))
	fmt.Fprintf(&b, `<c:dispBlanksAs val="%s"/>`, []string{"gap", "zero", "span"}[min(int(c.DispBlanks), 2)])
	b.WriteString(`</c:chart>`)
	b.WriteString(spPrXML(c.ChartArea, model.Paint{Kind: model.FillSolid, Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
		model.Paint{Kind: model.FillSolid, Color: defLineColor}, 0.75))
	b.WriteString(txPrXML(c.Font, 0, color.NRGBA{}, nil))
	b.WriteString(`</c:chartSpace>`)
	return b.String()
}

func bit(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// colorXML writes a solid fill.
func colorXML(c color.NRGBA) string {
	s := fmt.Sprintf(`<a:solidFill><a:srgbClr val="%02X%02X%02X"`, c.R, c.G, c.B)
	if c.A < 255 {
		return s + fmt.Sprintf(`><a:alpha val="%d"/></a:srgbClr></a:solidFill>`, int(c.A)*100000/255)
	}
	return s + `/></a:solidFill>`
}

func paintXML(p model.Paint, def model.Paint) string {
	if p.Kind == model.FillAuto {
		p = def
	}
	switch p.Kind {
	case model.FillNone:
		return `<a:noFill/>`
	case model.FillSolid:
		return colorXML(p.Color)
	}
	return ""
}

// spPrXML writes a shape's format, with defaults for what is automatic
// (FillAuto defaults write nothing).
func spPrXML(sp model.ShapeProps, defFill, defLine model.Paint, defWidth float32) string {
	fill := paintXML(sp.Fill, defFill)
	line := paintXML(sp.Line, defLine)
	w := sp.Width
	if w <= 0 {
		w = defWidth
	}
	if fill == "" && line == "" && sp.Dash == "" && sp.Width <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<c:spPr>`)
	b.WriteString(fill)
	if line != "" || sp.Dash != "" || w > 0 {
		b.WriteString(`<a:ln`)
		if w > 0 {
			fmt.Fprintf(&b, ` w="%d"`, int(w*12700+0.5))
		}
		b.WriteString(`>`)
		b.WriteString(line)
		if sp.Dash != "" {
			fmt.Fprintf(&b, `<a:prstDash val="%s"/>`, xmlEscape(sp.Dash))
		}
		b.WriteString(`<a:round/></a:ln>`)
	}
	b.WriteString(`</c:spPr>`)
	return b.String()
}

// txPrXML writes a text format; defSize (points) and defColor fill what
// t leaves unset (0: leave it unset). rot overrides t's rotation.
func txPrXML(t model.TextProps, defSize float32, defColor color.NRGBA, rot *int) string {
	size := t.SizePt
	if size <= 0 {
		size = defSize
	}
	col := defColor
	if t.Color.Kind == model.FillSolid {
		col = t.Color.Color
	}
	if size <= 0 && col.A == 0 && t.Bold == nil && !t.Italic && t.Font == "" && !t.HasRot && rot == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<c:txPr><a:bodyPr`)
	switch {
	case rot != nil:
		fmt.Fprintf(&b, ` rot="%d" vert="horz"`, *rot*60000)
	case t.HasRot:
		fmt.Fprintf(&b, ` rot="%d" vert="horz"`, t.Rotation*60000)
	}
	b.WriteString(`/><a:lstStyle/><a:p><a:pPr>`)
	b.WriteString(defRPrXML("defRPr", t, size, col))
	b.WriteString(`</a:pPr><a:endParaRPr lang="en-US"/></a:p></c:txPr>`)
	return b.String()
}

func defRPrXML(tag string, t model.TextProps, size float32, col color.NRGBA) string {
	var b strings.Builder
	b.WriteString(`<a:` + tag)
	if tag == "rPr" {
		b.WriteString(` lang="en-US"`)
	}
	if size > 0 {
		fmt.Fprintf(&b, ` sz="%d"`, int(size*100+0.5))
	}
	if t.Bold != nil {
		fmt.Fprintf(&b, ` b="%s"`, bit(*t.Bold))
	}
	if t.Italic {
		b.WriteString(` i="1"`)
	}
	b.WriteString(`>`)
	if col.A > 0 {
		b.WriteString(colorXML(col))
	}
	if t.Font != "" {
		fmt.Fprintf(&b, `<a:latin typeface="%s"/>`, xmlEscape(t.Font))
	}
	b.WriteString(`</a:` + tag + `>`)
	return b.String()
}

func (x *writer) titleXML(t *model.ChartTitle, home string, defSize float32, visibleOnly bool) string {
	var b strings.Builder
	b.WriteString(`<c:title>`)
	bold := false
	f := t.Font
	if f.Bold == nil {
		f.Bold = &bold
	}
	switch {
	case t.Text != "":
		b.WriteString(`<c:tx><c:rich><a:bodyPr`)
		if t.Font.HasRot {
			fmt.Fprintf(&b, ` rot="%d" vert="horz"`, t.Font.Rotation*60000)
		}
		b.WriteString(`/><a:lstStyle/>`)
		col := defTextColor
		if f.Color.Kind == model.FillSolid {
			col = f.Color.Color
		}
		size := f.SizePt
		if size <= 0 {
			size = defSize
		}
		for _, line := range strings.Split(t.Text, "\n") {
			b.WriteString(`<a:p><a:pPr>` + defRPrXML("defRPr", f, size, col) + `</a:pPr>`)
			b.WriteString(`<a:r>` + defRPrXML("rPr", f, size, col) + `<a:t>` + xmlEscape(line) + `</a:t></a:r></a:p>`)
		}
		b.WriteString(`</c:rich></c:tx>`)
	case t.Ref.Ref != "":
		fmt.Fprintf(&b, `<c:tx><c:strRef><c:f>%s</c:f>%s</c:strRef></c:tx>`, xmlEscape(formula0(t.Ref.Ref)), x.chartCacheXML(&t.Ref, home, visibleOnly, false, "c:"))
	}
	fmt.Fprintf(&b, `<c:overlay val="%s"/>`, bit(t.Overlay))
	b.WriteString(spPrXML(t.Shape, model.Paint{Kind: model.FillNone}, model.Paint{Kind: model.FillNone}, 0))
	if t.Text == "" {
		var rot *int
		if t.Font.HasRot {
			r := t.Font.Rotation
			rot = &r
		}
		b.WriteString(txPrXML(f, defSize, defTextColor, rot))
	}
	b.WriteString(`</c:title>`)
	return b.String()
}

// dataXML writes a series' name, categories or values.
func (x *writer) dataXML(tag string, d *model.ChartData, home string, visibleOnly bool, num bool) string {
	if d.IsZero() {
		return ""
	}
	if d.Ref == "" {
		// Literal values.
		if tag == "tx" {
			return `<c:tx><c:v>` + xmlEscape(d.Literal) + `</c:v></c:tx>`
		}
		var b strings.Builder
		lit := "strLit"
		if num {
			lit = "numLit"
		}
		fmt.Fprintf(&b, `<c:%s><c:%s>`, tag, lit)
		if num {
			f := d.Format
			if f == "" {
				f = "General"
			}
			b.WriteString(`<c:formatCode>` + xmlEscape(f) + `</c:formatCode>`)
		}
		fmt.Fprintf(&b, `<c:ptCount val="%d"/>`, len(d.Cache))
		for i, v := range d.Cache {
			if v.Kind == model.ValueBlank {
				continue
			}
			t := v.Text
			if num {
				t = strconv.FormatFloat(v.Num, 'g', -1, 64)
			}
			fmt.Fprintf(&b, `<c:pt idx="%d"><c:v>%s</c:v></c:pt>`, i, xmlEscape(t))
		}
		fmt.Fprintf(&b, `</c:%s></c:%s>`, lit, tag)
		return b.String()
	}
	ref := "strRef"
	if num {
		ref = "numRef"
	}
	return fmt.Sprintf(`<c:%s><c:%s><c:f>%s</c:f>%s</c:%s></c:%s>`, tag, ref, xmlEscape(formula0(d.Ref)),
		x.chartCacheXML(d, home, visibleOnly, num, "c:"), ref, tag)
}

// numericData reports whether data's current values are all numbers (or
// blank), so it's written as numbers.
func (x *writer) numericData(d *model.ChartData, home string) bool {
	vals := x.wb.SeriesData(d, home, false)
	any := false
	for _, v := range vals {
		switch v.Kind {
		case model.ValueText:
			return false
		case model.ValueNumber:
			any = true
		}
	}
	return any || !d.Text
}

// validLabelPos lists the data label positions each chart type allows;
// Excel refuses a file with any other.
func validLabelPos(p *model.ChartPlot, pos string) bool {
	switch p.Type {
	case model.ChartBar:
		if p.Grouping != model.GroupClustered {
			return slices.Contains([]string{"ctr", "inEnd", "inBase"}, pos)
		}
		return slices.Contains([]string{"ctr", "inEnd", "inBase", "outEnd"}, pos)
	case model.ChartLine, model.ChartScatter:
		return slices.Contains([]string{"ctr", "l", "r", "t", "b"}, pos)
	case model.ChartPie:
		return slices.Contains([]string{"ctr", "inEnd", "outEnd", "bestFit"}, pos)
	}
	return false
}

func dLblsXML(lb *model.DataLabels, p *model.ChartPlot) string {
	if lb == nil {
		return ""
	}
	if lb.Delete {
		return `<c:dLbls><c:delete val="1"/></c:dLbls>`
	}
	var b strings.Builder
	b.WriteString(`<c:dLbls>`)
	if lb.NumFmt != "" {
		fmt.Fprintf(&b, `<c:numFmt formatCode="%s" sourceLinked="0"/>`, xmlEscape(lb.NumFmt))
	}
	b.WriteString(spPrXML(lb.Shape, model.Paint{Kind: model.FillNone}, model.Paint{Kind: model.FillNone}, 0))
	b.WriteString(txPrXML(lb.Font, 9, defLabelColor, nil))
	if lb.Pos != "" && validLabelPos(p, lb.Pos) {
		fmt.Fprintf(&b, `<c:dLblPos val="%s"/>`, lb.Pos)
	}
	fmt.Fprintf(&b, `<c:showLegendKey val="%s"/><c:showVal val="%s"/><c:showCatName val="%s"/><c:showSerName val="%s"/><c:showPercent val="%s"/><c:showBubbleSize val="0"/>`,
		bit(lb.ShowKey), bit(lb.ShowVal), bit(lb.ShowCat), bit(lb.ShowSer), bit(lb.ShowPercent))
	if lb.Separator != "" {
		b.WriteString(`<c:separator>` + xmlEscape(lb.Separator) + `</c:separator>`)
	}
	if p.Type == model.ChartPie || p.Type == model.ChartDoughnut {
		fmt.Fprintf(&b, `<c:showLeaderLines val="%s"/>`, bit(lb.ShowLeader))
	}
	b.WriteString(`</c:dLbls>`)
	return b.String()
}

func (x *writer) plotXML(p *model.ChartPlot, th *model.Theme, home string, visibleOnly bool) string {
	var b strings.Builder
	grouping := []string{"standard", "stacked", "percentStacked"}[min(int(p.Grouping), 2)]
	tag := map[model.ChartType]string{model.ChartBar: "barChart", model.ChartLine: "lineChart", model.ChartPie: "pieChart",
		model.ChartDoughnut: "doughnutChart", model.ChartArea: "areaChart", model.ChartScatter: "scatterChart"}[p.Type]
	b.WriteString(`<c:` + tag + `>`)
	switch p.Type {
	case model.ChartBar:
		dir := "col"
		if p.Horizontal {
			dir = "bar"
		}
		if p.Grouping == model.GroupClustered {
			grouping = "clustered"
		}
		fmt.Fprintf(&b, `<c:barDir val="%s"/><c:grouping val="%s"/>`, dir, grouping)
	case model.ChartLine, model.ChartArea:
		fmt.Fprintf(&b, `<c:grouping val="%s"/>`, grouping)
	case model.ChartScatter:
		st := p.ScatterStyle
		if st == "" {
			st = "lineMarker"
		}
		fmt.Fprintf(&b, `<c:scatterStyle val="%s"/>`, st)
	}
	fmt.Fprintf(&b, `<c:varyColors val="%s"/>`, bit(p.VaryColors))
	for i := range p.Series {
		b.WriteString(x.seriesXML(p, &p.Series[i], th, home, visibleOnly))
	}
	b.WriteString(dLblsXML(p.Labels, p))
	switch p.Type {
	case model.ChartBar:
		ov := p.Overlap
		if p.Grouping != model.GroupClustered {
			ov = 100
		}
		fmt.Fprintf(&b, `<c:gapWidth val="%d"/>`, min(max(p.GapWidth, 0), 500))
		if ov != 0 {
			fmt.Fprintf(&b, `<c:overlap val="%d"/>`, min(max(ov, -100), 100))
		}
	case model.ChartLine:
		fmt.Fprintf(&b, `<c:marker val="%s"/>`, bit(p.Marker))
	case model.ChartPie, model.ChartDoughnut:
		fmt.Fprintf(&b, `<c:firstSliceAng val="%d"/>`, ((p.FirstSlice%360)+360)%360)
		if p.Type == model.ChartDoughnut {
			fmt.Fprintf(&b, `<c:holeSize val="%d"/>`, min(max(p.HoleSize, 10), 90))
		}
	}
	if p.Type != model.ChartPie && p.Type != model.ChartDoughnut {
		for _, id := range p.AxisIDs {
			fmt.Fprintf(&b, `<c:axId val="%d"/>`, id)
		}
	}
	b.WriteString(`</c:` + tag + `>`)
	return b.String()
}

func (x *writer) seriesXML(p *model.ChartPlot, s *model.ChartSeries, th *model.Theme, home string, visibleOnly bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<c:ser><c:idx val="%d"/><c:order val="%d"/>`, s.Idx, s.Order)
	b.WriteString(x.dataXML("tx", &s.Name, home, visibleOnly, false))
	auto := th.SeriesColor(s.Idx)
	solid := func(c color.NRGBA) model.Paint { return model.Paint{Kind: model.FillSolid, Color: c} }
	none := model.Paint{Kind: model.FillNone}
	switch p.Type {
	case model.ChartLine, model.ChartScatter:
		lineDef := solid(auto)
		if s.Shape.Fill.Kind == model.FillSolid {
			lineDef = s.Shape.Fill
		}
		sp := s.Shape
		sp.Fill = model.Paint{}
		b.WriteString(spPrXML(sp, model.Paint{}, lineDef, 2.25))
	case model.ChartPie, model.ChartDoughnut:
		if !p.VaryColors {
			b.WriteString(spPrXML(s.Shape, solid(auto), solid(color.NRGBA{R: 255, G: 255, B: 255, A: 255}), 1))
		} else {
			b.WriteString(spPrXML(s.Shape, model.Paint{}, solid(color.NRGBA{R: 255, G: 255, B: 255, A: 255}), 1))
		}
	default:
		b.WriteString(spPrXML(s.Shape, solid(auto), none, 0))
	}
	if p.Type == model.ChartBar {
		fmt.Fprintf(&b, `<c:invertIfNegative val="%s"/>`, bit(s.InvertNeg))
	}
	if p.Type == model.ChartLine || p.Type == model.ChartScatter {
		m := s.Marker
		if p.Type == model.ChartLine && !p.Marker && m.Symbol == "" {
			m.Symbol = "none" // a plain line: no automatic markers in Excel either
		}
		b.WriteString(markerXML(m, auto))
	}
	if (p.Type == model.ChartPie || p.Type == model.ChartDoughnut) && s.Explosion > 0 {
		fmt.Fprintf(&b, `<c:explosion val="%d"/>`, s.Explosion)
	}
	// Points: their own formats, and a varied-colour pie's slices made
	// explicit.
	pts := slices.Clone(s.Points)
	if p.VaryColors && (p.Type == model.ChartPie || p.Type == model.ChartDoughnut) {
		vals := x.wb.SeriesData(&s.Val, home, visibleOnly)
		for i := range vals {
			if !slices.ContainsFunc(pts, func(d model.DataPoint) bool { return d.Idx == i }) {
				pts = append(pts, model.DataPoint{Idx: i, Shape: model.ShapeProps{Fill: solid(th.SeriesColor(i))}})
			}
		}
		slices.SortFunc(pts, func(a, b model.DataPoint) int { return a.Idx - b.Idx })
	}
	for _, d := range pts {
		fmt.Fprintf(&b, `<c:dPt><c:idx val="%d"/>`, d.Idx)
		switch p.Type {
		case model.ChartBar:
			fmt.Fprintf(&b, `<c:invertIfNegative val="%s"/>`, bit(s.InvertNeg))
			b.WriteString(`<c:bubble3D val="0"/>`)
		case model.ChartPie, model.ChartDoughnut:
			b.WriteString(`<c:bubble3D val="0"/>`)
			if d.Explosion > 0 {
				fmt.Fprintf(&b, `<c:explosion val="%d"/>`, d.Explosion)
			}
		}
		b.WriteString(spPrXML(d.Shape, model.Paint{}, model.Paint{}, 0))
		b.WriteString(`</c:dPt>`)
	}
	b.WriteString(dLblsXML(s.Labels, p))
	if p.Type == model.ChartScatter {
		b.WriteString(x.dataXML("xVal", &s.Cat, home, visibleOnly, x.numericData(&s.Cat, home)))
		b.WriteString(x.dataXML("yVal", &s.Val, home, visibleOnly, true))
		fmt.Fprintf(&b, `<c:smooth val="%s"/>`, bit(s.Smooth))
	} else {
		b.WriteString(x.dataXML("cat", &s.Cat, home, visibleOnly, x.numericData(&s.Cat, home) && !s.Cat.Text))
		b.WriteString(x.dataXML("val", &s.Val, home, visibleOnly, true))
		if p.Type == model.ChartLine {
			fmt.Fprintf(&b, `<c:smooth val="%s"/>`, bit(s.Smooth))
		}
	}
	b.WriteString(`</c:ser>`)
	return b.String()
}

func markerXML(m model.Marker, auto color.NRGBA) string {
	if m.Symbol == "" || m.Symbol == "auto" {
		if m.Size == 0 && m.Shape == (model.ShapeProps{}) {
			return ""
		}
	}
	var b strings.Builder
	b.WriteString(`<c:marker>`)
	if m.Symbol != "" {
		fmt.Fprintf(&b, `<c:symbol val="%s"/>`, xmlEscape(m.Symbol))
	}
	if m.Size > 0 {
		fmt.Fprintf(&b, `<c:size val="%d"/>`, min(max(m.Size, 2), 72))
	}
	if m.Symbol != "none" {
		b.WriteString(spPrXML(m.Shape, model.Paint{Kind: model.FillSolid, Color: auto}, model.Paint{Kind: model.FillSolid, Color: auto}, 0.75))
	}
	b.WriteString(`</c:marker>`)
	return b.String()
}

func (x *writer) axisXML(a *model.ChartAxis, c *model.Chart, home string) string {
	var b strings.Builder
	tag := []string{"catAx", "valAx", "dateAx", "serAx"}[min(int(a.Kind), 3)]
	if a.Kind == model.AxisSer {
		tag = "catAx"
	}
	fmt.Fprintf(&b, `<c:%s><c:axId val="%d"/><c:scaling>`, tag, a.ID)
	if a.LogBase >= 2 && a.Kind == model.AxisVal {
		fmt.Fprintf(&b, `<c:logBase val="%s"/>`, num(a.LogBase))
	}
	orient := "minMax"
	if a.Reversed {
		orient = "maxMin"
	}
	fmt.Fprintf(&b, `<c:orientation val="%s"/>`, orient)
	if a.HasMax {
		fmt.Fprintf(&b, `<c:max val="%s"/>`, num(a.Max))
	}
	if a.HasMin {
		fmt.Fprintf(&b, `<c:min val="%s"/>`, num(a.Min))
	}
	b.WriteString(`</c:scaling>`)
	pos := a.Pos
	if !slices.Contains([]string{"b", "l", "r", "t"}, pos) {
		pos = "b"
	}
	fmt.Fprintf(&b, `<c:delete val="%s"/><c:axPos val="%s"/>`, bit(a.Delete), pos)
	grid := func(tag string, sp model.ShapeProps) {
		fmt.Fprintf(&b, `<c:%s>%s</c:%s>`, tag, spPrXML(sp, model.Paint{}, model.Paint{Kind: model.FillSolid, Color: defLineColor}, 0.75), tag)
	}
	if a.MajorGrid {
		grid("majorGridlines", a.MajorGridLine)
	}
	if a.MinorGrid {
		grid("minorGridlines", a.MinorGridLine)
	}
	if a.Title != nil {
		b.WriteString(x.titleXML(a.Title, home, 10, c.PlotVisOnly))
	}
	nf := a.NumFmt
	if nf == "" {
		nf = "General"
	}
	fmt.Fprintf(&b, `<c:numFmt formatCode="%s" sourceLinked="%s"/>`, xmlEscape(nf), bit(a.SourceLinked || a.NumFmt == ""))
	tick := func(v string) string {
		if !slices.Contains([]string{"none", "in", "out", "cross"}, v) {
			return "none"
		}
		return v
	}
	fmt.Fprintf(&b, `<c:majorTickMark val="%s"/><c:minorTickMark val="%s"/>`, tick(a.MajorTick), tick(a.MinorTick))
	lp := a.TickLblPos
	if !slices.Contains([]string{"high", "low", "nextTo", "none"}, lp) {
		lp = "nextTo"
	}
	fmt.Fprintf(&b, `<c:tickLblPos val="%s"/>`, lp)
	lineDef := model.Paint{Kind: model.FillNone}
	if a.Kind != model.AxisVal {
		lineDef = model.Paint{Kind: model.FillSolid, Color: defLineColor}
	}
	b.WriteString(spPrXML(a.Line, model.Paint{Kind: model.FillNone}, lineDef, 0.75))
	var rot *int
	if a.Font.HasRot {
		r := a.Font.Rotation
		rot = &r
	}
	b.WriteString(txPrXML(a.Font, 9, defTextColor, rot))
	fmt.Fprintf(&b, `<c:crossAx val="%d"/>`, a.CrossAx)
	if a.HasCrossesAt {
		fmt.Fprintf(&b, `<c:crossesAt val="%s"/>`, num(a.CrossesAt))
	} else {
		cr := a.Crosses
		if !slices.Contains([]string{"autoZero", "min", "max"}, cr) {
			cr = "autoZero"
		}
		fmt.Fprintf(&b, `<c:crosses val="%s"/>`, cr)
	}
	switch a.Kind {
	case model.AxisVal:
		cb := a.CrossBetween
		if cb != "midCat" {
			cb = "between"
		}
		fmt.Fprintf(&b, `<c:crossBetween val="%s"/>`, cb)
		if a.MajorUnit > 0 {
			fmt.Fprintf(&b, `<c:majorUnit val="%s"/>`, num(a.MajorUnit))
		}
		if a.MinorUnit > 0 {
			fmt.Fprintf(&b, `<c:minorUnit val="%s"/>`, num(a.MinorUnit))
		}
	case model.AxisDate:
		b.WriteString(`<c:auto val="1"/><c:lblOffset val="100"/><c:baseTimeUnit val="days"/>`)
	default:
		b.WriteString(`<c:auto val="1"/><c:lblAlgn val="ctr"/><c:lblOffset val="100"/>`)
		if a.LblSkip > 0 {
			fmt.Fprintf(&b, `<c:tickLblSkip val="%d"/>`, a.LblSkip)
		}
		if a.TickSkip > 0 {
			fmt.Fprintf(&b, `<c:tickMarkSkip val="%d"/>`, a.TickSkip)
		}
		fmt.Fprintf(&b, `<c:noMultiLvlLbl val="%s"/>`, bit(a.NoMultiLvl))
	}
	fmt.Fprintf(&b, `</c:%s>`, tag)
	return b.String()
}
