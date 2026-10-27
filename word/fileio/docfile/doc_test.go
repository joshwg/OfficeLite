package docfile

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"officelite/common/ole"
	"officelite/word/model"
)

func readFixture(t *testing.T, name string) *model.Document {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Read(data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return doc
}

func roundTrip(t *testing.T, doc *model.Document) *model.Document {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(doc, &buf); err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("DOCOUT"); dir != "" {
		name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()) + ".doc"
		if err := os.WriteFile(filepath.Join(dir, name), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	back, err := Read(buf.Bytes())
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	return back
}

func paragraphs(doc *model.Document) []*model.Paragraph {
	var ps []*model.Paragraph
	doc.Paragraphs(func(p *model.Paragraph) bool { ps = append(ps, p); return true })
	return ps
}

// canon renders what a document shows: text, styles, direct and resolved
// formatting, tables, lists and page setup.
func canon(doc *model.Document) string {
	var b strings.Builder
	fmt.Fprintf(&b, "page %+v\n", doc.Page)
	var ids []int
	for id := range doc.Lists {
		ids = append(ids, id)
	}
	for id := 1; id <= len(ids)+10; id++ {
		if l := doc.Lists[id]; l != nil {
			fmt.Fprintf(&b, "list %d %+v\n", id, l.Levels)
		}
	}
	var blocks func([]model.Block, string)
	blocks = func(bs []model.Block, ind string) {
		for _, bl := range bs {
			switch bl := bl.(type) {
			case *model.Paragraph:
				fmt.Fprintf(&b, "%sP %q %s mark=%+v resolved=%s\n", ind, bl.Style, paraStr(bl.Props), bl.Mark, paraStr(zeroNil(doc.ResolvePara(bl))))
				for i := range bl.Runs {
					r := &bl.Runs[i]
					if r.Opaque != nil && strings.Contains(string(r.Opaque.Data), `"k":"shape"`) {
						continue // floating shapes are dropped on save
					}
					kind, note := r.Kind, r.Opaque != nil && strings.Contains(string(r.Opaque.Data), `"k":"note"`)
					if note {
						kind = model.RunText // saved as plain text
					}
					fmt.Fprintf(&b, "%s  R%d %q cs=%q %+v => %+v", ind, kind, r.Text, r.Style, r.Props, doc.ResolveRun(bl, r))
					if r.Opaque != nil && !note {
						fmt.Fprintf(&b, " opaque %s %.1fx%.1f %s", r.Opaque.Format, r.Opaque.Width, r.Opaque.Height, r.Opaque.Data)
					}
					b.WriteString("\n")
				}
			case *model.Table:
				fmt.Fprintf(&b, "%sT cols=%v borders=%v indent=%v align=%v\n", ind, bl.Columns, bl.Borders, bl.Indent, bl.Align)
				for _, row := range bl.Rows {
					fmt.Fprintf(&b, "%s row h=%v hdr=%v\n", ind, row.Height, row.Header)
					for _, c := range row.Cells {
						fmt.Fprintf(&b, "%s  cell span=%d vm=%d w=%v shd=%q va=%d\n", ind, c.Span, c.VMerge, c.Width, c.Shading, c.VAlign)
						blocks(c.Blocks, ind+"    ")
					}
				}
			}
		}
	}
	blocks(doc.Body, "")
	return b.String()
}

// zeroNil makes unset spacing and indents explicit zeros.
func zeroNil(p model.ParaProps) model.ParaProps {
	for _, f := range []**float64{&p.SpaceBefore, &p.SpaceAfter, &p.IndentLeft, &p.IndentRight, &p.IndentFirst} {
		if *f == nil {
			*f = model.F(0)
		}
	}
	return p
}

func diffLines(t *testing.T, want, got string) {
	t.Helper()
	if want == got {
		return
	}
	wl, gl := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			t.Errorf("first difference at line %d:\nwant %s\ngot  %s", i+1, w, g)
			return
		}
	}
}

