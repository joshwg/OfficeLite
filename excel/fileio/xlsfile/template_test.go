// Tests for shared formulas decoded once (fmlaTemplate): every cell's text
// is the one decoding the SHRFMLA at that cell gives.

package xlsfile

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
	"time"
)

// refN is a tRefN token: offsets (or absolute parts) row and col, with
// the relative flags in col's top bits.
func refN(row, col int) []byte { return cat([]byte{ptgRefN}, u16b(row), u16b(col)) }

// areaN is a tAreaN token.
func areaN(r1, r2, c1, c2 int) []byte {
	return cat([]byte{ptgAreaN}, u16b(r1), u16b(r2), u16b(c1), u16b(c2))
}

// The template gives decodeFormula's text (or error) at every cell,
// offsets wrapping around the sheet and whole-row/column areas included;
// formulas it can't split (a NUL in a string, near the length cap) fall
// back to decoding per cell.
func TestFormulaTemplateMatchesDecode(t *testing.T) {
	rel, rowRel, colRel := 0xC000, 0x8000, 0x4000
	long := []byte{ptgInt, 1, 0}
	for range 1000 {
		long = append(long, ptgAttr, attrSpace, 0, 15, ptgInt, 1, 0, ptgAdd)
	}
	for _, c := range []struct {
		name string
		rgce []byte
	}{
		{"refs", cat(refN(0xFFFF, 0xC0FF), areaN(0, 2, rel, 1|rowRel), []byte{ptgAdd}, refN(5, 3), []byte{ptgMul})},
		{"mixed", cat(refN(1, colRel|2), refN(0xFFFE, rowRel|7), []byte{ptgSub}, areaN(0, 0xFFFF, colRel, colRel|1), []byte{ptgAdd})},
		{"whole rows", cat(areaN(0, 0xFFFF, rowRel, rowRel|0xFF), areaN(0xFFFF, 0, rowRel|1, 3), []byte{ptgRange})},
		{"3d", cat([]byte{ptgRef3d}, u16b(1), u16b(2), u16b(rel|1), []byte{ptgArea3d}, u16b(5), u16b(0), u16b(1), u16b(colRel), u16b(colRel|2), []byte{ptgAdd})},
		{"3d deleted", cat([]byte{ptgRef3d}, u16b(5), u16b(1), u16b(rel), refN(1, rel|1), []byte{ptgAdd})},
		{"string", cat([]byte{ptgStr, 3, 0, 'A', '1', '"'}, refN(0, rel), []byte{ptgConcat, 0x42, 1, 4, 0})},
		{"nul string", cat([]byte{ptgStr, 21, 0, 0}, bytes.Repeat([]byte{'0'}, 19), []byte{1}, refN(0, rel), []byte{ptgConcat})},
		{"absolute", cat([]byte{ptgRef}, u16b(3), u16b(4), []byte{ptgInt, 2, 0, ptgAdd})},
		{"too long", cat(long, refN(0, rel), []byte{ptgAdd})},
		{"bad", []byte{ptgAdd}},
	} {
		p := testParser()
		tm := p.formulaTemplate(c.rgce, nil, 2, 1)
		for _, at := range [][2]int{{2, 1}, {0, 0}, {1, 0}, {100, 7}, {65534, 254}, {65535, 255}, {40000, 128}} {
			want, wantErr := p.decodeFormula(c.rgce, nil, at[0], at[1], true)
			got, err := tm.at(p, at[0], at[1])
			if got != want || (err == nil) != (wantErr == nil) || err != nil && err.Error() != wantErr.Error() {
				t.Errorf("%s at %v: %q, %v; want %q, %v", c.name, at, got, err, want, wantErr)
			}
		}
		if wantSlow := c.name == "nul string" || c.name == "too long"; tm.slow != wantSlow {
			t.Errorf("%s: slow = %v", c.name, tm.slow)
		}
	}
}

// sharedSheetStream builds a one-sheet workbook whose cells rows×cols from
// A1 all share rgce: a FORMULA record holding tExp to A1 per cell, the
// SHRFMLA after A1's.
func sharedSheetStream(rgce []byte, rows, cols int) []byte {
	var sheet []byte
	for r := range rows {
		for c := range cols {
			body := cat(u16b(r), u16b(c), u16b(0), binary.LittleEndian.AppendUint64(nil, math.Float64bits(1)),
				u16b(0), make([]byte, 4), u16b(5), []byte{ptgExp}, u16b(0), u16b(0))
			sheet = append(sheet, brec(recFormula, body...)...)
			if r == 0 && c == 0 {
				sheet = append(sheet, brec(recShrFmla, cat(u16b(0), u16b(rows-1), []byte{0, byte(cols - 1), 0, 0}, u16b(len(rgce)), rgce)...)...)
			}
		}
	}
	return workbook(nil, sheet)
}

// Every cell of a shared range reads as decoding the SHRFMLA at that cell.
func TestSharedFormulaSheet(t *testing.T) {
	rel := 0xC000
	rgce := cat(refN(0, rel|1), areaN(0xFFFF, 1, rel, rel|2), []byte{0x42, 1, 4, 0, ptgAdd}, []byte{ptgRef}, u16b(0), u16b(0), []byte{ptgMul})
	const rows, cols = 40, 6
	wb, err := Read(compound(t, sharedSheetStream(rgce, rows, cols)))
	if err != nil {
		t.Fatal(err)
	}
	p := testParser()
	sh := wb.Sheets[0]
	for r := range rows {
		for c := range cols {
			want, err := p.decodeFormula(rgce, nil, r, c, true)
			if err != nil {
				t.Fatal(err)
			}
			if got := sh.Get(r, c).Formula; got != want {
				t.Errorf("(%d,%d): %q, want %q", r, c, got, want)
			}
		}
	}
}

// Shared cells cost the same each however many share a large SHRFMLA: 4×
// the cells take well under 8× the time.
func TestSharedFormulaScales(t *testing.T) {
	rel := 0xC000
	rgce := refN(0, rel)
	for i := range 250 {
		rgce = append(rgce, refN(i%7, rel|i%5)...)
		rgce = append(rgce, ptgAdd)
	}
	small := compound(t, sharedSheetStream(rgce, 200, 5))
	large := compound(t, sharedSheetStream(rgce, 800, 5))
	timeRead := func(data []byte) time.Duration {
		best := time.Duration(math.MaxInt64)
		for range 3 {
			start := time.Now()
			if _, err := Read(data); err != nil {
				t.Fatal(err)
			}
			best = min(best, time.Since(start))
		}
		return best
	}
	ts, tl := timeRead(small), timeRead(large)
	if tl > 8*ts {
		t.Errorf("4000 cells took %v, 1000 cells %v: more than 8×", tl, ts)
	}
}
