// opc_test.go tests the relationship-part and target-resolution helpers.

package opc

import "testing"

// TestRelsPath checks the relationships part of the package and of parts.
func TestRelsPath(t *testing.T) {
	for in, want := range map[string]string{
		"":                  "_rels/.rels",
		"word/document.xml": "word/_rels/document.xml.rels",
		"a.xml":             "_rels/a.xml.rels",
	} {
		if got := RelsPath(in); got != want {
			t.Errorf("RelsPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestResolve checks relative, parent, absolute and dotted targets.
func TestResolve(t *testing.T) {
	for _, c := range []struct{ from, target, want string }{
		{"", "word/document.xml", "word/document.xml"},
		{"xl/workbook.xml", "worksheets/sheet1.xml", "xl/worksheets/sheet1.xml"},
		{"xl/worksheets/sheet1.xml", "../comments1.xml", "xl/comments1.xml"},
		{"xl/workbook.xml", "/xl/styles.xml", "xl/styles.xml"},
		{"word/document.xml", "./media/../media/image1.png", "word/media/image1.png"},
	} {
		if got := Resolve(c.from, c.target); got != c.want {
			t.Errorf("Resolve(%q, %q) = %q, want %q", c.from, c.target, got, c.want)
		}
	}
}
