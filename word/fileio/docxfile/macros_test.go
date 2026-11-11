// macros_test.go checks that a .docm (a document with a VBA project) is
// read as having macros, with the load warning.

package docxfile

import (
	"bytes"
	"slices"
	"testing"

	"officelite/common/macrotest"
	"officelite/word/model"
)

// TestReadMacros reads a plain document, then the same one with a VBA
// project.
func TestReadMacros(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(model.New("Test"), &buf); err != nil {
		t.Fatal(err)
	}
	vba, err := macrotest.AddToPackage(buf.Bytes(), "word/_rels/document.xml.rels")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		data []byte
		want bool
	}{{"plain", buf.Bytes(), false}, {"VBA project", vba, true}} {
		got, err := Read(tc.data)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.HasMacros != tc.want || slices.Contains(got.Warnings, model.MacroWarning) != tc.want {
			t.Errorf("%s: HasMacros %v, warnings %q; want %v", tc.name, got.HasMacros, got.Warnings, tc.want)
		}
	}
}
