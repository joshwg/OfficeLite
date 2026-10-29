package pagesfile

import (
	"fmt"

	"officelite/common/iwork"
	"officelite/word/model"
)

// maxTableRows bounds the rows of a table brought into a document.
const maxTableRows = 5000

// table converts a Numbers-style table to a Word table: cell text with
// the cells' text styles, fills, merged cells and column widths.
func (r *reader) table(t *iwork.Table) *model.Table {
	tbl := &model.Table{Borders: true}
	for c := range t.Cols {
		w := t.DefaultColWidth
		if h, ok := t.ColInfo[c]; ok && h.Size > 0 {
			w = h.Size
		}
		if t.ColInfo[c].Hidden {
			w = 0
		}
		tbl.Columns = append(tbl.Columns, w)
	}
	merged := map[[2]int]iwork.Range{} // every cell of a merge -> the merge
	for _, m := range t.Merges {
		for row := m.R1; row <= m.R2; row++ {
			for col := m.C1; col <= m.C2; col++ {
				merged[[2]int{row, col}] = m
			}
		}
	}
	rows := t.Rows
	if rows > maxTableRows {
		rows = maxTableRows
		r.out.Warnings = append(r.out.Warnings, fmt.Sprintf("Table %q: only its first %d rows were imported", t.Name, maxTableRows))
	}
	for row := range rows {
		tr := &model.TableRow{Header: row < t.HeaderRows}
		if h := t.RowInfo[row]; h.Size > 0 {
			tr.Height = h.Size
		}
		for col := 0; col < t.Cols; col++ {
			m, inMerge := merged[[2]int{row, col}]
			cell := &model.TableCell{Span: 1, Width: tbl.Columns[col]}
			if inMerge {
				if col != m.C1 {
					continue // covered by the merge's first column
				}
				cell.Span = min(m.C2, t.Cols-1) - m.C1 + 1
				for k := m.C1 + 1; k <= m.C2 && k < len(tbl.Columns); k++ {
					cell.Width += tbl.Columns[k]
				}
				switch {
				case m.R2 > m.R1 && row == m.R1:
					cell.VMerge = 1
				case row > m.R1:
					cell.VMerge = 2
				}
			}
			r.fillCell(cell, t, row, col, inMerge && row > m.R1)
			tr.Cells = append(tr.Cells, cell)
		}
		tbl.Rows = append(tbl.Rows, tr)
	}
	return tbl
}

// fillCell sets a Word cell's content and look from table cell row, col.
func (r *reader) fillCell(cell *model.TableCell, t *iwork.Table, row, col int, covered bool) {
	c := t.Cell(row, col)
	area := t.Area(row, col)
	textIDs := []uint64{t.AreaTextStyle[area], t.ColInfo[col].TextStyle, t.RowInfo[row].TextStyle}
	cellIDs := []uint64{t.AreaCellStyle[area], t.ColInfo[col].CellStyle, t.RowInfo[row].CellStyle}
	if c != nil {
		textIDs = append(textIDs, c.TextStyle)
		cellIDs = append(cellIDs, c.CellStyle)
	}
	var text iwork.TextProps
	var para iwork.ParaProps
	for _, id := range textIDs {
		if s := r.doc.Style(id); s != nil {
			text = text.Merge(s.Text)
			para = para.Merge(s.Para)
		}
	}
	if t.Banded(row) {
		cell.Shading = t.BandFill.Hex()
	}
	for _, id := range cellIDs {
		if s := r.doc.Style(id); s != nil {
			if s.Cell.Fill.Set && s.Cell.Fill.A > 0 {
				cell.Shading = s.Cell.Fill.Hex()
			} else if s.Cell.Fill.Set && !t.Banded(row) {
				cell.Shading = ""
			}
			switch s.Cell.VAlign {
			case 1:
				cell.VAlign = 0
			case 2:
				cell.VAlign = 1
			case 3:
				cell.VAlign = 2
			}
		}
	}
	pp := paraProps(para)
	pp.SpaceBefore, pp.SpaceAfter = model.F(0), model.F(0)
	if para.Align == nil || *para.Align == iwork.AlignNatural {
		pp.Align = model.AlignLeft
		if c != nil && c.Type != iwork.CellText && c.Type != iwork.CellRichText && c.Type != iwork.CellEmpty {
			pp.Align = model.AlignRight // numbers align right, as in Numbers
		}
	}
	rp := runProps(text)
	if c != nil && c.Type == iwork.CellRichText && c.Rich != 0 && !covered {
		for _, b := range r.storage(c.Rich) {
			if p, ok := b.(*model.Paragraph); ok {
				p.Props = pp.Merge(p.Props)
				for i := range p.Runs {
					p.Runs[i].Props = rp.Merge(p.Runs[i].Props)
				}
				p.Mark = rp.Merge(p.Mark)
			}
			cell.Blocks = append(cell.Blocks, b)
		}
		if len(cell.Blocks) > 0 {
			return
		}
	}
	p := &model.Paragraph{Props: pp, Mark: rp}
	if c != nil && !covered {
		if s := r.doc.Display(c); s != "" {
			p.Runs = append(p.Runs, model.Run{Kind: model.RunText, Text: s, Props: rp})
		}
	}
	cell.Blocks = []model.Block{p}
}