func TestReadRich(t *testing.T) {
	doc := readFixture(t, "rich.doc")
	if doc.SourceFormat != "doc" || doc.Source == nil {
		t.Error("source not set")
	}
	ps := paragraphs(doc)
	if len(ps) != 30-3 { // Word counts each row-end mark as a paragraph
		t.Errorf("%d paragraphs", len(ps))
	}
	want := []struct{ style, text string }{
		{"Title", "Document Title"}, {"Heading1", "Heading One"},
	}
	for i, w := range want {
		if ps[i].Style != w.style || ps[i].Text() != w.text {
			t.Errorf("p%d = %q %q", i, ps[i].Style, ps[i].Text())
		}
	}
	if !strings.Contains(ps[3].Text(), "café and Ελληνικά") || ps[3].Style != "MyPara" {
		t.Errorf("unicode paragraph = %q (%q)", ps[3].Text(), ps[3].Style)
	}
	runs := map[string]model.Run{}
	for _, r := range ps[2].Runs {
		runs[r.Text] = r
	}
	checks := []struct {
		text string
		ok   func(model.RunProps) bool
	}{
		{"bold", func(p model.RunProps) bool { return p.Bold == model.On }},
		{"italic", func(p model.RunProps) bool { return p.Italic == model.On }},
		{"red", func(p model.RunProps) bool { return p.Color == "FF0000" }},
		{"big", func(p model.RunProps) bool { return p.Size == 18 }},
		{"courier", func(p model.RunProps) bool { return p.Font == "Courier New" }},
		{"underlined", func(p model.RunProps) bool { return p.Underline == model.UnderlineSingle }},
		{"struck", func(p model.RunProps) bool { return p.Strike == model.On }},
		{"2", func(p model.RunProps) bool { return p.Vert == model.VertSuper }},
		{"small caps", func(p model.RunProps) bool { return p.SmallCaps == model.On }},
		{"highlight end.", func(p model.RunProps) bool { return p.Highlight == "yellow" }},
	}
	for _, c := range checks {
		r, ok := runs[c.text]
		if !ok || !c.ok(r.Props) {
			t.Errorf("run %q = %+v (found %v)", c.text, r.Props, ok)
		}
	}
	if r := runs["char style"]; r.Style != "MyChar" || !doc.ResolveRun(ps[2], &r).Bold.Bool(false) {
		t.Errorf("char style run = %+v", r)
	}
	if al := ps[4].Props.Align; al != model.AlignCenter {
		t.Errorf("centered = %v", al)
	}
	p := ps[7]
	if *p.Props.SpaceBefore != 12 || *p.Props.SpaceAfter != 6 || p.Props.Line != 2 || p.Props.LineRule != model.LineAuto ||
		*p.Props.IndentLeft != 36 || *p.Props.IndentRight != 18 || *p.Props.IndentFirst != -18 {
		t.Errorf("spacing paragraph = %s", paraStr(p.Props))
	}
	if tabs := ps[8].Props.Tabs; len(tabs) != 2 || tabs[0].Pos != 144 || tabs[0].Align != model.AlignCenter || tabs[1].Leader != '.' {
		t.Errorf("tabs = %+v", tabs)
	}
	for i, fmt := range map[int]string{9: "bullet", 11: "decimal"} {
		l := ps[i].Props.List
		if l == nil || doc.Lists[l.NumID] == nil || doc.Lists[l.NumID].Levels[0].Format != fmt {
			t.Errorf("list paragraph %d = %+v", i, l)
		}
	}
	// Table with a merged first cell.
	var tbl *model.Table
	for _, b := range doc.Body {
		if tb, ok := b.(*model.Table); ok {
			tbl = tb
		}
	}
	if tbl == nil || len(tbl.Rows) != 3 || len(tbl.Rows[0].Cells) != 2 || tbl.Rows[0].Cells[0].Span != 2 || len(tbl.Columns) != 3 || !tbl.Borders {
		t.Fatalf("table = %+v", tbl)
	}
	if got := tbl.Rows[2].Cells[2].Blocks[0].(*model.Paragraph).Text(); got != "R3C3" {
		t.Errorf("cell text %q", got)
	}
	// Fields, picture, line and page breaks, footnote reference.
	var kinds []string
	for _, p := range ps {
		for _, r := range p.Runs {
			switch r.Kind {
			case model.RunOpaque:
				kinds = append(kinds, "opaque:"+r.Text)
			case model.RunPageBreak:
				kinds = append(kinds, "page")
			case model.RunBreak:
				kinds = append(kinds, "br")
			}
		}
	}
	if got := strings.Join(kinds, ","); got != "page,opaque:2026,opaque:2,br,opaque:[picture],opaque:1" {
		t.Errorf("special runs = %s", got)
	}
	if doc.Page.MarginLeft != 54 || doc.Page.Width != 612 {
		t.Errorf("page = %+v", doc.Page)
	}
	if len(doc.Warnings) != 2 {
		t.Errorf("warnings = %q", doc.Warnings)
	}
	if s := doc.Styles.Find("Heading1"); s == nil || s.Run.Size != 20 || s.Para.KeepNext != model.On || s.BasedOn != "Normal" {
		t.Errorf("Heading1 = %+v", s)
	}
	if s := doc.Styles.Find("DefaultParagraphFont"); s == nil || !s.Default || s.Type != model.CharacterStyle {
		t.Errorf("DefaultParagraphFont = %+v", s)
	}
}

func TestReadLandscape(t *testing.T) {
	doc := readFixture(t, "landscape.doc")
	if !doc.Page.Landscape || doc.Page.Width <= doc.Page.Height {
		t.Errorf("page = %+v", doc.Page)
	}
}

