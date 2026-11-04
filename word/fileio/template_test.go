package fileio

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"officelite/common/ole"
	"officelite/word/model"
)

// Content types of a .docx's and a .dotx's main part.
const (
	docxMainType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	dotxMainType = "application/vnd.openxmlformats-officedocument.wordprocessingml.template.main+xml"
)

// TestTemplateFormats checks the template formats' extensions, names
// and the document formats they're built on.
func TestTemplateFormats(t *testing.T) {
	for _, c := range []struct {
		name     string
		want     Format
		ext      string
		template bool
		doc      Format
	}{
		{"a.docx", DOCX, ".docx", false, DOCX},
		{"a.doc", DOC, ".doc", false, DOC},
		{"a.txt", TXT, ".txt", false, TXT},
		{"Letter.DOTX", DOTX, ".dotx", true, DOCX},
		{"a.dotm", DOTX, ".dotx", true, DOCX},
		{"a.dot", DOT, ".dot", true, DOC},
	} {
		f, ok := FormatFor(c.name)
		if !ok || f != c.want {
			t.Errorf("FormatFor(%q) = %v, %v; want %v", c.name, f, ok, c.want)
			continue
		}
		if f.Ext() != c.ext || f.IsTemplate() != c.template || f.Document() != c.doc {
			t.Errorf("%v: Ext %q IsTemplate %v Document %v", f, f.Ext(), f.IsTemplate(), f.Document())
		}
	}
	if _, ok := FormatFor("a.pages"); ok {
		t.Error("FormatFor(.pages) is a save format")
	}
	if got := DOTX.String(); got != "Word Template (*.dotx)" {
		t.Errorf("DOTX.String() = %q", got)
	}
	if got := DOT.String(); got != "Word 97-2003 Template (*.dot)" {
		t.Errorf("DOT.String() = %q", got)
	}
}

// templateDoc is a small document to save as a template: a heading and a
// body paragraph.
func templateDoc() *model.Document {
	doc := model.New("Letter")
	doc.Body = []model.Block{
		&model.Paragraph{Style: "Heading1", Runs: []model.Run{{Text: "Company Letterhead"}}},
		&model.Paragraph{Runs: []model.Run{{Text: "Dear customer,"}}},
	}
	return doc
}

// docxMainContentType is the content type [Content_Types].xml gives a
// .docx package's word/document.xml.
func docxMainContentType(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name != "[Content_Types].xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		ct, err := io.ReadAll(rc)
		if cerr := rc.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			t.Fatal(err)
		}
		re := regexp.MustCompile(`PartName="/word/document\.xml"\s+ContentType="([^"]+)"`)
		if m := re.FindSubmatch(ct); m != nil {
			return string(m[1])
		}
		t.Fatalf("no override for word/document.xml in %s", ct)
	}
	t.Fatal("no [Content_Types].xml")
	return ""
}

// docIsTemplate reports whether a .doc's FIB has fDot set.
func docIsTemplate(t *testing.T, data []byte) bool {
	t.Helper()
	_, wd, err := ole.ReadStream(data, "WordDocument")
	if err != nil {
		t.Fatal(err)
	}
	if len(wd) < 12 {
		t.Fatalf("WordDocument stream is %d bytes", len(wd))
	}
	return binary.LittleEndian.Uint16(wd[10:])&0x0001 != 0
}

// isTemplate reports whether data, written in format f, is marked as a
// template.
func isTemplate(t *testing.T, data []byte, f Format) bool {
	t.Helper()
	switch f.Document() {
	case DOCX:
		switch ct := docxMainContentType(t, data); ct {
		case dotxMainType:
			return true
		case docxMainType:
			return false
		default:
			t.Fatalf("main part content type %q", ct)
		}
	case DOC:
		return docIsTemplate(t, data)
	}
	t.Fatalf("format %v has no template marker", f)
	return false
}

// bodyText is the text of the document's paragraphs, one per line.
func bodyText(doc *model.Document) string {
	var b bytes.Buffer
	doc.Paragraphs(func(p *model.Paragraph) bool {
		b.WriteString(p.Text())
		b.WriteByte('\n')
		return true
	})
	return b.String()
}

