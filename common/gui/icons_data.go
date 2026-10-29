package gui

import (
	"math"

	"golang.org/x/exp/shiny/iconvg"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// Icons of Excel's Data tab. Material Design has no funnel or A-Z sort
// icons, so these are drawn here as IconVG vectors on a 24×24 grid:
// polylines (stroked as quads) and filled polygons.
var (
	IconSortAZ      = vectorIcon(sortLetters(false))
	IconSortZA      = vectorIcon(sortLetters(true))
	IconSortCustom  = vectorIcon(customSort())
	IconFilter      = vectorIcon(nil, [][]pt{funnel(3, 3, 18)})
	IconFilterClear = vectorIcon(clearFilter())
	IconReapply     = mustIcon(icons.NavigationRefresh)
	IconTextToCols  = vectorIcon(textToColumns())
	IconRemoveDups  = vectorIcon(removeDuplicates())
	IconValidation  = vectorIcon(dataValidation())
	IconGroup       = vectorIcon(outlineGroup(false))
	IconUngroup     = vectorIcon(outlineGroup(true))
	IconSubtotal    = vectorIcon(subtotal())
	IconShowDetail  = vectorIcon(outlineDetail(true))
	IconHideDetail  = vectorIcon(outlineDetail(false))
)

type pt [2]float32

// vectorIcon encodes polylines (2 units wide) and filled polygons.
func vectorIcon(lines, fills [][]pt) *Icon {
	var e iconvg.Encoder
	e.Reset(iconvg.Metadata{
		ViewBox: iconvg.Rectangle{Max: [2]float32{24, 24}},
		Palette: iconvg.DefaultPalette,
	})
	poly := func(p []pt) {
		e.StartPath(0, p[0][0], p[0][1])
		for _, q := range p[1:] {
			e.AbsLineTo(q[0], q[1])
		}
		e.ClosePathEndPath()
	}
	const h = 1 // half the stroke width
	for _, l := range lines {
		for i := 0; i+1 < len(l); i++ {
			a, b := l[i], l[i+1]
			dx, dy := b[0]-a[0], b[1]-a[1]
			n := float32(math.Hypot(float64(dx), float64(dy)))
			if n == 0 {
				continue
			}
			ux, uy := dx/n*h, dy/n*h // along, extended by h at both ends for square joints
			a = pt{a[0] - ux, a[1] - uy}
			b = pt{b[0] + ux, b[1] + uy}
			poly([]pt{{a[0] - uy, a[1] + ux}, {b[0] - uy, b[1] + ux}, {b[0] + uy, b[1] - ux}, {a[0] + uy, a[1] - ux}})
		}
	}
	for _, f := range fills {
		poly(f)
	}
	data, err := e.Bytes()
	if err != nil {
		panic(err)
	}
	return mustIcon(data)
}

// letterA and letterZ are 7 wide and 9 tall with the top-left at x, y.
func letterA(x, y float32) [][]pt {
	return [][]pt{{{x, y + 9}, {x + 3.5, y}, {x + 7, y + 9}}, {{x + 1.6, y + 6}, {x + 5.4, y + 6}}}
}

func letterZ(x, y float32) [][]pt {
	return [][]pt{{{x, y}, {x + 7, y}, {x, y + 9}, {x + 7, y + 9}}}
}

// downArrow is a shaft at x from y0 down to a head ending at y1.
func downArrow(x, y0, y1 float32) ([]pt, []pt) {
	return []pt{{x, y0}, {x, y1 - 4}}, []pt{{x - 4, y1 - 5}, {x + 4, y1 - 5}, {x, y1}}
}

// sortLetters is Excel's Sort A to Z (A over Z, arrow down) or Z to A.
func sortLetters(desc bool) (lines, fills [][]pt) {
	top, bottom := letterA(2, 2), letterZ(2, 13)
	if desc {
		top, bottom = letterZ(2, 2), letterA(2, 13)
	}
	shaft, head := downArrow(17, 2, 22)
	lines = append(append(top, bottom...), shaft)
	return lines, [][]pt{head}
}

// customSort is the Sort dialog's icon: a small table with Z A over it.
func customSort() (lines, fills [][]pt) {
	lines = append(letterZ(2, 1), letterA(11, 1)...)
	for _, y := range []float32{15, 19, 23} {
		lines = append(lines, []pt{{2, y}, {22, y}})
	}
	lines = append(lines, []pt{{2, 13}, {2, 23}}, []pt{{12, 13}, {12, 23}}, []pt{{22, 13}, {22, 23}})
	return lines, nil
}

// funnel is a filter funnel with its top-left at x, y and width w.
func funnel(x, y, w float32) []pt {
	s := w / 18
	return []pt{{x, y}, {x + w, y}, {x + 11*s, y + 8*s}, {x + 11*s, y + 17*s}, {x + 7*s, y + 15*s}, {x + 7*s, y + 8*s}}
}

// clearFilter is a funnel with a cross: Clear.
func clearFilter() (lines, fills [][]pt) {
	return [][]pt{{{15, 15}, {22, 22}}, {{22, 15}, {15, 22}}}, [][]pt{funnel(1, 2, 15)}
}

// textToColumns is a column of text split into two by an arrow.
func textToColumns() (lines, fills [][]pt) {
	lines = [][]pt{
		{{2, 3}, {9, 3}, {9, 21}, {2, 21}, {2, 3}},
		{{4, 8}, {7, 8}}, {{4, 12}, {7, 12}}, {{4, 16}, {7, 16}},
		{{15, 3}, {22, 3}, {22, 21}, {15, 21}, {15, 3}},
		{{18.5, 3}, {18.5, 21}},
		{{10, 12}, {12, 12}},
	}
	return lines, [][]pt{{{11, 8}, {15, 12}, {11, 16}}}
}

// removeDuplicates is a table whose rows carry a cross.
func removeDuplicates() (lines, fills [][]pt) {
	lines = [][]pt{
		{{2, 3}, {14, 3}, {14, 21}, {2, 21}, {2, 3}},
		{{2, 9}, {14, 9}}, {{2, 15}, {14, 15}},
		{{8, 3}, {8, 21}},
		{{16, 11}, {22, 17}}, {{22, 11}, {16, 17}},
	}
	return lines, nil
}

// dataValidation is a cell with a check mark and a no-entry circle.
func dataValidation() (lines, fills [][]pt) {
	lines = [][]pt{
		{{2, 3}, {16, 3}, {16, 13}, {2, 13}, {2, 3}},
		{{4.5, 8}, {7.5, 11}, {13.5, 5}},
		{{14, 15}, {22, 15}, {22, 22}, {14, 22}, {14, 15}},
		{{15.5, 20.5}, {20.5, 16.5}},
	}
	return lines, nil
}

// outlineRows are three rows of data at the right.
func outlineRows() [][]pt {
	return [][]pt{{{11, 5}, {22, 5}}, {{11, 12}, {22, 12}}, {{11, 19}, {22, 19}}}
}

// outlineGroup is rows joined by the outline's bracket (Group), or with
// the bracket crossed out (Ungroup).
func outlineGroup(ungroup bool) (lines, fills [][]pt) {
	lines = outlineRows()
	if ungroup {
		return append(lines, []pt{{2, 9}, {8, 15}}, []pt{{8, 9}, {2, 15}}), nil
	}
	return append(lines, []pt{{8, 5}, {4, 5}, {4, 19}, {8, 19}}), nil
}

// subtotal is a sigma over a total line.
func subtotal() (lines, fills [][]pt) {
	return [][]pt{{{17, 3}, {5, 3}, {12, 11}, {5, 19}, {17, 19}}, {{3, 22.5}, {21, 22.5}}}, nil
}

// outlineDetail is rows with the outline's + (Show Detail) or - (Hide
// Detail) button.
func outlineDetail(plus bool) (lines, fills [][]pt) {
	lines = append(outlineRows(), []pt{{1.5, 7.5}, {8.5, 7.5}, {8.5, 14.5}, {1.5, 14.5}, {1.5, 7.5}}, []pt{{3.5, 11}, {6.5, 11}})
	if plus {
		lines = append(lines, []pt{{5, 9.5}, {5, 12.5}})
	}
	return lines, nil
}
