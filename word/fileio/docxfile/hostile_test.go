// hostile_test.go checks the reader on synthetic documents built to be
// slow: many complex fields left open.

package docxfile

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
	"time"
)

// bodyDocx is a minimal package whose document.xml body is body.
func bodyDocx(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(`<w:document xmlns:w="` + nsW + `"><w:body>` + body + `</w:body></w:document>`)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestManyOpenFields checks that many field begins never ended (each
// hiding everything after it) read in about linear time, and
// that a field's code is hidden only until its separator.
func TestManyOpenFields(t *testing.T) {
	// Time n and 4n begins: linear reading takes about 4 times as long,
	// quadratic 16 times.
	elapsed := func(n int) time.Duration {
		body := "<w:p>" + strings.Repeat(`<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:t>x</w:t></w:r>`, n) + "</w:p>"
		data := bodyDocx(t, body)
		start := time.Now()
		if _, err := Read(data); err != nil {
			t.Fatal(err)
		}
		return time.Since(start)
	}
	small, big := elapsed(5000), elapsed(20000)
	if big > 10*small {
		t.Errorf("4× the fields took %v against %v: not linear", big, small)
	}

	// A begin and separator in one paragraph, the result and end in the
	// next: the code is hidden, the result is text.
	body := `<w:p><w:r><w:t>a</w:t></w:r><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText>PAGE</w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r></w:p>` +
		`<w:p><w:r><w:t>b</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r><w:r><w:t>c</w:t></w:r></w:p>`
	doc, err := Read(bodyDocx(t, body))
	if err != nil {
		t.Fatal(err)
	}
	var text []string
	for _, p := range paras(doc) {
		var b strings.Builder
		for _, r := range p.Runs {
			b.WriteString(r.Text)
		}
		text = append(text, b.String())
	}
	if len(text) != 2 || text[0] != "a" || text[1] != "bc" {
		t.Errorf("paragraph text %q, want [a bc]", text)
	}
}