// TestRoundTripFixtures: .doc → model → .doc → model shows the same.
func TestRoundTripFixtures(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.doc")
	for _, f := range files {
		name := filepath.Base(f)
		if strings.HasPrefix(name, "bad_") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			doc := readFixture(t, name)
			back := roundTrip(t, doc)
			diffLines(t, canon(doc), canon(back))
			// And once more: the written file is stable.
			again := roundTrip(t, back)
			diffLines(t, canon(back), canon(again))
		})
	}
}

// builtDoc is a document made in code with every feature the writer handles.
func builtDoc() *model.Document {
	d := model.New("built")
	bullet := &model.List{Levels: []model.ListLevel{{Format: "bullet", Text: "\uF0B7", Start: 1, Indent: 36, Hang: 18, Font: "Symbol"}}}
	num := &model.List{}
	for i := 0; i < 9; i++ {
		num.Levels = append(num.Levels, model.ListLevel{Format: []string{"decimal", "lowerLetter", "lowerRoman"}[i%3],
			Text: fmt.Sprintf("%%%d.", i+1), Start: 1, Indent: float64(36 * (i + 1)), Hang: 18})
	}
	d.Lists[3] = bullet
	d.Lists[7] = num
	d.Styles.Styles = append(d.Styles.Styles,
		&model.Style{ID: "Accent", Name: "Accent", Type: model.CharacterStyle, BasedOn: "DefaultParagraphFont", Run: model.RunProps{Color: "C00000", Bold: model.On}},
		&model.Style{ID: "BoxedNote", Name: "Boxed Note", Type: model.ParagraphStyle, BasedOn: "Normal", Next: "Normal",
			Para: model.ParaProps{Shading: "DDEEFF", IndentLeft: model.F(18), Align: model.AlignJustify}, Run: model.RunProps{Font: "Georgia", Italic: model.On}})
	f := model.F
	text := func(s string) model.Run { return model.Run{Text: s} }
	d.Page = model.PageSetup{Width: 595.3, Height: 841.9, MarginTop: 72, MarginBottom: 72, MarginLeft: 63, MarginRight: 63, Header: 35.4, Footer: 35.4}
	cell := func(s string) *model.TableCell {
		return &model.TableCell{Span: 1, Blocks: []model.Block{&model.Paragraph{Runs: []model.Run{text(s)}}}}
	}
	inner := &model.Table{Columns: []float64{60, 60}, Rows: []*model.TableRow{{Cells: []*model.TableCell{cell("in1"), cell("in2")}}}}
	d.Body = []model.Block{
		&model.Paragraph{Style: "Title", Runs: []model.Run{text("Built In Code")}},
		&model.Paragraph{Style: "Heading1", Runs: []model.Run{text("First heading")}},
		&model.Paragraph{Runs: []model.Run{
			text("Normal "),
			{Text: "bold", Props: model.RunProps{Bold: model.On}},
			text(" "),
			{Text: "italic red 14pt", Props: model.RunProps{Italic: model.On, Color: "FF0000", Size: 14}},
			text(" "),
			{Text: "Courier", Props: model.RunProps{Font: "Courier New"}},
			text(" "),
			{Text: "double", Props: model.RunProps{Underline: model.UnderlineDouble}},
			text(" "),
			{Text: "struck caps", Props: model.RunProps{Strike: model.On, Caps: model.On}},
			text(" H"),
			{Text: "2", Props: model.RunProps{Vert: model.VertSub}},
			text("O "),
			{Text: "marked", Props: model.RunProps{Highlight: "green"}},
			text(" "),
			{Text: "shaded", Props: model.RunProps{Highlight: "FFCC00"}},
			text(" "),
			{Text: "accent", Style: "Accent"},
			text(" "),
			{Text: "not bold accent", Style: "Accent", Props: model.RunProps{Bold: model.Off}},
			{Kind: model.RunTab},
			text("after tab"),
			{Kind: model.RunBreak},
			text("after break — café Ελληνικά 日本語 😀"),
		}},
		&model.Paragraph{Style: "BoxedNote", Runs: []model.Run{text("A styled note paragraph with shading.")}},
		&model.Paragraph{Props: model.ParaProps{Align: model.AlignCenter, SpaceBefore: f(6), SpaceAfter: f(12), Line: 18, LineRule: model.LineExact}, Runs: []model.Run{text("Centered, exact 18pt lines")}},
		&model.Paragraph{Props: model.ParaProps{Align: model.AlignRight, Line: 1.5, LineRule: model.LineAuto, IndentRight: f(36)}, Runs: []model.Run{text("Right, 1.5 lines")}},
		&model.Paragraph{Props: model.ParaProps{IndentLeft: f(72), IndentFirst: f(-36), KeepNext: model.On, KeepLines: model.On, WidowControl: model.Off, Line: 14, LineRule: model.LineAtLeast},
			Runs: []model.Run{text("Hanging indent, keep with next")}},
		&model.Paragraph{Props: model.ParaProps{Tabs: []model.TabStop{{Pos: 72, Align: model.AlignLeft}, {Pos: 216, Align: model.AlignCenter, Leader: '.'}, {Pos: 360, Decimal: true, Align: model.AlignLeft}, {Pos: 432, Align: model.AlignRight, Leader: '_'}}},
			Runs: []model.Run{text("a"), {Kind: model.RunTab}, text("b"), {Kind: model.RunTab}, text("c"), {Kind: model.RunTab}, text("1.5"), {Kind: model.RunTab}, text("d")}},
		&model.Paragraph{Style: "ListParagraph", Props: model.ParaProps{List: &model.ListRef{NumID: 3}}, Runs: []model.Run{text("First bullet")}},
		&model.Paragraph{Style: "ListParagraph", Props: model.ParaProps{List: &model.ListRef{NumID: 3}}, Runs: []model.Run{text("Second bullet")}},
		&model.Paragraph{Style: "ListParagraph", Props: model.ParaProps{List: &model.ListRef{NumID: 7}}, Runs: []model.Run{text("Number one")}},
		&model.Paragraph{Style: "ListParagraph", Props: model.ParaProps{List: &model.ListRef{NumID: 7, Level: 1}}, Runs: []model.Run{text("Sub item a")}},
		&model.Paragraph{Style: "ListParagraph", Props: model.ParaProps{List: &model.ListRef{NumID: 7}}, Runs: []model.Run{text("Number two")}},
		&model.Table{Columns: []float64{120, 100, 140}, Borders: true, Align: model.AlignCenter, Indent: 0, Rows: []*model.TableRow{
			{Header: true, Height: 24, Cells: []*model.TableCell{
				{Span: 2, Shading: "D9E2F3", Blocks: []model.Block{&model.Paragraph{Runs: []model.Run{{Text: "Merged header", Props: model.RunProps{Bold: model.On}}}}}},
				{Span: 1, VMerge: 1, VAlign: 1, Blocks: []model.Block{&model.Paragraph{Runs: []model.Run{text("Tall")}}}},
			}},
			{Cells: []*model.TableCell{cell("a1"), {Span: 1, Blocks: []model.Block{
				&model.Paragraph{Runs: []model.Run{text("two")}},
				&model.Paragraph{Props: model.ParaProps{Align: model.AlignRight}, Runs: []model.Run{text("paragraphs")}},
			}}, {Span: 1, VMerge: 2, Blocks: []model.Block{&model.Paragraph{}}}}},
			{Cells: []*model.TableCell{cell("b1"), {Span: 1, Blocks: []model.Block{&model.Paragraph{}}}, cell("b3")}},
		}},
		&model.Paragraph{Runs: []model.Run{text("Nested table follows")}},
		&model.Table{Columns: []float64{200, 200}, Rows: []*model.TableRow{{Cells: []*model.TableCell{
			{Span: 1, Blocks: []model.Block{&model.Paragraph{Runs: []model.Run{text("outer")}}, inner}}, cell("right")}}}},
		&model.Paragraph{Runs: []model.Run{text("Before page break"), {Kind: model.RunPageBreak}, text("after page break")}},
		&model.Paragraph{Style: "Heading2", Props: model.ParaProps{PageBreakBefore: model.On}, Runs: []model.Run{
			text("Picture from docx: "), {Kind: model.RunOpaque, Text: "[picture]", Opaque: &model.Opaque{Format: "docx", Data: []byte("<w:r/>")}}}},
		&model.Paragraph{Mark: model.RunProps{Size: 20}},
		&model.Table{Columns: []float64{100}, Rows: []*model.TableRow{{Cells: []*model.TableCell{cell("last block is a table")}}}},
	}
	return d
}

