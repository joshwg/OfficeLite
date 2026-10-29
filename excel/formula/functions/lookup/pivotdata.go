package lookup

import (
	f "officelite/excel/formula"
)

// pivotData is implemented by contexts that know the workbook's
// PivotTables (the calc engine); elsewhere GETPIVOTDATA is #REF!.
type pivotData interface {
	PivotData(sheet, row, col int, dataField string, pairs []f.Value) f.Value
}

// GETPIVOTDATA(data_field, pivot_table, [field1, item1], ...): the value
// of a PivotTable for the given items (totals for the fields left out).
func getPivotData(e *f.Env, a []f.Value) f.Value {
	pd, ok := e.Ctx.(pivotData)
	ref, isRef := a[1].(f.Ref)
	if !ok || !isRef {
		return f.ErrRef
	}
	name, ok := a[0].(f.String)
	if !ok {
		if err, isErr := a[0].(f.Error); isErr {
			return err
		}
		return f.ErrRef
	}
	pairs := a[2:]
	if len(pairs)%2 != 0 {
		return f.ErrRef
	}
	for _, p := range pairs {
		if err, isErr := p.(f.Error); isErr {
			return err
		}
	}
	return pd.PivotData(ref.Sheet, ref.R1, ref.C1, string(name), pairs)
}
