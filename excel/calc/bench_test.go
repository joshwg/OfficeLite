package calc

import (
	"fmt"
	"testing"

	"officelite/excel/formula/functions"
	"officelite/excel/model"
)

// bigBook has n rows of a value, a dependent formula and a running total:
// 3n cells, 2n formulas.
func bigBook(n int) *model.Workbook {
	sh := model.NewSheet("Sheet1")
	for r := 0; r < n; r++ {
		sh.Set(r, 0, model.Cell{Kind: model.KindNumber, Num: float64(r), Text: fmt.Sprint(r)})
		sh.Set(r, 1, model.Cell{Formula: fmt.Sprintf("A%d*2+1", r+1)})
		if r == 0 {
			sh.Set(r, 2, model.Cell{Formula: "B1"})
		} else {
			sh.Set(r, 2, model.Cell{Formula: fmt.Sprintf("C%d+B%d", r, r+1)})
		}
	}
	return &model.Workbook{Sheets: []*model.Sheet{sh}, Styles: []model.Style{model.DefaultStyle()}}
}

func BenchmarkFullRecalc100k(b *testing.B) {
	wb := bigBook(50000)
	for b.Loop() {
		Recalculate(wb, functions.Default(), Full, now)
	}
}

func BenchmarkIncrementalEdit100k(b *testing.B) {
	wb := bigBook(50000)
	c := NewCalculator(wb, functions.Default())
	c.Full(now)
	sh := wb.Sheets[0]
	i := 0
	for b.Loop() {
		// Edit a value near the bottom: its row formula and the running
		// totals below it recalculate.
		i++
		sh.Set(49000, 0, model.Cell{Kind: model.KindNumber, Num: float64(i), Text: fmt.Sprint(i)})
		c.Update([]Area{CellArea(0, 49000, 0)}, now)
	}
}

func BenchmarkIncrementalEditTop100k(b *testing.B) {
	wb := bigBook(50000)
	c := NewCalculator(wb, functions.Default())
	c.Full(now)
	sh := wb.Sheets[0]
	i := 0
	for b.Loop() {
		// Editing A1 changes every running total: the worst case.
		i++
		sh.Set(0, 0, model.Cell{Kind: model.KindNumber, Num: float64(i), Text: fmt.Sprint(i)})
		c.Update([]Area{CellArea(0, 0, 0)}, now)
	}
}

func BenchmarkBuild100k(b *testing.B) {
	wb := bigBook(50000)
	for b.Loop() {
		c := NewCalculator(wb, functions.Default())
		c.build()
	}
}