func TestWriteBuilt(t *testing.T) {
	doc := builtDoc()
	ws := Warnings(doc)
	if len(ws) != 2 || !strings.Contains(strings.Join(ws, "|"), "[picture]") || !strings.Contains(strings.Join(ws, "|"), "Nested") {
		t.Errorf("warnings = %q", ws)
	}
	back := roundTrip(t, doc)

	ps, bp := paragraphs(doc), paragraphs(back)
	// The nested table is flattened into its cell and a paragraph follows
	// the final table.
	if len(bp) != len(ps)+1 {
		t.Fatalf("%d paragraphs, want %d", len(bp), len(ps)+1)
	}
	if a := Warnings(back); len(a) != 0 {
		t.Errorf("re-read document still warns: %q", a)
	}
	// The writer numbers lists 1..n in NumID order.
	renum := map[int]int{3: 1, 7: 2}
	for _, p := range ps {
		if l := p.Props.List; l != nil {
			p.Props.List = &model.ListRef{NumID: renum[l.NumID], Level: l.Level}
		}
	}
	doc.Lists = map[int]*model.List{1: doc.Lists[3], 2: doc.Lists[7]}
	for i, p := range ps {
		q := bp[i]
		want := p.Text()
		if strings.Contains(want, "￼") {
			want = strings.ReplaceAll(want, "￼", "[picture]")
		}
		if q.Text() != want {
			t.Errorf("p%d text %q, want %q", i, q.Text(), want)
		}
		if q.Style != p.Style {
			t.Errorf("p%d style %q, want %q", i, q.Style, p.Style)
		}
		if a, b := paraStr(zeroNil(doc.ResolvePara(p))), paraStr(zeroNil(back.ResolvePara(q))); a != b {
			t.Errorf("p%d %q resolved para\n got %s\nwant %s", i, p.Text(), b, a)
		}
		if a, b := paraStr(p.Props), paraStr(q.Props); a != b {
			t.Errorf("p%d %q direct para\n got %s\nwant %s", i, p.Text(), b, a)
		}
		for j := range p.Runs {
			if j >= len(q.Runs) {
				break
			}
			r, s := &p.Runs[j], &q.Runs[j]
			if r.Kind == model.RunOpaque {
				continue
			}
			if a, b := doc.ResolveRun(p, r), back.ResolveRun(q, s); a != b {
				t.Errorf("p%d run %q resolved\n got %+v\nwant %+v", i, r.Text, b, a)
			}
			if r.Style != s.Style || r.Props != s.Props {
				t.Errorf("p%d run %q = %q %+v, want %q %+v", i, r.Text, s.Style, s.Props, r.Style, r.Props)
			}
		}
	}
	if back.Page != doc.Page {
		t.Errorf("page %+v, want %+v", back.Page, doc.Page)
	}
	for id, l := range doc.Lists {
		if found := back.Lists[id]; found == nil || fmt.Sprint(found.Levels) != fmt.Sprint(l.Levels) {
			t.Errorf("list %d = %+v, want %+v", id, found, l)
		}
	}
	var tables []*model.Table
	for _, b := range back.Body {
		if tb, ok := b.(*model.Table); ok {
			tables = append(tables, tb)
		}
	}
	if len(tables) != 3 {
		t.Fatalf("%d tables", len(tables))
	}
	t0 := tables[0]
	if fmt.Sprint(t0.Columns) != "[120 100 140]" || !t0.Borders || t0.Align != model.AlignCenter || len(t0.Rows) != 3 {
		t.Errorf("table = %+v", t0)
	}
	r0 := t0.Rows[0]
	if !r0.Header || r0.Height != 24 || len(r0.Cells) != 2 || r0.Cells[0].Span != 2 || r0.Cells[0].Shading != "D9E2F3" ||
		r0.Cells[1].VMerge != 1 || r0.Cells[1].VAlign != 1 || t0.Rows[1].Cells[2].VMerge != 2 {
		t.Errorf("row 0 = %+v cells %+v %+v", r0, r0.Cells[0], r0.Cells[1])
	}
	if s := back.Styles.Find("BoxedNote"); s == nil || s.Name != "Boxed Note" || s.BasedOn != "Normal" || s.Para.Shading != "DDEEFF" || s.Run.Font != "Georgia" {
		t.Errorf("BoxedNote = %+v", s)
	}
	if s := back.Styles.Find("Accent"); s == nil || s.Type != model.CharacterStyle || s.Run.Color != "C00000" {
		t.Errorf("Accent = %+v", s)
	}
}

