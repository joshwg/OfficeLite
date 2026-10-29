package xlsxfile

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"officelite/excel/model"
	"officelite/excel/numfmt"
	"officelite/excel/pivot"
)

// Writing PivotTables: for each report, a pivotCacheDefinition with its
// source and every field's items, the cache records, and the
// pivotTableDefinition (fields, axes, the row and column items as laid
// out, value fields, report filters), with the relationships, content
// types and the workbook's <pivotCaches>. The cache asks Excel to
// refresh on load, so Excel rebuilds anything it would have written
// differently.

const (
	relPivotTable   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/pivotTable"
	relPivotCache   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/pivotCacheDefinition"
	relPivotRecords = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/pivotCacheRecords"
	ctPivotTable    = "application/vnd.openxmlformats-officedocument.spreadsheetml.pivotTable+xml"
	ctPivotCache    = "application/vnd.openxmlformats-officedocument.spreadsheetml.pivotCacheDefinition+xml"
	ctPivotRecords  = "application/vnd.openxmlformats-officedocument.spreadsheetml.pivotCacheRecords+xml"
	nsMain          = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	nsRel           = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsPkgRel        = "http://schemas.openxmlformats.org/package/2006/relationships"
)

// pendingPivot is a report to write, with its sheet's worksheet part.
type pendingPivot struct {
	sheet int
	part  string
	pt    *model.PivotTable
}

// part is a package part written whole.
type part struct {
	name string
	data []byte
}

// collectPivots lists the reports of sheet i whose source can be read.
func (x *writer) collectPivots(i int, sh *model.Sheet, part string) {
	for _, pt := range sh.Pivots {
		if _, _, ok := pivot.ResolveSource(x.wb, i, pt.Source); ok {
			x.pivots = append(x.pivots, pendingPivot{i, part, pt})
		}
	}
}

// placeholder reports whether row, col is in an empty report's box,
// whose text Excel draws itself and isn't saved.
func placeholder(sh *model.Sheet, row, col int) bool {
	for _, p := range sh.Pivots {
		if pivot.Empty(p) && p.Out.Body.Contains(row, col) {
			return true
		}
	}
	return false
}

