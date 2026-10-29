package docxfile

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"officelite/word/model"
)

// docxWithBody returns the pydocx fixture with its body replaced.
func docxWithBody(t *testing.T, body string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "pydocx.docx"))
	if err != nil {
		t.Skip(err)
	}
	zr, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		if f.Name == "word/document.xml" {
			s := string(b)
			i := strings.Index(s, "<w:body>") + len("<w:body>")
			j := strings.Index(s, "<w:sectPr")
			b = []byte(s[:i] + body + s[j:])
		}
		w, _ := zw.Create(f.Name)
		w.Write(b)
	}
	zw.Close()
	return buf.Bytes()
}

// The address box of a letter: an auto-width, centered one-cell table
// whose grid fills the page but whose cell is 5.5" wide, with its own
// borders (a 3pt bar on the left).
const addressBox = `<w:tbl><w:tblPr><w:tblW w:type="auto" w:w="0"/><w:jc w:val="center"/></w:tblPr>` +
	`<w:tblGrid><w:gridCol w:w="9936"/></w:tblGrid><w:tr><w:tc><w:tcPr><w:tcW w:type="dxa" w:w="7920"/><w:shd w:fill="F4F6F4"/>` +
	`<w:tcBorders><w:top w:val="single" w:sz="6" w:space="0" w:color="135A28"/><w:left w:val="single" w:sz="24" w:space="0" w:color="135a28"/>` +
	`<w:bottom w:val="single" w:sz="6" w:space="0" w:color="auto"/><w:right w:val="none"/></w:tcBorders></w:tcPr>` +
	`<w:p><w:r><w:t>PO Box 3144</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`

