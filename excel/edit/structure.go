package edit

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"officelite/excel/calc"
	"officelite/excel/formula"
	"officelite/excel/model"
)

type opKind int

const (
	insertRows opKind = iota
	deleteRows
	insertCols
	deleteCols
	insertSheet
	deleteSheet
	renameSheet
	shiftCells // see cells.go
	sortRows   // see sort.go
	rowSet     // see subtotal.go
)

// structural is an insertion or deletion of rows, columns or sheets, or a
// sheet rename. apply fills in what revert needs.
type structural struct {
	kind  opKind
	sheet int // the sheet changed (for insertSheet, the new sheet's index)
	at, n int
	name  string       // insertSheet, renameSheet: the new name
	chart *model.Chart // insertSheet: a chart sheet's chart (chartedit.go)

	saved    *model.Sheet        // the sheet before (rows/cols), or the deleted sheet
	oldName  string              // renameSheet
	formulas []cellChange        // formulas elsewhere whose text changed
	names    []model.DefinedName // the defined names before
	// valids are the data validation rules, by sheet, of the sheets
	// whose rule formulas rewriteFormulas changed; pivots likewise the
	// PivotTables whose source rewriteFormulas changed.
	valids map[int][]model.Validation
	pivots map[int][]*model.PivotTable
	charts map[int]sheetCharts // likewise the charts whose references changed

	cells *cellsOp  // shiftCells
	sort  *SortSpec // sortRows
	rows  []int     // rowSet: rows inserted before (n > 0) or deleted
}

// ErrShiftOffSheet is Excel's refusal to push data past the sheet's end.
var ErrShiftOffSheet = errors.New("To prevent possible loss of data, nonblank cells can't be shifted off the worksheet")

// InsertRows inserts n rows before row at on sheet.
func (b *Book) InsertRows(sheet, at, n int) (calc.Result, error) {
	defer b.Op("Insert Rows")()
	return b.structural(&structural{kind: insertRows, sheet: sheet, at: at, n: n},
		Where{sheet, model.Range{R1: at, R2: at + n - 1, C2: model.MaxCols - 1}})
}

// DeleteRows deletes rows at..at+n-1 on sheet.
func (b *Book) DeleteRows(sheet, at, n int) (calc.Result, error) {
	defer b.Op("Delete Rows")()
	return b.structural(&structural{kind: deleteRows, sheet: sheet, at: at, n: n},
		Where{sheet, model.Range{R1: at, R2: at + n - 1, C2: model.MaxCols - 1}})
}

// InsertCols inserts n columns before column at on sheet.
func (b *Book) InsertCols(sheet, at, n int) (calc.Result, error) {
	defer b.Op("Insert Columns")()
	return b.structural(&structural{kind: insertCols, sheet: sheet, at: at, n: n},
		Where{sheet, model.Range{C1: at, C2: at + n - 1, R2: model.MaxRows - 1}})
}

// DeleteCols deletes columns at..at+n-1 on sheet.
func (b *Book) DeleteCols(sheet, at, n int) (calc.Result, error) {
	defer b.Op("Delete Columns")()
	return b.structural(&structural{kind: deleteCols, sheet: sheet, at: at, n: n},
		Where{sheet, model.Range{C1: at, C2: at + n - 1, R2: model.MaxRows - 1}})
}

// InsertSheet adds an empty sheet before sheet index at, named Sheet<n>
// with the first free n, and returns its name.
func (b *Book) InsertSheet(at int) (string, error) {
	defer b.Op("Insert Sheet")()
	name := ""
	for i := len(b.WB.Sheets) + 1; ; i++ {
		name = fmt.Sprintf("Sheet%d", i)
		if b.sheetIndex(name) < 0 {
			break
		}
	}
	_, err := b.structural(&structural{kind: insertSheet, sheet: at, name: name}, Where{Sheet: at})
	return name, err
}

// DeleteSheet deletes a sheet. References to it become #REF!.
func (b *Book) DeleteSheet(sheet int) (calc.Result, error) {
	defer b.Op("Delete Sheet")()
	visible := 0
	for i, sh := range b.WB.Sheets {
		if !sh.Hidden && i != sheet {
			visible++
		}
	}
	if visible == 0 {
		return calc.Result{}, errors.New("A workbook must contain at least one visible worksheet")
	}
	// Undo selects the sheet put back; redo the one now at its place (the
	// next sheet, or the last), as deleting does (the caller clamps).
	return b.structural(&structural{kind: deleteSheet, sheet: sheet}, Where{Sheet: sheet})
}

