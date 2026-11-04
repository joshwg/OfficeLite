// xmltree_test.go checks parseXML's limits on hostile input: deep nesting
// and text split into many pieces.

package docxfile

import (
	"strings"
	"testing"
	"time"

	"officelite/word"
)

// TestParseXMLDeepNesting checks that nesting beyond word.DocxMaxXMLDepth is an
// error rather than a tree the recursive readers would overflow on.
func TestParseXMLDeepNesting(t *testing.T) {
	deep := strings.Repeat("<a>", word.DocxMaxXMLDepth+1) + strings.Repeat("</a>", word.DocxMaxXMLDepth+1)
	if _, err := parseXML([]byte(deep)); err == nil {
		t.Fatal("no error for elements nested too deeply")
	}
	ok := strings.Repeat("<a>", 50) + strings.Repeat("</a>", 50)
	if _, err := parseXML([]byte(ok)); err != nil {
		t.Fatal(err)
	}
}

// TestParseXMLSplitText checks that leaf text split by comments is joined
// and gathered in linear time.
func TestParseXMLSplitText(t *testing.T) {
	root, err := parseXML([]byte(`<r><t>ab<!--x-->cd<?pi?>ef</t><u>g</u></r>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := root.kids[0].text; got != "abcdef" {
		t.Errorf("text %q, want abcdef", got)
	}
	if got := root.kids[1].text; got != "g" {
		t.Errorf("text %q, want g", got)
	}
	const n = 200000
	big := "<t>" + strings.Repeat("a<!---->", n) + "</t>"
	start := time.Now()
	root, err = parseXML([]byte(big))
	if err != nil {
		t.Fatal(err)
	}
	if len(root.text) != n {
		t.Errorf("text length %d, want %d", len(root.text), n)
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("parsing took %v", d)
	}
}