func TestAutoWidthTableFromCells(t *testing.T) {
	fixed := strings.Replace(addressBox, `<w:tblW w:type="auto" w:w="0"/>`, `<w:tblW w:type="dxa" w:w="9936"/>`, 1)
	body := addressBox + `<w:p/>` + fixed + `<w:p/>`
	doc, err := Read(docxWithBody(t, body))
	if err != nil {
		t.Fatal(err)
	}
	var tables []*model.Table
	for _, b := range doc.Body {
		if tb, ok := b.(*model.Table); ok {
			tables = append(tables, tb)
		}
	}
	if len(tables) != 2 {
		t.Fatalf("%d tables", len(tables))
	}
	box := tables[0]
	if len(box.Columns) != 1 || box.Columns[0] != 396 || box.Align != model.AlignCenter {
		t.Errorf("auto-width table: columns %v align %v, want [396] centered", box.Columns, box.Align)
	}
	if got := tables[1].Columns; len(got) != 1 || got[0] != 496.8 {
		t.Errorf("fixed-width table columns %v, want the grid's [496.8]", got)
	}
	c := box.Rows[0].Cells[0]
	want := [4]model.Border{{Width: 0.75, Color: "135A28"}, {Width: 3, Color: "135A28"}, {Width: 0.75}, {}}
	for i, b := range c.Borders {
		if b == nil || *b != want[i] {
			t.Errorf("border %d = %+v, want %+v", i, b, want[i])
		}
	}
	// The cell borders and widths survive a re-save.
	var out bytes.Buffer
	if err := Write(doc, &out); err != nil {
		t.Fatal(err)
	}
	doc2, err := Read(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	c2 := doc2.Body[0].(*model.Table).Rows[0].Cells[0]
	if c2.Borders[model.EdgeLeft] == nil || c2.Borders[model.EdgeLeft].Width != 3 || c2.Width != 396 {
		t.Errorf("after re-save: borders %+v width %v", c2.Borders, c2.Width)
	}
}

func TestContextualSpacingRoundTrip(t *testing.T) {
	body := `<w:p><w:pPr><w:contextualSpacing/></w:pPr><w:r><w:t>a</w:t></w:r></w:p>` +
		`<w:p><w:pPr><w:contextualSpacing w:val="0"/></w:pPr><w:r><w:t>b</w:t></w:r></w:p><w:p><w:r><w:t>c</w:t></w:r></w:p>`
	doc, err := Read(docxWithBody(t, body))
	if err != nil {
		t.Fatal(err)
	}
	ps := paras(doc)
	if ps[0].Props.ContextualSpacing != model.On || ps[1].Props.ContextualSpacing != model.Off || ps[2].Props.ContextualSpacing != model.Unset {
		t.Fatalf("read: %v %v %v", ps[0].Props.ContextualSpacing, ps[1].Props.ContextualSpacing, ps[2].Props.ContextualSpacing)
	}
	// Set on a paragraph that didn't have it, and through a style.
	ps[2].Props.ContextualSpacing = model.On
	doc.Styles.Styles = append(doc.Styles.Styles, &model.Style{ID: "ListBullet", Name: "List Bullet", Type: model.ParagraphStyle,
		BasedOn: "Normal", Para: model.ParaProps{ContextualSpacing: model.On}})
	var out bytes.Buffer
	if err := Write(doc, &out); err != nil {
		t.Fatal(err)
	}
	doc2, err := Read(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	ps = paras(doc2)
	if ps[0].Props.ContextualSpacing != model.On || ps[1].Props.ContextualSpacing != model.Off || ps[2].Props.ContextualSpacing != model.On {
		t.Errorf("after re-save: %v %v %v", ps[0].Props.ContextualSpacing, ps[1].Props.ContextualSpacing, ps[2].Props.ContextualSpacing)
	}
	if st := doc2.Styles.Find("ListBullet"); st == nil || st.Para.ContextualSpacing != model.On {
		t.Errorf("style lost contextual spacing: %+v", st)
	}
	if got := doc2.ResolvePara(&model.Paragraph{Style: "ListBullet"}).ContextualSpacing; got != model.On {
		t.Errorf("resolved %v", got)
	}
}

func TestParagraphBordersRead(t *testing.T) {
	body := `<w:p><w:pPr><w:spacing w:before="80" w:after="320"/><w:pBdr><w:bottom w:val="single" w:sz="12" w:space="1" w:color="135A28"/></w:pBdr></w:pPr><w:r/></w:p>`
	doc, err := Read(docxWithBody(t, body))
	if err != nil {
		t.Fatal(err)
	}
	want := model.Border{Width: 1.5, Color: "135A28", Space: 1}
	if b := paras(doc)[0].Props.Borders[model.EdgeBottom]; b == nil || *b != want {
		t.Fatalf("bottom border %+v, want %+v", b, want)
	}
	var out bytes.Buffer
	if err := Write(doc, &out); err != nil {
		t.Fatal(err)
	}
	doc2, err := Read(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if b := paras(doc2)[0].Props.Borders[model.EdgeBottom]; b == nil || *b != want {
		t.Errorf("after re-save: %+v", b)
	}
}

// Numbering instances keep their abstract definition (lists sharing one
// count together in Word); a start override restarts; a missing w:start
// is 0, and 0 is kept.
func TestNumberingAbstractAndStart(t *testing.T) {
	root, err := parseXML([]byte(`<w:numbering xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">` +
		`<w:abstractNum w:abstractNumId="0"><w:lvl w:ilvl="0"><w:start w:val="1"/><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl></w:abstractNum>` +
		`<w:abstractNum w:abstractNumId="1"><w:lvl w:ilvl="0"><w:numFmt w:val="decimal"/><w:lvlText w:val="%1."/></w:lvl></w:abstractNum>` +
		`<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>` +
		`<w:num w:numId="2"><w:abstractNumId w:val="0"/></w:num>` +
		`<w:num w:numId="3"><w:abstractNumId w:val="0"/><w:lvlOverride w:ilvl="0"><w:startOverride w:val="1"/></w:lvlOverride></w:num>` +
		`<w:num w:numId="4"><w:abstractNumId w:val="1"/></w:num></w:numbering>`))
	if err != nil {
		t.Fatal(err)
	}
	ls := parseNumbering(root, &model.StyleSheet{})
	if ls[1].Abstract != "0" || ls[2].Abstract != "0" || ls[4].Abstract != "1" {
		t.Errorf("abstract ids %q %q %q", ls[1].Abstract, ls[2].Abstract, ls[4].Abstract)
	}
	if ls[2].Levels[0].Restart || !ls[3].Levels[0].Restart || ls[3].Levels[0].Start != 1 {
		t.Errorf("restart: %+v %+v", ls[2].Levels[0], ls[3].Levels[0])
	}
	if ls[4].Levels[0].Start != 0 {
		t.Errorf("missing start = %d", ls[4].Levels[0].Start)
	}
}

// w:cantSplit is read and survives saving.
func TestCantSplitRow(t *testing.T) {
	doc := mustRead(t, docxWithBody(t, `<w:tbl><w:tblGrid><w:gridCol w:w="2000"/></w:tblGrid>`+
		`<w:tr><w:trPr><w:cantSplit/></w:trPr><w:tc><w:p><w:r><w:t>a</w:t></w:r></w:p></w:tc></w:tr>`+
		`<w:tr><w:tc><w:p><w:r><w:t>b</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:p/>`))
	tb := doc.Body[0].(*model.Table)
	if !tb.Rows[0].CantSplit || tb.Rows[1].CantSplit {
		t.Fatalf("cantSplit %v %v", tb.Rows[0].CantSplit, tb.Rows[1].CantSplit)
	}
	tb.Rows[1].CantSplit = true
	tb.Rows[0].CantSplit = false
	back := mustRead(t, write(t, doc)).Body[0].(*model.Table)
	if back.Rows[0].CantSplit || !back.Rows[1].CantSplit {
		t.Errorf("after saving: %v %v", back.Rows[0].CantSplit, back.Rows[1].CantSplit)
	}
}