// TestTemplateRoundTrip saves a document as each template format and
// back: the template carries its marker and text, reads back like the
// document it's built on, and a document saved from it loses the marker
// (a new document made from a template is not a template).
func TestTemplateRoundTrip(t *testing.T) {
	for _, f := range []Format{DOTX, DOT} {
		t.Run(f.Ext(), func(t *testing.T) {
			dir := t.TempDir()
			src := templateDoc()
			want := bodyText(src)
			tpl := filepath.Join(dir, "Letter"+f.Ext())
			if err := SaveFile(src, tpl, f); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(tpl)
			if err != nil {
				t.Fatal(err)
			}
			if !isTemplate(t, data, f) {
				t.Fatalf("%s is not marked as a template", tpl)
			}
			doc, err := Open(tpl)
			if err != nil {
				t.Fatal(err)
			}
			if got := bodyText(doc); got != want {
				t.Fatalf("template text = %q, want %q", got, want)
			}
			if p, ok := doc.Body[0].(*model.Paragraph); !ok || p.Style != "Heading1" {
				t.Errorf("first paragraph lost its style: %+v", doc.Body[0])
			}

			// A document made from the template saves as a plain document.
			var out bytes.Buffer
			if err := Write(doc, &out, f.Document()); err != nil {
				t.Fatal(err)
			}
			if isTemplate(t, out.Bytes(), f.Document()) {
				t.Errorf("document saved from the template is still marked as one")
			}
			again, err := Read("Document1"+f.Document().Ext(), out.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if got := bodyText(again); got != want {
				t.Errorf("document text = %q, want %q", got, want)
			}
		})
	}
}

// TestDocxToTemplateKeepsPackage saves a Word-made .docx as a .dotx: the
// package is kept (its headers survive) and only the main part's content
// type changes, and saving the template as .docx changes it back.
func TestDocxToTemplateKeepsPackage(t *testing.T) {
	doc, err := Open("../../Samples/word/Headers-Footers-and-Styles.docx")
	if err != nil {
		t.Skip(err)
	}
	want := bodyText(doc)
	var tpl bytes.Buffer
	if err := Write(doc, &tpl, DOTX); err != nil {
		t.Fatal(err)
	}
	if ct := docxMainContentType(t, tpl.Bytes()); ct != dotxMainType {
		t.Fatalf("main content type %q", ct)
	}
	back, err := Read("Headers-Footers-and-Styles.dotx", tpl.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got := bodyText(back); got != want {
		t.Errorf("text changed:\n got %q\nwant %q", got, want)
	}
	if !model.AnyHF(back.Sections()) {
		t.Error("headers and footers lost")
	}
	var docx bytes.Buffer
	if err := Write(back, &docx, DOCX); err != nil {
		t.Fatal(err)
	}
	if ct := docxMainContentType(t, docx.Bytes()); ct != docxMainType {
		t.Errorf("document saved from template has content type %q", ct)
	}
}

// TestTemplateSamples opens the sample templates in Samples/word-templates
// (made from Samples/word/Headers-Footers-and-Styles.docx): each reads,
// carries its template marker and has the source's text.
func TestTemplateSamples(t *testing.T) {
	src, err := Open("../../Samples/word/Headers-Footers-and-Styles.docx")
	if err != nil {
		t.Skip(err)
	}
	want := bodyText(src)
	for _, f := range []Format{DOTX, DOT} {
		path := "../../Samples/word-templates/Headers-Footers-and-Styles" + f.Ext()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if !isTemplate(t, data, f) {
			t.Errorf("%s is not marked as a template", path)
		}
		doc, err := Read(filepath.Base(path), data)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if got := bodyText(doc); got != want {
			t.Errorf("%s text:\n got %q\nwant %q", path, got, want)
		}
		if f == DOTX && !model.AnyHF(doc.Sections()) {
			t.Errorf("%s: headers and footers lost", path)
		}
	}
}
