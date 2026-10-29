// Chart command icons, drawn as vector line and fill paths on a 24-unit
// grid.

package gui

import (
	"math"

	"golang.org/x/exp/shiny/materialdesign/icons"
)

// Icons of the chart commands: the kinds of chart (Insert > Charts,
// Change Chart Type) and the Chart Design and Format tabs.
var (
	IconChartColumn   = vectorIcon(chartColumn(false))
	IconChartStacked  = vectorIcon(chartStacked())
	IconChartBar      = vectorIcon(chartColumn(true))
	IconChartLine     = vectorIcon(chartLine(false))
	IconChartMarkers  = vectorIcon(chartLine(true))
	IconChartPie      = vectorIcon(chartPie(0))
	IconChartDoughnut = vectorIcon(chartPie(4.5))
	IconChartArea     = vectorIcon(chartArea())
	IconChartScatter  = vectorIcon(chartScatter())
	IconChartCombo    = vectorIcon(chartCombo())
	IconChartElement  = vectorIcon(chartElement())
	IconChartColors   = mustIcon(icons.ImagePalette)
	IconChartStyles   = mustIcon(icons.ImageBrush)
	IconSwitchRowCol  = mustIcon(icons.ActionSwapHoriz)
	IconSelectData    = mustIcon(icons.DeviceStorage)
	IconChangeType    = mustIcon(icons.EditorInsertChart)
	IconShapeOutline  = mustIcon(icons.EditorBorderColor)
)

// chartColumn is three columns on an axis, or bars beside one.
func chartColumn(bars bool) (lines, fills [][]pt) {
	box := func(x0, y0, x1, y1 float32) []pt {
		if bars {
			x0, y0, x1, y1 = y0, x0, y1, x1
			x0, x1 = 24-x1, 24-x0
		}
		return []pt{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
	}
	fills = [][]pt{box(4, 11, 8, 20), box(10, 5, 14, 20), box(16, 14, 20, 20)}
	lines = [][]pt{{{2, 21}, {22, 21}}}
	if bars {
		lines = [][]pt{{{3, 2}, {3, 22}}}
	}
	return lines, fills
}

// chartStacked is columns split in two.
func chartStacked() (lines, fills [][]pt) {
	lines = [][]pt{{{2, 21}, {22, 21}}, {{4, 15}, {8, 15}}, {{10, 11}, {14, 11}}, {{16, 17}, {20, 17}}}
	fills = [][]pt{{{4, 9}, {8, 9}, {8, 14}, {4, 14}}, {{4, 16}, {8, 16}, {8, 20}, {4, 20}},
		{{10, 4}, {14, 4}, {14, 10}, {10, 10}}, {{10, 12}, {14, 12}, {14, 20}, {10, 20}},
		{{16, 12}, {20, 12}, {20, 16}, {16, 16}}, {{16, 18}, {20, 18}, {20, 20}, {16, 20}}}
	return lines, fills
}

// chartLine is a line over the axes, with square markers if asked.
func chartLine(markers bool) (lines, fills [][]pt) {
	path := []pt{{4, 17}, {9, 10}, {14, 14}, {20, 5}}
	lines = [][]pt{{{2, 3}, {2, 21}, {22, 21}}, path}
	if markers {
		for _, p := range path {
			fills = append(fills, []pt{{p[0] - 2, p[1] - 2}, {p[0] + 2, p[1] - 2}, {p[0] + 2, p[1] + 2}, {p[0] - 2, p[1] + 2}})
		}
	}
	return lines, fills
}

// chartPie is a pie with a slice pulled out, or a ring when hole > 0.
func chartPie(hole float32) (lines, fills [][]pt) {
	arc := func(cx, cy, r float32, a0, a1 float64) []pt {
		var out []pt
		for a := a0; a <= a1+1e-9; a += (a1 - a0) / 24 {
			out = append(out, pt{cx + r*float32(math.Sin(a)), cy - r*float32(math.Cos(a))})
		}
		return out
	}
	if hole > 0 {
		for _, r := range []float32{9, 9 - hole/2 + 1, 9 - hole + 2} {
			lines = append(lines, arc(12, 12, r, 0, 2*math.Pi))
		}
		return lines, nil
	}
	body := append([]pt{{11, 12}}, arc(11, 12, 9, math.Pi/2, 2*math.Pi)...)
	slice := append([]pt{{13, 10}}, arc(13, 10, 9, 0, math.Pi/2)...)
	return nil, [][]pt{body, slice}
}

// chartArea is a filled area over an axis.
func chartArea() (lines, fills [][]pt) {
	return [][]pt{{{2, 3}, {2, 21}, {22, 21}}}, [][]pt{{{3, 20}, {3, 13}, {9, 7}, {14, 12}, {21, 5}, {21, 20}}}
}

// chartScatter is dots over the axes.
func chartScatter() (lines, fills [][]pt) {
	lines = [][]pt{{{2, 3}, {2, 21}, {22, 21}}}
	for _, p := range []pt{{6, 16}, {9, 12}, {12, 14}, {14, 8}, {18, 10}, {19, 5}} {
		fills = append(fills, []pt{{p[0] - 1.5, p[1] - 1.5}, {p[0] + 1.5, p[1] - 1.5}, {p[0] + 1.5, p[1] + 1.5}, {p[0] - 1.5, p[1] + 1.5}})
	}
	return lines, fills
}

// chartCombo is columns with a line over them.
func chartCombo() (lines, fills [][]pt) {
	lines, fills = chartColumn(false)
	lines = append(lines, []pt{{3, 9}, {9, 6}, {15, 10}, {21, 4}})
	return lines, fills
}

// chartElement is a column chart with a plus.
func chartElement() (lines, fills [][]pt) {
	lines = [][]pt{{{2, 21}, {22, 21}}, {{17, 3}, {17, 11}}, {{13, 7}, {21, 7}}}
	fills = [][]pt{{{4, 12}, {8, 12}, {8, 20}, {4, 20}}, {{10, 8}, {13, 8}, {13, 20}, {10, 20}}, {{15, 14}, {19, 14}, {19, 20}, {15, 20}}}
	return lines, fills
}
