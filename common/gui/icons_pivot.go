package gui

import "golang.org/x/exp/shiny/materialdesign/icons"

// Icons of the PivotTable commands (Insert > PivotTable and the
// PivotTable Analyze and Design tabs).
var (
	IconPivotTable    = vectorIcon(pivotTable())
	IconRefreshAll    = mustIcon(icons.ActionCached)
	IconDataSource    = mustIcon(icons.DeviceStorage)
	IconFieldList     = mustIcon(icons.ActionViewList)
	IconClearAll      = mustIcon(icons.ContentClear)
	IconMove          = mustIcon(icons.ActionOpenWith)
	IconSubtotals     = vectorIcon(subtotal())
	IconGrandTotals   = vectorIcon(grandTotals())
	IconReportLayout  = mustIcon(icons.ActionViewQuilt)
	IconBlankRows     = mustIcon(icons.EditorFormatLineSpacing)
	IconFieldHeaders  = mustIcon(icons.ActionLabel)
	IconPlusMinus     = mustIcon(icons.ContentAddCircleOutline)
	IconSelectPivot   = mustIcon(icons.ContentSelectAll)
	IconShowDetails   = mustIcon(icons.ActionZoomIn)
	IconValueSettings = mustIcon(icons.ActionSettings)
)

// pivotTable is a table with a shaded header row and label column.
func pivotTable() (lines, fills [][]pt) {
	lines = [][]pt{
		{{3, 3}, {21, 3}, {21, 21}, {3, 21}, {3, 3}},
		{{9, 3}, {9, 21}}, {{15, 9}, {15, 21}},
		{{3, 9}, {21, 9}}, {{9, 15}, {21, 15}},
	}
	fills = [][]pt{{{3, 3}, {21, 3}, {21, 9}, {3, 9}}, {{3, 9}, {9, 9}, {9, 21}, {3, 21}}}
	return lines, fills
}

// grandTotals is a table with a heavy last row and column.
func grandTotals() (lines, fills [][]pt) {
	lines = [][]pt{
		{{3, 3}, {21, 3}, {21, 21}, {3, 21}, {3, 3}},
		{{9, 3}, {9, 21}}, {{3, 9}, {21, 9}},
	}
	fills = [][]pt{{{15, 3}, {21, 3}, {21, 21}, {15, 21}}, {{3, 15}, {21, 15}, {21, 21}, {3, 21}}}
	return lines, fills
}
