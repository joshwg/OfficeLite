// contenttypes_test.go checks the main part's content type when saving a
// package whose [Content_Types].xml types it by a Default only.

package docxfile

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// TestTemplateTypeFromDefault checks that saving as a template a package
// typing document.xml through a Default (no Override) still marks the
// main part as a template, and that saving it back as a document does
// the reverse.
func TestTemplateTypeFromDefault(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range map[string]string{
		"[Content_Types].xml": `<Types xmlns="` + nsCT + `"><Default Extension="rels" ContentType="` + ctRels + `"/><Default Extension="xml" ContentType="` + ctMain + `"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="` + nsRel + `"><Relationship Id="rId1" Type="` + relTypePrefix + relOfficeDoc + `" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<w:document xmlns:w="` + nsW + `"><w:body><w:p><w:r><w:t>hi</w:t></w:r></w:p></w:body></w:document>`,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	doc := mustRead(t, buf.Bytes())
	var out bytes.Buffer
	if err := WriteTemplate(doc, &out); err != nil {
		t.Fatal(err)
	}
	ct := parseContentTypes(zipParts(t, out.Bytes())["[Content_Types].xml"])
	if got := ct.typeOf("word/document.xml"); got != ctTemplate {
		t.Errorf("template main part type %q, want %q", got, ctTemplate)
	}
	tpl := mustRead(t, out.Bytes())
	out.Reset()
	if err := Write(tpl, &out); err != nil {
		t.Fatal(err)
	}
	ct = parseContentTypes(zipParts(t, out.Bytes())["[Content_Types].xml"])
	if got := ct.typeOf("word/document.xml"); got != ctMain {
		t.Errorf("document main part type %q, want %q", got, ctMain)
	}
	if strings.Count(string(zipParts(t, out.Bytes())["[Content_Types].xml"]), "/word/document.xml") != 1 {
		t.Error("main part has more than one Override")
	}
}