func TestWriteEmpty(t *testing.T) {
	doc := &model.Document{}
	back := roundTrip(t, doc)
	if len(back.Body) != 1 {
		t.Errorf("body = %+v", back.Body)
	}
}

func TestBigTableHugePapx(t *testing.T) {
	doc := model.New("wide")
	row := &model.TableRow{}
	for i := 0; i < 40; i++ {
		row.Cells = append(row.Cells, &model.TableCell{Span: 1, Shading: "FFFF00", Blocks: []model.Block{&model.Paragraph{Runs: []model.Run{{Text: fmt.Sprint(i)}}}}})
		doc.Body = nil
	}
	tbl := &model.Table{Rows: []*model.TableRow{row, row}}
	for i := 0; i < 40; i++ {
		tbl.Columns = append(tbl.Columns, 12)
	}
	doc.Body = []model.Block{tbl, &model.Paragraph{}}
	back := roundTrip(t, doc)
	bt, ok := back.Body[0].(*model.Table)
	if !ok || len(bt.Rows) != 2 || len(bt.Rows[1].Cells) != 40 || bt.Rows[1].Cells[39].Shading != "FFFF00" {
		t.Fatalf("table = %+v", back.Body[0])
	}
}

func TestManyParagraphs(t *testing.T) {
	doc := model.New("long")
	doc.Body = nil
	for i := 0; i < 3000; i++ {
		p := &model.Paragraph{Runs: []model.Run{{Text: fmt.Sprintf("Paragraph %d ", i)}, {Text: "bold", Props: model.RunProps{Bold: model.TriOf(i%2 == 0), Size: float64(8 + i%20)}}}}
		if i%7 == 0 {
			p.Props.Align = model.AlignCenter
		}
		doc.Body = append(doc.Body, p)
	}
	back := roundTrip(t, doc)
	diffLines(t, canon(doc), canon(back))
}