// RenameSheet renames a sheet and the references to it.
func (b *Book) RenameSheet(sheet int, name string) (calc.Result, error) {
	defer b.Op("Rename Sheet")()
	if err := ValidSheetName(name); err != nil {
		return calc.Result{}, err
	}
	if i := b.sheetIndex(name); i >= 0 && i != sheet {
		return calc.Result{}, fmt.Errorf("That name is already taken. Try a different one.")
	}
	if b.WB.Sheets[sheet].Name == name {
		return calc.Result{}, nil
	}
	return b.structural(&structural{kind: renameSheet, sheet: sheet, name: name}, Where{Sheet: sheet})
}

// ValidSheetName checks a sheet name against Excel's rules.
func ValidSheetName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New("A sheet name can't be blank")
	case len([]rune(name)) > 31:
		return errors.New("A sheet name can't be longer than 31 characters")
	case strings.ContainsAny(name, `:\/?*[]`):
		return errors.New(`A sheet name can't contain : \ / ? * [ or ]`)
	case name[0] == '\'' || name[len(name)-1] == '\'':
		return errors.New("A sheet name can't begin or end with an apostrophe")
	case strings.EqualFold(name, "History"):
		return errors.New("History is a reserved sheet name")
	}
	return nil
}

func (b *Book) sheetIndex(name string) int {
	for i, sh := range b.WB.Sheets {
		if strings.EqualFold(sh.Name, name) {
			return i
		}
	}
	return -1
}

func (b *Book) structural(op *structural, where Where) (calc.Result, error) {
	res, err := b.applyStructural(op)
	if err != nil {
		return res, err
	}
	b.record(&change{where: where, st: op})
	return res, nil
}

// rewriteFormulas applies fn to every formula of the workbook except those
// on sheet skip, recording the changes in op.formulas, and to the defined
// names.
func (b *Book) rewriteFormulas(op *structural, skip int, fn func(f, home string) string) {
	op.formulas = nil
	op.valids = nil
	op.pivots = nil
	op.charts = nil
	for si, sh := range b.WB.Sheets {
		if si == skip {
			continue
		}
		if before := model.CloneValidations(sh.Validations); adjustValidations(sh, fn) {
			if op.valids == nil {
				op.valids = map[int][]model.Validation{}
			}
			op.valids[si] = before
		}
		if before := model.ClonePivots(sh.Pivots); adjustPivotSources(sh, fn) {
			if op.pivots == nil {
				op.pivots = map[int][]*model.PivotTable{}
			}
			op.pivots[si] = before
		}
		if before := chartsOf(sh); adjustCharts(sh, fn) {
			if op.charts == nil {
				op.charts = map[int]sheetCharts{}
			}
			op.charts[si] = before
		}
		for r := 0; r < sh.UsedRows(); r++ {
			for c := 0; c < sh.RowLen(r); c++ {
				cell := sh.Get(r, c)
				if cell.Formula == "" {
					continue
				}
				if g := fn(cell.Formula, sh.Name); g != cell.Formula {
					after := *cell
					after.Formula = g
					op.formulas = append(op.formulas, cellChange{si, r, c, *cell, after})
					cell.Formula = g
				}
			}
		}
	}
	for i := range b.WB.Names {
		b.WB.Names[i].Formula = fn(b.WB.Names[i].Formula, "")
	}
}

// adjustSheet rewrites the formulas on sheet itself (which revert
// restores from op.saved).
func adjustSheet(sh *model.Sheet, fn func(f, home string) string) {
	for r := 0; r < sh.UsedRows(); r++ {
		for c := 0; c < sh.RowLen(r); c++ {
			if cell := sh.Get(r, c); cell.Formula != "" {
				cell.Formula = fn(cell.Formula, sh.Name)
			}
		}
	}
	adjustValidations(sh, fn)
	adjustPivotSources(sh, fn)
	adjustCharts(sh, fn)
}

// adjustPivotSources rewrites the source references of sh's PivotTables;
// it reports whether any changed.
func adjustPivotSources(sh *model.Sheet, fn func(f, home string) string) bool {
	changed := false
	for _, p := range sh.Pivots {
		if g := fn(p.Source, sh.Name); g != p.Source {
			p.Source, changed = g, true
		}
	}
	return changed
}

// adjustValidations rewrites the formulas of sh's data validation rules;
// it reports whether any changed.
func adjustValidations(sh *model.Sheet, fn func(f, home string) string) bool {
	changed := false
	for i := range sh.Validations {
		v := &sh.Validations[i]
		for _, f := range []*string{&v.Formula1, &v.Formula2} {
			if *f == "" {
				continue
			}
			if g := fn(*f, sh.Name); g != *f {
				*f, changed = g, true
			}
		}
	}
	return changed
}

