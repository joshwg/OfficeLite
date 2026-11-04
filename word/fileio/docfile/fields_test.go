// fields_test.go checks readFields on synthetic text: which characters a
// flattened field hides, and that many or deeply nested fields don't take
// quadratic time.

package docfile

import (
	"testing"
	"time"

	"officelite/word/model"
)

// fieldReader is a reader over text alone, enough for readFields.
func fieldReader(text []uint16) *reader {
	return &reader{fib: &fib{}, text: text, warned: map[string]bool{}, doc: &model.Document{}}
}

// TestReadFieldsFlatten checks a field whose code spans paragraphs: its
// code (a nested field included) is hidden, paragraph marks and the
// result are not, and a field on one line becomes a unit.
func TestReadFieldsFlatten(t *testing.T) {
	//                 0     1    2     3     4    5     6     7    8     9    10   11    12   13    14    15   16    17
	text := []uint16{0x13, 'A', 0x0D, 0x13, 'B', 0x14, 'b', 0x15, 0x14, 'r', 0x0D, 's', 0x15, 0x13, 'C', 0x14, 'c', 0x15}
	r := fieldReader(text)
	r.readFields()
	wantSkip := map[int]bool{0: true, 1: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true, 12: true}
	for cp := range text {
		if r.skip[cp] != wantSkip[cp] {
			t.Errorf("skip[%d] = %v, want %v", cp, r.skip[cp], wantSkip[cp])
		}
	}
	if e, ok := r.unitEnd[13]; !ok || e != 17 {
		t.Errorf("unitEnd[13] = %d, %v; want 17", e, ok)
	}
	if len(r.doc.Warnings) != 1 {
		t.Errorf("warnings %q, want one", r.doc.Warnings)
	}
}

// TestReadFieldsManyFields checks that many fields, side by side or
// nested inside a flattened field's code, are read in about linear time.
func TestReadFieldsManyFields(t *testing.T) {
	const n = 100000
	var flat []uint16
	for range n {
		flat = append(flat, 0x13, 'a', 0x14, 'b', 0x15)
	}
	var nested []uint16
	for range n {
		nested = append(nested, 0x13, 'a', 0x0D)
	}
	for range n {
		nested = append(nested, 0x14, 'x', 0x15)
	}
	for name, text := range map[string][]uint16{"side by side": flat, "nested": nested} {
		start := time.Now()
		r := fieldReader(text)
		r.readFields()
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("%s: readFields took %v", name, d)
		}
	}
}