func TestRejects(t *testing.T) {
	doc := model.New("x")
	var buf bytes.Buffer
	if err := Write(doc, &buf); err != nil {
		t.Fatal(err)
	}
	patch := func(f func(wd []byte)) []byte {
		_, wd, _ := ole.ReadStream(buf.Bytes(), "WordDocument")
		_, tbl, _ := ole.ReadStream(buf.Bytes(), "1Table")
		f(wd)
		var out bytes.Buffer
		if err := ole.Write(&out, []ole.Stream{{Name: "WordDocument", Data: wd}, {Name: "1Table", Data: tbl}}); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	cases := []struct {
		name, want string
		f          func([]byte)
	}{
		{"word95", "Word 6.0/95", func(wd []byte) { binary.LittleEndian.PutUint16(wd[2:], 0x0065) }},
		{"encrypted", "password", func(wd []byte) { wd[11] |= 0x01 }},
		{"obfuscated", "obfuscated", func(wd []byte) { wd[11] |= 0x81 }},
		{"signature", "not a Word document", func(wd []byte) { wd[0] = 0 }},
	}
	for _, c := range cases {
		_, err := Read(patch(c.f))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
	if _, err := Read([]byte("not a compound file")); err == nil {
		t.Error("garbage accepted")
	}
	// Truncated streams must not panic.
	data := buf.Bytes()
	for _, n := range []int{512, 1024, 1536, len(data) / 2} {
		Read(data[:n])
	}
}

func TestReadEncryptedFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/bad_encrypted.doc")
	if err != nil {
		t.Skip("no fixture")
	}
	if _, err := Read(data); err == nil || !strings.Contains(err.Error(), "password") {
		t.Errorf("err = %v", err)
	}
}

func TestSprmSizes(t *testing.T) {
	var g grpprl
	g.b(sprmCFBold, 1)
	g.w(sprmCHps, 24)
	g.l(sprmCCv, 0x000000FF)
	g.v(sprmCShd, encShd("00FF00"))
	g.op(sprmPChgTabs)
	g = append(g, 255, 1, 0x20, 0x03, 0x30, 0x03, 1, 0x40, 0x06, 0x02) // cb 255: 1 del + close, 1 add
	g.op(sprmTDefTable)
	g = append(g, 2+1+4-1, 0, 1, 0, 0, 0x40, 0x01) // cb, n=1, 2 centers
	g.op(0xEA3F)                                   // sprmCSizePos: 3-byte operand (spra 7)
	g = append(g, 1, 2, 3)
	g.b(sprmPJc, 1)
	var ops []uint16
	var args [][]byte
	eachSprm(g, func(op uint16, a []byte) { ops = append(ops, op); args = append(args, a) })
	want := []uint16{sprmCFBold, sprmCHps, sprmCCv, sprmCShd, sprmPChgTabs, sprmTDefTable, 0xEA3F, sprmPJc}
	if fmt.Sprint(ops) != fmt.Sprint(want) {
		t.Fatalf("ops = %x, want %x", ops, want)
	}
	if colorRef(args[2]) != "FF0000" || shdColor(args[3]) != "00FF00" || len(args[5]) != 5 || args[5][0] != 1 {
		t.Errorf("args = % x", args)
	}
	tabs := decodeTabs(sprmPChgTabs, args[4])
	if len(tabs) != 2 || !tabs[0].Clear || tabs[0].Pos != 40 || tabs[1].Pos != 80 || tabs[1].Align != model.AlignRight {
		t.Errorf("tabs = %+v", tabs)
	}
	// Truncated input stops cleanly.
	eachSprm(g[:len(g)-1], func(uint16, []byte) {})
}

func TestPropsRoundTrip(t *testing.T) {
	w := newWriter(model.New("x"))
	r := &reader{fonts: w.fonts, doc: model.New("x")}
	rp := model.RunProps{Font: "Arial", Size: 13.5, Bold: model.On, Italic: model.Off, Underline: model.UnderlineWave,
		Strike: model.On, Caps: model.Off, SmallCaps: model.On, Hidden: model.On, Color: "123456", Highlight: "darkCyan", Vert: model.VertSuper}
	var g grpprl
	w.encRun(&g, rp)
	c := &chpInfo{istd: -1}
	r.fonts = w.fonts
	r.decodeChp(c, g)
	if got := resolveToggles(c, model.RunProps{}); got != rp {
		t.Errorf("run props\n got %+v\nwant %+v", got, rp)
	}
	// Toggles relative to the style.
	c = &chpInfo{istd: -1}
	r.decodeChp(c, []byte{0x35, 0x08, 0x81, 0x36, 0x08, 0x80})
	if got := resolveToggles(c, model.RunProps{Bold: model.On, Italic: model.On}); got.Bold != model.Off || got.Italic != model.Unset {
		t.Errorf("toggles = %+v", got)
	}
	pp := model.ParaProps{Align: model.AlignJustify, SpaceBefore: model.F(3), SpaceAfter: model.F(0), Line: 1.15, LineRule: model.LineAuto,
		IndentLeft: model.F(10), IndentRight: model.F(-5), IndentFirst: model.F(-10), KeepNext: model.On, KeepLines: model.Off,
		PageBreakBefore: model.On, WidowControl: model.Off, Shading: "ABCDEF", OutlineLevel: 3,
		Tabs: []model.TabStop{{Pos: 30, Clear: true}, {Pos: 50, Align: model.AlignCenter, Leader: '-'}}}
	var pi papInfo
	r.decodePap(&pi, w.encPap(pp))
	if a, b := paraStr(pp), paraStr(pi.props); a != b {
		t.Errorf("para props\n got %s\nwant %s", b, a)
	}
}

func TestPieceTable(t *testing.T) {
	// Text "Hi “x”" as a compressed piece followed by "Ωmega" as Unicode.
	wd := make([]byte, 0x300)
	copy(wd[0x200:], []byte{'H', 'i', ' ', 0x93, 'x', 0x94})
	for i, c := range []rune("Ωmega\r") {
		binary.LittleEndian.PutUint16(wd[0x280+2*i:], uint16(c))
	}
	clx := []byte{0x01, 3, 0, 0x36, 0x08, 0x01, 0x02} // Prc: sprmCFItalic 1; then the Pcdt
	plc := []byte{}
	for _, cp := range []uint32{0, 6, 12} {
		plc = binary.LittleEndian.AppendUint32(plc, cp)
	}
	plc = append(plc, 0, 0)
	plc = binary.LittleEndian.AppendUint32(plc, 0x200*2|0x40000000)
	plc = binary.LittleEndian.AppendUint16(plc, 0)
	plc = append(plc, 0, 0)
	plc = binary.LittleEndian.AppendUint32(plc, 0x280)
	plc = binary.LittleEndian.AppendUint16(plc, 1) // Prm: igrpprl 0
	clx = binary.LittleEndian.AppendUint32(clx, uint32(len(plc)))
	clx = append(clx, plc...)
	f := &fib{fcLcb: make([][2]uint32, nFcLcb97)}
	f.fcLcb[fcClx] = [2]uint32{0, uint32(len(clx))}
	f.lw[lwCcpText] = 12
	r := &reader{fib: f, wd: wd, table: clx}
	if err := r.readPieces(); err != nil {
		t.Fatal(err)
	}
	if got := string(utf16.Decode(r.text)); got != "Hi “x”Ωmega\r" {
		t.Errorf("text = %q", got)
	}
	if r.cpFc[1] != 0x201 || r.cpFc[7] != 0x282 {
		t.Errorf("fcs = %x", r.cpFc)
	}
	var italic model.Tri
	c := &chpInfo{istd: -1}
	r.doc = model.New("x")
	r.decodeChp(c, r.pieceSprms(1))
	italic = c.props.Italic
	if italic != model.On {
		t.Errorf("piece sprms = % x", r.pieceSprms(1))
	}
}

func TestStsh(t *testing.T) {
	doc := builtDoc()
	w := newWriter(doc)
	stsh := w.stsh()
	f := &fib{fcLcb: make([][2]uint32, nFcLcb97)}
	f.fcLcb[fcStshf] = [2]uint32{0, uint32(len(stsh))}
	r := &reader{fib: f, table: stsh, fonts: w.fonts, doc: &model.Document{}}
	r.readStyles()
	got := r.doc.Styles
	if got.DefaultRun.Font != "Calibri" {
		t.Errorf("default font %q", got.DefaultRun.Font)
	}
	for _, id := range []string{"Normal", "Heading1", "Heading3", "Title", "Strong", "Hyperlink", "DefaultParagraphFont", "TableNormal", "NoList", "Accent", "BoxedNote"} {
		s, o := got.Find(id), doc.Styles.Find(id)
		if s == nil {
			t.Errorf("style %s missing", id)
			continue
		}
		if o != nil && (s.Type != o.Type || s.Hidden != o.Hidden) {
			t.Errorf("%s = %+v, want %+v", id, s, o)
		}
	}
	h := got.Find("Heading1")
	if h.BasedOn != "Normal" || h.Next != "Normal" || h.Run.Font != "Calibri Light" || h.Run.Size != 16 || h.Run.Color != "2F5496" || h.Para.OutlineLevel != 1 || *h.Para.SpaceBefore != 12 {
		t.Errorf("Heading1 = %+v", h)
	}
	n := got.Find("Normal")
	if !n.Default || n.Run.Size != 11 || n.Run.Font != "Calibri" || *n.Para.SpaceAfter != 8 || n.Para.Line != 1.08 {
		t.Errorf("Normal = %+v %s", n, paraStr(n.Para))
	}
	if got.Find("TableGrid") != nil {
		t.Error("table styles are not written")
	}
}

func TestFieldsAndLinks(t *testing.T) {
	doc := readFixture(t, "rich.doc")
	var instrs []string
	for _, p := range paragraphs(doc) {
		for _, r := range p.Runs {
			if r.Opaque != nil && r.Opaque.Field != "" {
				instrs = append(instrs, r.Opaque.Field+"="+r.Text)
			}
		}
	}
	if got := strings.Join(instrs, "|"); got != `DATE \@ "yyyy"=2026|PAGE  \* MERGEFORMAT=2` {
		t.Errorf("fields = %s", got)
	}

	d := model.New("links")
	link := &model.Span{Kind: model.SpanLink, URL: "https://example.com/a?b=1", Tooltip: "Example"}
	d.DefaultTabStop = 28
	d.Body = []model.Block{&model.Paragraph{Runs: []model.Run{
		{Text: "See "},
		{Text: "the ", Span: link, Props: model.RunProps{Color: "0563C1"}},
		{Text: "site", Span: link, Props: model.RunProps{Bold: model.On}},
		{Text: " page "},
		{Kind: model.RunOpaque, Text: "7", Opaque: &model.Opaque{Format: "docx", Data: []byte("<w:fldSimple/>"), Field: "PAGE"}},
		{Text: "."},
	}}}
	if ws := Warnings(d); len(ws) != 0 {
		t.Errorf("warnings = %q", ws)
	}
	back := roundTrip(t, d)
	p := back.Body[0].(*model.Paragraph)
	var got []string
	for _, r := range p.Runs {
		s := fmt.Sprintf("%d:%s", r.Kind, r.Text)
		if r.Opaque != nil {
			s += "{" + r.Opaque.Field + "}"
		}
		got = append(got, s)
	}
	want := `0:See |4:the site{HYPERLINK "https://example.com/a?b=1" \o "Example"}|0: page |4:7{PAGE}|0:.`
	if strings.Join(got, "|") != want {
		t.Errorf("runs = %s\nwant   %s", strings.Join(got, "|"), want)
	}
	if back.DefaultTabStop != 28 {
		t.Errorf("default tab = %v", back.DefaultTabStop)
	}
	// The kept field writes back unchanged.
	again := roundTrip(t, back)
	diffLines(t, canon(back), canon(again))
}

func TestReadComplex(t *testing.T) {
	doc := readFixture(t, "complex.doc")
	w := strings.Join(doc.Warnings, "|")
	for _, want := range []string{"2 sections", "table of contents", "Endnotes", "Comments", "Text boxes", "Headers and footers", "Nested tables", "Floating"} {
		if !strings.Contains(w, want) {
			t.Errorf("no warning about %s: %q", want, doc.Warnings)
		}
	}
	if !doc.Page.Landscape {
		t.Error("last section is landscape")
	}
	ps := paragraphs(doc)
	if ps[0].Style != "TOC1" || ps[0].Text() != "Chapter A\t1" {
		t.Errorf("TOC entry = %q %q", ps[0].Style, ps[0].Text())
	}
	var link *model.Run
	for i, r := range ps[3].Runs {
		if r.Opaque != nil && strings.HasPrefix(r.Opaque.Field, "HYPERLINK") {
			link = &ps[3].Runs[i]
		}
	}
	if link == nil || link.Text != "Example site" || link.Style != "Hyperlink" {
		t.Errorf("hyperlink = %+v", link)
	}
	var tbl *model.Table
	for _, b := range doc.Body {
		if tb, ok := b.(*model.Table); ok {
			tbl = tb
		}
	}
	if tbl == nil || len(tbl.Rows) != 3 || tbl.Align != model.AlignCenter {
		t.Fatalf("table = %+v", tbl)
	}
	r0, r1, r2 := tbl.Rows[0], tbl.Rows[1], tbl.Rows[2]
	if !r0.Header || r0.Height != 30 || r0.Cells[1].Shading != "FFFF00" {
		t.Errorf("header row = %+v %+v", r0, r0.Cells[1])
	}
	if r1.Cells[0].VMerge != 1 || r1.Cells[0].VAlign != 1 || r2.Cells[0].VMerge != 2 {
		t.Errorf("vertical merge = %+v / %+v", r1.Cells[0], r2.Cells[0])
	}
	var nested []string
	model.WalkBlocks(r1.Cells[1].Blocks, func(p *model.Paragraph) bool { nested = append(nested, p.Text()); return true })
	if strings.Join(nested, ",") != "n1,n4," {
		t.Errorf("nested cell = %q", nested)
	}
}