func (b *Book) applyStructural(op *structural) (calc.Result, error) {
	wb := b.WB
	op.names = slices.Clone(wb.Names)
	switch op.kind {
	case shiftCells:
		return b.applyShiftCells(op)
	case sortRows:
		return b.applySort(op)
	case rowSet:
		return b.applyRowSet(op)
	case insertRows, deleteRows, insertCols, deleteCols:
		sh := wb.Sheets[op.sheet]
		rows := op.kind == insertRows || op.kind == deleteRows
		n := op.n
		if op.kind == deleteRows || op.kind == deleteCols {
			n = -n
		} else if !fits(sh, rows, n) {
			return calc.Result{}, ErrShiftOffSheet
		}
		if pivotCut(sh, rows, op.at, n) {
			return calc.Result{}, ErrPivotChange
		}
		op.saved = sh.Clone()
		adjust := func(f, home string) string {
			if rows {
				return formula.AdjustRows(f, home, sh.Name, op.at, n)
			}
			return formula.AdjustCols(f, home, sh.Name, op.at, n)
		}
		b.rewriteFormulas(op, op.sheet, adjust)
		adjustSheet(sh, adjust)
		switch op.kind {
		case insertRows:
			sh.InsertRows(op.at, op.n)
		case deleteRows:
			sh.DeleteRows(op.at, op.n)
		case insertCols:
			sh.InsertCols(op.at, op.n)
		case deleteCols:
			sh.DeleteCols(op.at, op.n)
		}
	case insertSheet:
		sh := model.NewSheet(op.name)
		sh.ChartSheet = op.chart.Clone()
		wb.Sheets = slices.Insert(wb.Sheets, op.sheet, sh)
		for i := range wb.Names {
			if wb.Names[i].Sheet >= op.sheet {
				wb.Names[i].Sheet++
			}
		}
	case deleteSheet:
		sh := wb.Sheets[op.sheet]
		op.saved = sh
		b.rewriteFormulas(op, op.sheet, func(f, _ string) string { return formula.DeleteSheetRefs(f, sh.Name) })
		wb.Sheets = slices.Delete(wb.Sheets, op.sheet, op.sheet+1)
		names := wb.Names[:0]
		for _, n := range wb.Names {
			switch {
			case n.Sheet == op.sheet:
				continue
			case n.Sheet > op.sheet:
				n.Sheet--
			}
			names = append(names, n)
		}
		wb.Names = names
	case renameSheet:
		sh := wb.Sheets[op.sheet]
		op.oldName = sh.Name
		b.rewriteFormulas(op, -1, func(f, _ string) string { return formula.RenameSheetRefs(f, op.oldName, op.name) })
		sh.Name = op.name
	}
	return b.recalculateAll(), nil
}

func (b *Book) revertStructural(op *structural) calc.Result {
	wb := b.WB
	switch op.kind {
	case insertRows, deleteRows, insertCols, deleteCols, shiftCells, sortRows, rowSet:
		*wb.Sheets[op.sheet] = *op.saved
	case insertSheet:
		wb.Sheets = slices.Delete(wb.Sheets, op.sheet, op.sheet+1)
	case deleteSheet:
		wb.Sheets = slices.Insert(wb.Sheets, op.sheet, op.saved)
	case renameSheet:
		wb.Sheets[op.sheet].Name = op.oldName
	}
	op.saved = nil
	for _, c := range op.formulas {
		wb.Sheets[c.sheet].Get(c.row, c.col).Formula = c.before.Formula
	}
	for si, vs := range op.valids {
		wb.Sheets[si].Validations = model.CloneValidations(vs)
	}
	for si, ps := range op.pivots {
		wb.Sheets[si].Pivots = model.ClonePivots(ps)
	}
	for si, cs := range op.charts {
		cs.restore(wb.Sheets[si])
	}
	wb.Names = slices.Clone(op.names)
	return b.recalculateAll()
}

// recalculateAll recalculates everything after a structural change, which
// moves cells and renumbers sheets.
func (b *Book) recalculateAll() calc.Result {
	b.calc.Invalidate()
	return b.calc.Full(b.now())
}

// fits reports whether n rows (or columns) can be inserted without
// pushing nonblank cells off the sheet.
func fits(sh *model.Sheet, rows bool, n int) bool {
	if rows {
		for r := model.MaxRows - n; r < sh.UsedRows(); r++ {
			for c := 0; c < sh.RowLen(r); c++ {
				if cell := sh.Get(r, c); cell.Formula != "" || cell.Kind != model.KindEmpty {
					return false
				}
			}
		}
		return true
	}
	for r := 0; r < sh.UsedRows(); r++ {
		for c := model.MaxCols - n; c < sh.RowLen(r); c++ {
			if cell := sh.Get(r, c); cell.Formula != "" || cell.Kind != model.KindEmpty {
				return false
			}
		}
	}
	return true
}
