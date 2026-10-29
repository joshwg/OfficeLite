package edit

import (
	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/model"
)

// PasteMode is a Paste menu choice.
type PasteMode int

const (
	PasteAll      PasteMode = iota // formulas, values and formats (Paste)
	PasteValues                    // values only, keeping the destination's formats
	PasteFormulas                  // formulas (and constants), keeping formats
	PasteFormats                   // formats only (also the Format Painter)
)

// PasteSpecial pastes part of clip into sel on sheet, tiled as Paste
// does. Only PasteAll moves cut cells; the other modes copy them.
func (b *Book) PasteSpecial(clip *Clip, sheet int, sel model.Range, mode PasteMode) (model.Range, calc.Result, error) {
	defer b.Op("Paste Special")()
	if mode == PasteAll {
		return b.Paste(clip, sheet, sel)
	}
	h, w := len(clip.Cells), len(clip.Cells[0])
	sh := b.WB.Sheets[sheet]
	dest, err := pasteTarget(sh, sel, h, w)
	if err != nil {
		return dest, calc.Result{}, err
	}
	child := spillChildren(sh)
	var cells []cellChange
	for r := dest.R1; r <= dest.R2; r++ {
		if h == 1 && sh.RowFiltered(r) {
			continue // as Paste: a single row fills only the rows shown
		}
		for c := dest.C1; c <= dest.C2; c++ {
			i, j := (r-dest.R1)%h, (c-dest.C1)%w
			src := clip.Cells[i][j]
			before := snapshot(sh, child, r, c)
			var v model.Cell
			switch mode {
			case PasteValues:
				v = model.Cell{Kind: src.Kind, Num: src.Num, Text: src.Text}
				if src.Formula != "" && src.Kind == model.KindEmpty {
					v.Kind, v.Num = model.KindNumber, 0 // a formula showing nothing yields 0
				}
				v = b.withStyle(v, before.Style)
			case PasteFormulas:
				v = src
				if v.Formula != "" {
					v.Formula = formula.Shift(v.Formula, r-clip.srcRow(i), c-(clip.Src.C1+j))
				}
				v = b.withStyle(v, before.Style)
			case PasteFormats:
				v = b.withStyle(before, src.Style)
			}
			if v != before {
				cells = append(cells, cellChange{sheet, r, c, before, v})
			}
		}
	}
	return dest, b.apply(Where{sheet, dest}, cells), nil
}

// CopyFormats takes the styles of r's cells (whole columns or rows up to
// the used area) for the Format Painter, which pastes them with
// PasteFormats.
func (b *Book) CopyFormats(sheet int, r model.Range) *Clip {
	sh := b.WB.Sheets[sheet]
	u, ok := FormatArea(sh, r)
	if !ok {
		u = model.Range{R1: r.R1, C1: r.C1, R2: r.R1, C2: r.C1}
	}
	clip := &Clip{WB: b.WB, Sheet: sheet, Src: u}
	for row := u.R1; row <= u.R2; row++ {
		line := make([]model.Cell, u.C2-u.C1+1)
		for col := u.C1; col <= u.C2; col++ {
			line[col-u.C1] = model.Cell{Style: sh.Get(row, col).Style}
		}
		clip.Cells = append(clip.Cells, line)
	}
	return clip
}