// pivotParts makes the reports' parts and adds the edits of the parts
// that list them.
func (x *writer) pivotParts(zr *zip.Reader, edits map[string][]func([]byte) ([]byte, error)) ([]part, error) {
	if len(x.pivots) == 0 {
		return nil, nil
	}
	fmtIDs := map[string]int{}
	for _, zf := range zr.File {
		if zf.Name == "xl/styles.xml" {
			rc, err := zf.Open()
			if err != nil {
				return nil, err
			}
			b, _ := io.ReadAll(rc)
			rc.Close()
			for _, m := range numFmtRe.FindAllSubmatch(b, -1) {
				id, _ := strconv.Atoi(string(m[1]))
				fmtIDs[xmlUnescape(string(m[2]))] = id
			}
		}
	}
	var parts []part
	var overrides, wbRels, caches strings.Builder
	sheetRels := map[string][]string{} // worksheet part -> relationships
	for k, pp := range x.pivots {
		n := k + 1
		sheetIdx, rng, _ := pivot.ResolveSource(x.wb, pp.sheet, pp.pt.Source)
		src, err := pivot.ReadSource(x.wb.Sheets[sheetIdx], sheetIdx, rng, x.isDate)
		if err != nil {
			continue
		}
		pt := pp.pt
		cacheXML, recordsXML := x.cacheXML(pt, src, sheetIdx, rng)
		tableXML := x.tableXML(pt, src, n, fmtIDs)
		cachePart := fmt.Sprintf("xl/pivotCache/pivotCacheDefinition%d.xml", n)
		parts = append(parts,
			part{cachePart, []byte(cacheXML)},
			part{fmt.Sprintf("xl/pivotCache/pivotCacheRecords%d.xml", n), []byte(recordsXML)},
			part{fmt.Sprintf("xl/pivotCache/_rels/pivotCacheDefinition%d.xml.rels", n), []byte(relsXML(
				fmt.Sprintf(`<Relationship Id="rId1" Type="%s" Target="pivotCacheRecords%d.xml"/>`, relPivotRecords, n)))},
			part{fmt.Sprintf("xl/pivotTables/pivotTable%d.xml", n), []byte(tableXML)},
			part{fmt.Sprintf("xl/pivotTables/_rels/pivotTable%d.xml.rels", n), []byte(relsXML(
				fmt.Sprintf(`<Relationship Id="rId1" Type="%s" Target="../pivotCache/pivotCacheDefinition%d.xml"/>`, relPivotCache, n)))},
		)
		fmt.Fprintf(&overrides, `<Override PartName="/xl/pivotCache/pivotCacheDefinition%d.xml" ContentType="%s"/>`, n, ctPivotCache)
		fmt.Fprintf(&overrides, `<Override PartName="/xl/pivotCache/pivotCacheRecords%d.xml" ContentType="%s"/>`, n, ctPivotRecords)
		fmt.Fprintf(&overrides, `<Override PartName="/xl/pivotTables/pivotTable%d.xml" ContentType="%s"/>`, n, ctPivotTable)
		fmt.Fprintf(&wbRels, `<Relationship Id="rIdPivotCache%d" Type="%s" Target="pivotCache/pivotCacheDefinition%d.xml"/>`, n, relPivotCache, n)
		fmt.Fprintf(&caches, `<pivotCache cacheId="%d" r:id="rIdPivotCache%d"/>`, n, n)
		sheetRels[pp.part] = append(sheetRels[pp.part],
			fmt.Sprintf(`<Relationship Id="rIdPivot%d" Type="%s" Target="../pivotTables/pivotTable%d.xml"/>`, n, relPivotTable, n))
	}
	if caches.Len() == 0 {
		return nil, nil
	}
	add := func(name string, fn func([]byte) ([]byte, error)) { edits[name] = append(edits[name], fn) }
	add("[Content_Types].xml", func(b []byte) ([]byte, error) { return insertBefore(b, "</Types>", overrides.String()) })
	add("xl/_rels/workbook.xml.rels", func(b []byte) ([]byte, error) { return insertBefore(b, "</Relationships>", wbRels.String()) })
	add("xl/workbook.xml", func(b []byte) ([]byte, error) {
		return addPivotCaches(b, "<pivotCaches>"+caches.String()+"</pivotCaches>")
	})
	for sp, rels := range sheetRels {
		name := strings.Replace(sp, "worksheets/", "worksheets/_rels/", 1) + ".rels"
		all := strings.Join(rels, "")
		add(name, func(b []byte) ([]byte, error) {
			if b == nil {
				return []byte(relsXML(all)), nil
			}
			return insertBefore(b, "</Relationships>", all)
		})
	}
	return parts, nil
}

var numFmtRe = regexp.MustCompile(`<numFmt numFmtId="(\d+)" formatCode="([^"]*)"`)

func xmlUnescape(s string) string {
	r := strings.NewReplacer("&quot;", `"`, "&apos;", "'", "&lt;", "<", "&gt;", ">", "&amp;", "&")
	return r.Replace(s)
}

