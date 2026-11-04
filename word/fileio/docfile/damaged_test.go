// damaged_test.go checks the readers on damaged or hostile synthetic
// structures: deeply nested shape groups and bad counts.

package docfile

import (
	"encoding/binary"
	"testing"

	"officelite/word"
	"officelite/word/model"
)

// officeArt wraps data in an OfficeArt record header of type typ.
func officeArt(typ uint16, data []byte) []byte {
	h := make([]byte, 8, 8+len(data))
	binary.LittleEndian.PutUint16(h, 0x000F)
	binary.LittleEndian.PutUint16(h[2:], typ)
	binary.LittleEndian.PutUint32(h[4:], uint32(len(data)))
	return append(h, data...)
}

// TestCollectShapesDepth checks that shapes in nested groups are found
// and that groups nested beyond word.DocMaxShapeDepth are skipped, not followed
// until the stack runs out.
func TestCollectShapesDepth(t *testing.T) {
	sp := make([]byte, 8)
	binary.LittleEndian.PutUint32(sp, 7) // spid
	shapeAt := func(depth int) []byte {
		b := officeArt(0xF004, officeArt(0xF00A, sp))
		for range depth {
			b = officeArt(0xF003, b)
		}
		return b
	}
	r := &reader{shapes: map[uint32]*shape{}}
	r.collectShapes(shapeAt(3), 0)
	if r.shapes[7] == nil {
		t.Error("shape three groups down not found")
	}
	r = &reader{shapes: map[uint32]*shape{}}
	r.collectShapes(shapeAt(word.DocMaxShapeDepth+5), 0)
	if r.shapes[7] != nil {
		t.Error("shape nested beyond word.DocMaxShapeDepth was followed")
	}
}

// TestReadListsNegativeCount checks that a list table whose count is
// negative reads as no lists instead of failing the whole document.
func TestReadListsNegativeCount(t *testing.T) {
	f := &fib{fcLcb: make([][2]uint32, nFcLcb97)}
	f.fcLcb[fcPlfLst] = [2]uint32{0, 2}
	f.fcLcb[fcPlfLfo] = [2]uint32{2, 4}
	r := &reader{fib: f, table: []byte{0xFF, 0xFF, 0, 0, 0, 0}, doc: &model.Document{Lists: map[int]*model.List{}}}
	r.readLists()
	if len(r.doc.Lists) != 0 {
		t.Errorf("lists %v, want none", r.doc.Lists)
	}
}

// TestTableDepthBounded checks that a paragraph claiming a table depth of
// billions (sprmPItap) is read into tables nested at most word.DocMaxTableDepth
// deep instead of allocating a table per level.
func TestTableDepthBounded(t *testing.T) {
	var g grpprl
	g.l(sprmPItap, 0x7FFFFFFF)
	r := &reader{fib: &fib{}, text: []uint16{'a', 0x0D}, cpFc: []uint32{0, 1}, cpPiece: []int32{0, 0},
		skip: make([]bool, 2), pieces: []piece{{cp: 0, cpEnd: 2}}, papx: []fkpRun{{start: 0, end: 2, grpprl: g}},
		warned: map[string]bool{}, chpCache: map[[2]int]*chpInfo{}, doc: &model.Document{}}
	blocks := r.blocks(0, 2, false)
	depth := 0
	for len(blocks) == 1 {
		tb, ok := blocks[0].(*model.Table)
		if !ok || len(tb.Rows) == 0 || len(tb.Rows[0].Cells) == 0 {
			break
		}
		depth++
		blocks = tb.Rows[0].Cells[0].Blocks
	}
	if depth == 0 || depth > word.DocMaxTableDepth {
		t.Errorf("table depth %d, want 1..%d", depth, word.DocMaxTableDepth)
	}
}
