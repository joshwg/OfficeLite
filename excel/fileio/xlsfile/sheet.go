package xlsfile

import (
	"errors"

	"officelite/excel/model"
)

// readSheet reads one worksheet substream starting at its BOF record.
func (p *parser) readSheet(pos int, sh *model.Sheet) {
	var (
		depth      int
		dropped    int
		frozen     bool
		paneX      int
		paneY      int
		paneAct    = 3                // active pane: 3 = the only one
		selections = map[int][2]int{} // SELECTION active cell by pane
		defRowTw   = -1
		pendingRow = -1 // FORMULA cell waiting for its STRING record
		pendingCol int
		strSegs    [][]byte // STRING record and its CONTINUE records
		lastRow    = -1     // the last FORMULA cell, owner of a following SHRFMLA/ARRAY
		lastCol    int
		exps       []expCell                 // cells whose formula is a tExp
		shared     = map[[2]int]sharedFmla{} // SHRFMLA / ARRAY / TABLE by anchor
		badFormula int
	)
	set := func(row, col int, c model.Cell) {
		if !sh.Set(row, col, c) && c.Kind != model.KindEmpty {
			dropped++
		}
	}
	number := func(row, col, ixfe int, v float64) {
		style, id, code := p.cellFormat(ixfe)
		set(row, col, model.Cell{Text: p.nf.Format(v, id, code), Num: v, Kind: model.KindNumber, Style: style})
	}
	text := func(row, col, ixfe int, s string) {
		style, _, _ := p.cellFormat(ixfe)
		set(row, col, model.Cell{Text: s, Kind: model.KindText, Style: style})
	}
	blank := func(row, col, ixfe int) {
		style, _, _ := p.cellFormat(ixfe)
		if style != 0 {
			set(row, col, model.Cell{Style: style})
		}
	}

	records(p.stream, pos, func(r record) bool {
		b := r.body
		switch r.id {
		case recBOF:
			depth++
			// Charts embedded in the sheet are nested BOF..EOF substreams.
			if depth == 1 && u16(b, 2) != bofTypeWorksheet {
				return false
			}
			return true
		case recEOF:
			depth--
			return depth > 0
		}
		if depth != 1 {
			return true
		}
		if r.id == recContinue && strSegs != nil {
			strSegs = append(strSegs, b)
			return true
		}
		if strSegs != nil {
			// The formula's text result, possibly continued.
			s := (&segments{segs: strSegs}).str()
			if c := sh.Get(pendingRow, pendingCol); c.Kind == model.KindText {
				c.Text = s
			}
			strSegs, pendingRow = nil, -1
		}
		switch r.id {
		case recString, recShrFmla, recArray, recTable:
		default:
			pendingRow = -1
		}
		switch r.id {
		case recShrFmla, recArray, recTable:
		default:
			lastRow = -1
		}
		row, col, ixfe := u16(b, 0), u16(b, 2), u16(b, 4)
		switch r.id {
		case recNumber:
			number(row, col, ixfe, f64(b, 6))
		case recRK:
			number(row, col, ixfe, rkValue(u32(b, 6)))
		case recMulRK:
			n := (len(b) - 6) / 6
			for i := range n {
				number(row, col+i, u16(b, 4+6*i), rkValue(u32(b, 6+6*i)))
			}
		case recLabelSST:
			if i := int(u32(b, 6)); i < len(p.sst) {
				text(row, col, ixfe, p.sst[i])
			}
		case recLabel, recRString:
			s, _ := unicodeString(b, 6, 2)
			text(row, col, ixfe, s)
		case recBoolErr:
			style, _, _ := p.cellFormat(ixfe)
			c := model.Cell{Style: style}
			if len(b) >= 8 && b[7] != 0 {
				c.Kind, c.Text = model.KindError, errorTexts[b[6]]
			} else if len(b) >= 7 {
				c.Kind, c.Text = model.KindBool, "FALSE"
				if b[6] != 0 {
					c.Text, c.Num = "TRUE", 1
				}
			}
			set(row, col, c)
		case recFormula:
			// The formula's last computed value is cached in the record,
			// the parsed expression follows.
			if len(b) < 14 {
				break
			}
			lastRow, lastCol = row, col
			defer func() { // once the cell holds its value
				cce := u16(b, 20)
				if len(b) < 22+cce {
					badFormula++
					return
				}
				f, err := p.decodeFormula(b[22:22+cce], b[22+cce:], row, col, false)
				var se errShared
				switch {
				case errors.As(err, &se):
					exps = append(exps, expCell{row, col, se.row, se.col})
				case err != nil:
					badFormula++
				default:
					setFormula(sh, row, col, f)
				}
			}()
			if b[12] == 0xFF && b[13] == 0xFF {
				style, _, _ := p.cellFormat(ixfe)
				switch b[6] {
				case 0: // string result follows in a STRING record
					pendingRow, pendingCol = row, col
					text(row, col, ixfe, "")
				case 1:
					c := model.Cell{Kind: model.KindBool, Text: "FALSE", Style: style}
					if b[8] != 0 {
						c.Text, c.Num = "TRUE", 1
					}
					set(row, col, c)
				case 2:
					set(row, col, model.Cell{Kind: model.KindError, Text: errorTexts[b[8]], Style: style})
				case 3:
					text(row, col, ixfe, "")
				}
			} else {
				number(row, col, ixfe, f64(b, 6))
			}
		case recString:
			if pendingRow >= 0 {
				strSegs = [][]byte{b}
			}
		case recShrFmla, recArray, recTable:
			if lastRow < 0 {
				break
			}
			sf := sharedFmla{kind: r.id, rng: model.Range{R1: u16(b, 0), R2: u16(b, 2), C1: int(b[4]), C2: int(b[5])}}
			start := 0
			switch r.id {
			case recShrFmla:
				start = 8
			case recArray:
				start = 12
			}
			if r.id != recTable && len(b) >= start+2 {
				cce := u16(b, start)
				end := min(start+2+cce, len(b))
				sf.rgce, sf.extra = b[start+2:end], b[end:]
			}
			shared[[2]int{lastRow, lastCol}] = sf
		case recBlank:
			blank(row, col, ixfe)
		case recMulBlank:
			n := (len(b) - 6) / 2
			for i := range n {
				blank(row, col+i, u16(b, 4+2*i))
			}
		case recMergedCells:
			n := u16(b, 0)
			for i := range n {
				o := 2 + 8*i
				if o+8 > len(b) {
					break
				}
				sh.Merges = append(sh.Merges, model.Range{
					R1: u16(b, o), R2: u16(b, o+2), C1: u16(b, o+4), C2: u16(b, o+6),
				})
			}
		case recDefColWidth:
			sh.BaseColWidth = float32(u16(b, 0))
		case recStandardWidth:
			// Overrides DEFCOLWIDTH: 1/256 characters, padding included.
			sh.DefaultColWidth = float32(u16(b, 0)) / 256
		case recDefaultRowHt:
			defRowTw = u16(b, 2)
			sh.DefaultRowHeight = model.PointsToPx(float64(defRowTw) / 20)
		case recColInfo:
			first, last, width := u16(b, 0), u16(b, 2), u16(b, 4)
			w := float32(width) / 256
			if u16(b, 8)&0x01 != 0 {
				w = 0
			}
			for c := first; c <= min(last, xlsMaxCols-1); c++ { // COLINFO may run to column 257
				sh.ColWidth[c] = w
			}
		case recRow:
			height := u16(b, 6) & 0x7FFF
			flags := u32(b, 12)
			switch {
			case flags&0x20 != 0:
				sh.RowHeight[row] = 0
			case flags&0x40 != 0 || (defRowTw >= 0 && height != defRowTw):
				sh.RowHeight[row] = model.PointsToPx(float64(height) / 20)
			}
		case recWindow2:
			grbit := u16(b, 0)
			sh.ShowGridLines = grbit&0x02 != 0
			frozen = grbit&0x08 != 0
			sh.TopRow, sh.LeftCol = u16(b, 2), u16(b, 4)
		case recPane:
			paneX, paneY = u16(b, 0), u16(b, 2)
			if len(b) > 8 {
				paneAct = int(b[8])
			}
		case recSelection:
			if len(b) >= 5 {
				selections[int(b[0])] = [2]int{u16(b, 1), u16(b, 3)}
			}
		}
		return true
	})
	if strSegs != nil {
		if c := sh.Get(pendingRow, pendingCol); c.Kind == model.KindText {
			c.Text = (&segments{segs: strSegs}).str()
		}
	}
	if frozen {
		sh.FreezeCols, sh.FreezeRows = paneX, paneY
	} else {
		paneAct = 3
	}
	if sel, ok := selections[paneAct]; ok {
		sh.ActiveRow, sh.ActiveCol = sel[0], sel[1]
	}
	for _, e := range exps {
		sf, ok := shared[[2]int{e.anchorRow, e.anchorCol}]
		if !ok {
			// Some writers point tExp at the range's corner rather than
			// the cell that carries the record.
			for _, s := range shared {
				if s.rng.R1 == e.anchorRow && s.rng.C1 == e.anchorCol {
					sf, ok = s, true
					break
				}
			}
		}
		switch {
		case !ok:
			badFormula++
		case sf.kind == recTable:
			p.warnf("%s: data tables (What-If Analysis) keep their saved values and are not recalculated", sh.Name)
		case sf.kind == recArray:
			// One formula for the whole range, kept on its top-left cell;
			// the other cells hold their values, as in .xlsx.
			if e.row != sf.rng.R1 || e.col != sf.rng.C1 {
				break
			}
			f, err := p.decodeFormula(sf.rgce, sf.extra, e.row, e.col, false)
			if err != nil {
				badFormula++
				break
			}
			if setFormula(sh, e.row, e.col, f) {
				sh.Get(e.row, e.col).Dynamic = true
				sh.Spills[[2]int{e.row, e.col}] = sf.rng
			}
		default:
			f, err := p.decodeFormula(sf.rgce, sf.extra, e.row, e.col, true)
			if err != nil {
				badFormula++
				break
			}
			setFormula(sh, e.row, e.col, f)
		}
	}
	if badFormula > 0 {
		p.warnf("%s: %d formulas could not be read; their saved values are shown", sh.Name, badFormula)
	}
	p.wb.WarnDropped(sh.Name, dropped)
}

// expCell is a FORMULA cell whose expression lives in the SHRFMLA, ARRAY
// or TABLE record anchored at anchorRow, anchorCol.
type expCell struct {
	row, col, anchorRow, anchorCol int
}

type sharedFmla struct {
	kind  int // recShrFmla, recArray or recTable
	rng   model.Range
	rgce  []byte
	extra []byte
}

// setFormula sets the formula of a loaded cell; cells beyond the sheet
// limits were not loaded and are skipped.
func setFormula(sh *model.Sheet, row, col int, f string) bool {
	if row >= sh.UsedRows() || col >= sh.RowLen(row) {
		return false
	}
	sh.Get(row, col).Formula = f
	return true
}