func relsXML(rels string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<Relationships xmlns="` + nsPkgRel + `">` + rels + `</Relationships>`
}

// addPivotCaches puts <pivotCaches> where CT_Workbook has it: after
// sheets, definedNames, calcPr, oleSize and customWorkbookViews.
func addPivotCaches(b []byte, caches string) ([]byte, error) {
	at := -1
	for _, end := range []string{"</customWorkbookViews>", "<oleSize", "<calcPr", "</definedNames>", "</sheets>"} {
		i := bytes.Index(b, []byte(end))
		if i < 0 {
			continue
		}
		if strings.HasPrefix(end, "</") {
			at = i + len(end)
		} else {
			j := bytes.IndexByte(b[i:], '>')
			if j < 0 {
				continue
			}
			at = i + j + 1
			if b[i+j-1] != '/' { // <calcPr ...>...</calcPr>
				k := bytes.Index(b[i:], []byte("</"+end[1:]+">"))
				if k >= 0 {
					at = i + k + len(end) + 2
				}
			}
		}
		break
	}
	if at < 0 {
		return nil, fmt.Errorf("no sheets")
	}
	return slices.Concat(b[:at], []byte(caches), b[at:]), nil
}

func (x *writer) isDate(i uint32) bool {
	if int(i) >= len(x.wb.Styles) {
		return false
	}
	st := x.wb.Styles[i]
	return numfmt.IsDate(st.NumFmtID, st.NumFmtCode)
}

// fieldItems describes the items of a source field for <sharedItems>.
type fieldItems struct {
	str, num, blank, boolean, errs, date, nonInt bool
	min, max                                     float64
}

func itemKinds(f *pivot.Field) fieldItems {
	var k fieldItems
	first := true
	for _, it := range f.Items {
		switch {
		case it.Key == "":
			k.blank = true
		case it.Kind == model.KindNumber:
			k.num = true
			if it.Num != math.Trunc(it.Num) {
				k.nonInt = true
			}
			if first {
				k.min, k.max, first = it.Num, it.Num, false
			}
			k.min, k.max = min(k.min, it.Num), max(k.max, it.Num)
		case it.Kind == model.KindText:
			k.str = true
		case it.Kind == model.KindBool:
			k.boolean = true
		default:
			k.errs = true
		}
	}
	k.date = f.Date && k.num && !k.str && !k.boolean && !k.errs
	return k
}

func (x *writer) cacheXML(pt *model.PivotTable, src *pivot.Source, sheet int, rng model.Range) (string, string) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	fmt.Fprintf(&b, `<pivotCacheDefinition xmlns="%s" xmlns:r="%s" r:id="rId1" refreshOnLoad="1" createdVersion="6" refreshedVersion="6" minRefreshableVersion="3" recordCount="%d">`, nsMain, nsRel, src.N)
	b.WriteString(`<cacheSource type="worksheet"><worksheetSource`)
	if _, _, ok := pivot.SplitSource(pt.Source); ok {
		fmt.Fprintf(&b, ` ref="%s" sheet="%s"`, model.CellName(rng.R1, rng.C1)+":"+model.CellName(rng.R2, rng.C2), xmlEscape(x.sheetName(sheet)))
	} else {
		fmt.Fprintf(&b, ` name="%s"`, xmlEscape(strings.TrimPrefix(pt.Source, "=")))
	}
	b.WriteString(`/></cacheSource>`)
	fmt.Fprintf(&b, `<cacheFields count="%d">`, len(src.Fields))
	for _, f := range src.Fields {
		k := itemKinds(f)
		numFmt := 0
		if k.date {
			numFmt = 14
		}
		fmt.Fprintf(&b, `<cacheField name="%s" numFmtId="%d"><sharedItems`, xmlEscape(f.Name), numFmt)
		mixed := 0
		for _, t := range []bool{k.str, k.num, k.boolean, k.errs} {
			if t {
				mixed++
			}
		}
		if !k.str && !k.blank && !k.boolean && !k.errs {
			b.WriteString(` containsSemiMixedTypes="0"`)
		}
		if !k.str {
			b.WriteString(` containsString="0"`)
		}
		if k.blank {
			b.WriteString(` containsBlank="1"`)
		}
		if mixed > 1 {
			b.WriteString(` containsMixedTypes="1"`)
		}
		switch {
		case k.date:
			fmt.Fprintf(&b, ` containsNonDate="0" containsDate="1" minDate="%s" maxDate="%s"`, x.isoDate(k.min), x.isoDate(k.max))
		case k.num:
			b.WriteString(` containsNumber="1"`)
			if !k.nonInt {
				b.WriteString(` containsInteger="1"`)
			}
			fmt.Fprintf(&b, ` minValue="%s" maxValue="%s"`, num(k.min), num(k.max))
		}
		fmt.Fprintf(&b, ` count="%d">`, len(f.Items))
		for _, it := range f.Items {
			b.WriteString(x.sharedItem(&it, k.date))
		}
		b.WriteString(`</sharedItems></cacheField>`)
	}
	b.WriteString(`</cacheFields></pivotCacheDefinition>`)

	var r strings.Builder
	r.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	fmt.Fprintf(&r, `<pivotCacheRecords xmlns="%s" xmlns:r="%s" count="%d">`, nsMain, nsRel, src.N)
	for i := range src.N {
		r.WriteString("<r>")
		for _, f := range src.Fields {
			fmt.Fprintf(&r, `<x v="%d"/>`, f.Rec[i])
		}
		r.WriteString("</r>")
	}
	r.WriteString(`</pivotCacheRecords>`)
	return b.String(), r.String()
}

func num(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

func (x *writer) isoDate(v float64) string {
	return serialDate(v, x.wb.Date1904).Format("2006-01-02T15:04:05")
}

func (x *writer) sharedItem(it *pivot.Item, dates bool) string {
	switch {
	case it.Key == "":
		return `<m/>`
	case it.Kind == model.KindNumber && dates:
		return `<d v="` + x.isoDate(it.Num) + `"/>`
	case it.Kind == model.KindNumber:
		return `<n v="` + num(it.Num) + `"/>`
	case it.Kind == model.KindBool:
		v := "0"
		if strings.EqualFold(it.Label, "TRUE") {
			v = "1"
		}
		return `<b v="` + v + `"/>`
	case it.Kind == model.KindError:
		return `<e v="` + xmlEscape(it.Label) + `"/>`
	}
	return `<s v="` + xmlEscape(it.Label) + `"/>`
}

// tableXML is report pt's pivotTableDefinition; its cache is number n.
func (x *writer) tableXML(pt *model.PivotTable, src *pivot.Source, n int, fmtIDs map[string]int) string {
	pt = pt.Clone()
	pivot.Sync(pt, src)
	// Lay it out again from the source written to the cache, so the
	// items match the cache's.
	pt.Out = pivot.Compute(pt, src, nil).Out
	o := &pt.Out
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	fmt.Fprintf(&b, `<pivotTableDefinition xmlns="%s" name="%s" cacheId="%d" applyNumberFormats="0" applyBorderFormats="0" applyFontFormats="0" applyPatternFormats="0" applyAlignmentFormats="0" applyWidthHeightFormats="1" dataCaption="Values" updatedVersion="6" minRefreshableVersion="3" useAutoFormatting="1" itemPrintTitles="1" createdVersion="6" indent="0"`,
		nsMain, xmlEscape(pt.Name), n)
	switch pt.Layout {
	case model.LayoutCompact:
		b.WriteString(` outline="1" outlineData="1"`)
	case model.LayoutOutline:
		b.WriteString(` compact="0" compactData="0" outline="1" outlineData="1"`)
	default:
		b.WriteString(` compact="0" compactData="0" gridDropZones="1"`)
	}
	if slices.Contains(pt.Rows, model.DataField) {
		b.WriteString(` dataOnRows="1"`)
	}
	if pt.NoRowGrand {
		b.WriteString(` rowGrandTotals="0"`)
	}
	if pt.NoColGrand {
		b.WriteString(` colGrandTotals="0"`)
	}
	b.WriteString(` multipleFieldFilters="0">`)
	fmt.Fprintf(&b, `<location ref="%s:%s" firstHeaderRow="%d" firstDataRow="%d" firstDataCol="%d"`,
		model.CellName(o.Body.R1, o.Body.C1), model.CellName(o.Body.R2, o.Body.C2), o.FirstHeaderRow, o.FirstDataRow, o.FirstDataCol)
	if len(pt.Pages) > 0 {
		fmt.Fprintf(&b, ` rowPageCount="%d" colPageCount="1"`, len(pt.Pages))
	}
	b.WriteString(`/>`)

	// pivotFields, with each axis field's items in report order; pos
	// maps an item key to its position in <items>.
	pos := make([]map[string]int, len(src.Fields))
	fmt.Fprintf(&b, `<pivotFields count="%d">`, len(src.Fields))
	for i, sf := range src.Fields {
		pf := &pt.Fields[i]
		axis := pt.AxisOf(i)
		isData := slices.ContainsFunc(pt.Values, func(v model.PivotValue) bool { return v.Field == i })
		b.WriteString(`<pivotField`)
		if pf.Caption != "" {
			fmt.Fprintf(&b, ` name="%s"`, xmlEscape(pf.Caption))
		}
		switch axis {
		case model.AxisRows:
			b.WriteString(` axis="axisRow"`)
		case model.AxisCols:
			b.WriteString(` axis="axisCol"`)
		case model.AxisPages:
			b.WriteString(` axis="axisPage"`)
		}
		if isData {
			b.WriteString(` dataField="1"`)
		}
		switch pt.Layout {
		case model.LayoutOutline:
			b.WriteString(` compact="0"`)
		case model.LayoutTabular:
			b.WriteString(` compact="0" outline="0"`)
		}
		if axis == model.AxisPages && len(pf.Hidden) > 0 {
			b.WriteString(` multipleItemSelectionAllowed="1"`)
		}
		b.WriteString(` showAll="0"`)
		if pt.BlankRows && axis == model.AxisRows {
			b.WriteString(` insertBlankRow="1"`)
		}
		onAxis := axis == model.AxisRows || axis == model.AxisCols || axis == model.AxisPages
		switch {
		case !onAxis:
		case pf.Sort == model.SortAscending:
			b.WriteString(` sortType="ascending"`)
		case pf.Sort == model.SortDescending:
			b.WriteString(` sortType="descending"`)
		}
		if pf.NoSubtotal {
			b.WriteString(` defaultSubtotal="0"`)
		}
		if pt.SubtotalsBottom && axis == model.AxisRows {
			b.WriteString(` subtotalTop="0"`)
		}
		if axis != model.AxisRows && axis != model.AxisCols && axis != model.AxisPages {
			b.WriteString(`/>`)
			continue
		}
		b.WriteString(`>`)
		order := pivot.Order(pf, sf)
		pos[i] = map[string]int{}
		cnt := len(order)
		if !pf.NoSubtotal {
			cnt++
		}
		fmt.Fprintf(&b, `<items count="%d">`, cnt)
		for p, it := range order {
			key := sf.Items[it].Key
			pos[i][key] = p
			if pf.IsHidden(key) {
				fmt.Fprintf(&b, `<item x="%d" h="1"/>`, it)
			} else {
				fmt.Fprintf(&b, `<item x="%d"/>`, it)
			}
		}
		if !pf.NoSubtotal {
			b.WriteString(`<item t="default"/>`)
		}
		b.WriteString(`</items>`)
		if pt.RepeatLabels && axis == model.AxisRows {
			b.WriteString(`<extLst><ext uri="{2946ED86-A175-432a-8AC1-64E0C546D7DE}" xmlns:x14="http://schemas.microsoft.com/office/spreadsheetml/2009/9/main"><x14:pivotField fillDownLabels="1"/></ext></extLst>`)
		}
		b.WriteString(`</pivotField>`)
	}
	b.WriteString(`</pivotFields>`)

	axisXML := func(tag string, axis []int, lines []model.PivotLine) {
		if len(axis) > 0 {
			fmt.Fprintf(&b, `<%sFields count="%d">`, tag, len(axis))
			for _, f := range axis {
				fmt.Fprintf(&b, `<field x="%d"/>`, f)
			}
			fmt.Fprintf(&b, `</%sFields>`, tag)
		}
		if len(lines) == 0 {
			return
		}
		fmt.Fprintf(&b, `<%sItems count="%d">`, tag, len(lines))
		var prev []int
		for _, l := range lines {
			var members []int
			it := 0
			for _, f := range axis {
				if f == model.DataField {
					if l.Data < 0 {
						break
					}
					members = append(members, l.Data)
					continue
				}
				if it >= len(l.Items) {
					break
				}
				members = append(members, pos[f][l.Items[it]])
				it++
			}
			r := 0
			if l.Kind != model.LineGrand {
				for r < len(members)-1 && r < len(prev) && prev[r] == members[r] {
					r++
				}
			}
			b.WriteString(`<i`)
			switch l.Kind {
			case model.LineSubtotal:
				b.WriteString(` t="default"`)
			case model.LineGrand:
				b.WriteString(` t="grand"`)
			case model.LineBlank:
				b.WriteString(` t="blank"`)
			}
			if r > 0 {
				fmt.Fprintf(&b, ` r="%d"`, r)
			}
			if l.Data > 0 {
				fmt.Fprintf(&b, ` i="%d"`, l.Data)
			}
			b.WriteString(`>`)
			if l.Kind == model.LineGrand || len(members) == 0 {
				if len(axis) > 0 {
					b.WriteString(`<x/>`)
				}
			} else {
				for _, m := range members[r:] {
					if m == 0 {
						b.WriteString(`<x/>`)
					} else {
						fmt.Fprintf(&b, `<x v="%d"/>`, m)
					}
				}
			}
			b.WriteString(`</i>`)
			if l.Kind != model.LineBlank {
				prev = members
			}
		}
		fmt.Fprintf(&b, `</%sItems>`, tag)
	}
	axisXML("row", pt.Rows, o.RowLines)
	axisXML("col", pt.Cols, o.ColLines)
	if len(pt.Pages) > 0 {
		fmt.Fprintf(&b, `<pageFields count="%d">`, len(pt.Pages))
		for _, f := range pt.Pages {
			fmt.Fprintf(&b, `<pageField fld="%d"`, f)
			// One item shown: it is the filter's item.
			shown := -1
			count := 0
			for _, it := range src.Fields[f].Items {
				if !pt.Fields[f].IsHidden(it.Key) {
					shown = pos[f][it.Key]
					count++
				}
			}
			if count == 1 {
				fmt.Fprintf(&b, ` item="%d"`, shown)
			}
			b.WriteString(` hier="-1"/>`)
		}
		b.WriteString(`</pageFields>`)
	}
	if len(pt.Values) > 0 {
		fmt.Fprintf(&b, `<dataFields count="%d">`, len(pt.Values))
		for _, v := range pt.Values {
			fmt.Fprintf(&b, `<dataField name="%s" fld="%d"`, xmlEscape(v.Name), v.Field)
			if v.Func != model.FuncSum {
				fmt.Fprintf(&b, ` subtotal="%s"`, pivotFuncs[v.Func])
			}
			if v.ShowAs != model.ShowNormal {
				fmt.Fprintf(&b, ` showDataAs="%s"`, showAsNames[v.ShowAs])
			}
			b.WriteString(` baseField="0" baseItem="0"`)
			id := v.NumFmtID
			if v.NumFmtCode != "" {
				id = fmtIDs[v.NumFmtCode]
			}
			if id > 0 {
				fmt.Fprintf(&b, ` numFmtId="%d"`, id)
			}
			b.WriteString(`/>`)
		}
		b.WriteString(`</dataFields>`)
	}
	style := pt.Style
	if style == "" {
		style = "PivotStyleLight16"
	}
	fmt.Fprintf(&b, `<pivotTableStyleInfo name="%s" showRowHeaders="1" showColHeaders="1" showRowStripes="0" showColStripes="0" showLastColumn="1"/>`, xmlEscape(style))
	b.WriteString(`<extLst><ext uri="{962EF5D1-5CA2-4c93-8EF4-DBF5C05439D2}" xmlns:x14="http://schemas.microsoft.com/office/spreadsheetml/2009/9/main"><x14:pivotTableDefinition hideValuesRow="1" xmlns:xm="http://schemas.microsoft.com/office/excel/2006/main"/></ext></extLst>`)
	b.WriteString(`</pivotTableDefinition>`)
	return b.String()
}

// sheetName is sheet's name as written to the file (sanitized, at most 31
// characters), which pivot sources and chart references must match.
func (x *writer) sheetName(sheet int) string {
	if sheet >= 0 && sheet < len(x.names) {
		return x.names[sheet]
	}
	return x.wb.Sheets[sheet].Name
}
