// Package model is the in-memory representation of a workbook, independent of
// the file format it was read from. Readers fill it in; the UI only draws it.
package model

import (
	"fmt"
	"image/color"
	"math"
	"strings"
)

// Sheet size limits. Anything beyond these is dropped on load with a warning.
const (
	MaxRows = 262144 // 256K
	MaxCols = 2048   // 2K
)

// DefaultRowHeightPx is Excel's default row height at 96 DPI (Calibri 11).
const DefaultRowHeightPx = 20

// Column widths are stored the way Excel stores them: in characters, i.e.
// multiples of the maximum digit width (the pixel width of '0') of the
// workbook's default font. They only become pixels once that font is
// measured, so a Verdana 12 workbook gets wider columns than a Calibri 11
// one with the same stored widths.

// DefaultDigitWidth is the maximum digit width of Calibri 11 and Arial 10,
// used when the default font cannot be measured.
const DefaultDigitWidth = 7

// DefaultBaseColWidth is Excel's default column width in characters,
// excluding cell padding.
const DefaultBaseColWidth = 8

// ColWidthToPx converts a column width in characters (cell padding
// included) to pixels for a font whose digit width is mdw pixels.
func ColWidthToPx(chars, mdw float64) float32 {
	return float32(math.Trunc((256*chars + math.Trunc(128/mdw)) / 256 * mdw))
}

// BaseColWidthToPx converts a default width in characters excluding
// padding; Excel adds the padding and rounds up to a multiple of 8 pixels.
func BaseColWidthToPx(chars, mdw float64) float32 {
	return float32(math.Ceil((chars*mdw+5)/8) * 8)
}

// PointsToPx converts points to 96-DPI pixels.
func PointsToPx(pt float64) float32 { return float32(math.Round(pt * 96 / 72)) }

// Kind is the type of a cell's value, which also drives General alignment.
type Kind uint8

const (
	KindEmpty Kind = iota
	KindText
	KindNumber
	KindBool
	KindError
)

// Cell is one sheet cell. Text is the value as Excel would display it (number
// format already applied); Num holds the raw value for numeric cells.
type Cell struct {
	Text    string
	Formula string // without the leading '='; empty if the cell is a constant
	Num     float64
	Style   uint32 // index into Workbook.Styles
	Kind    Kind
	// Dynamic marks a dynamic-array formula: its array result spills into
	// the neighbouring cells instead of being reduced to one value.
	Dynamic bool
}

// HAlign is horizontal alignment. HAlignGeneral means "by value type".
type HAlign uint8

const (
	HAlignGeneral HAlign = iota
	HAlignLeft
	HAlignCenter
	HAlignRight
	HAlignFill
	HAlignJustify
	HAlignCenterContinuous
	HAlignDistributed
)

type VAlign uint8

const (
	VAlignBottom VAlign = iota // Excel's default
	VAlignTop
	VAlignCenter
	VAlignJustify
	VAlignDistributed
)

// BorderStyle uses the same numbering as both BIFF8 and OOXML (and excelize).
type BorderStyle uint8

const (
	BorderNone BorderStyle = iota
	BorderThin
	BorderMedium
	BorderDashed
	BorderDotted
	BorderThick
	BorderDouble
	BorderHair
	BorderMediumDashed
	BorderDashDot
	BorderMediumDashDot
	BorderDashDotDot
	BorderMediumDashDotDot
	BorderSlantDashDot
)

// Edge is one side of a cell border.
type Edge struct {
	Style BorderStyle
	Color color.NRGBA
}

// Border sides, indexes into Style.Border.
const (
	Left = iota
	Top
	Right
	Bottom
)

type Font struct {
	Family    string
	SizePt    float32
	Bold      bool
	Italic    bool
	Strike    bool
	Underline bool
	Color     color.NRGBA
}

// Style is a fully resolved cell format (no theme/indexed colors left).
type Style struct {
	Font     Font
	Fill     color.NRGBA // A == 0 means no fill
	Border   [4]Edge
	HAlign   HAlign
	VAlign   VAlign
	Wrap     bool
	Indent   int
	Rotation int // degrees, 0 = horizontal

	// Number format: a built-in id, or a format code (which wins when set).
	NumFmtID   int
	NumFmtCode string
}

// Range is an inclusive, zero-based cell rectangle.
type Range struct {
	R1, C1, R2, C2 int
}

func (r Range) Contains(row, col int) bool {
	return row >= r.R1 && row <= r.R2 && col >= r.C1 && col <= r.C2
}

type Sheet struct {
	Name string

	// rows[r] holds cells 0..len-1 of row r; rows past the end are empty.
	rows [][]Cell

	ColWidth  map[int]float32 // characters (see ColWidthToPx); 0 = hidden
	RowHeight map[int]float32 // px; overrides DefaultRowHeight. 0 = hidden

	// Default column width in characters including padding; when zero, it
	// is derived from BaseColWidth (characters excluding padding).
	DefaultColWidth  float32
	BaseColWidth     float32
	DefaultRowHeight float32

	Merges     []Range
	FreezeRows int // rows frozen at the top
	FreezeCols int // columns frozen at the left

	ShowGridLines bool
	Hidden        bool

	// Saved view: the active cell and the top-left visible cell.
	ActiveRow, ActiveCol int
	TopRow, LeftCol      int

	// Spills maps a dynamic-array (or legacy array) formula's cell to the
	// range its result occupies.
	Spills map[[2]int]Range
}

func NewSheet(name string) *Sheet {
	return &Sheet{
		Name:             name,
		ColWidth:         map[int]float32{},
		RowHeight:        map[int]float32{},
		BaseColWidth:     DefaultBaseColWidth,
		DefaultRowHeight: DefaultRowHeightPx,
		ShowGridLines:    true,
		Spills:           map[[2]int]Range{},
	}
}

// Set stores a cell. Out-of-limit coordinates are ignored and reported false.
func (s *Sheet) Set(row, col int, c Cell) bool {
	if row < 0 || col < 0 || row >= MaxRows || col >= MaxCols {
		return false
	}
	if row >= len(s.rows) {
		s.rows = append(s.rows, make([][]Cell, row+1-len(s.rows))...)
	}
	r := s.rows[row]
	if col >= len(r) {
		r = append(r, make([]Cell, col+1-len(r))...)
		s.rows[row] = r
	}
	r[col] = c
	return true
}

// Get returns the cell at row, col, or a zero Cell if it is empty.
func (s *Sheet) Get(row, col int) *Cell {
	if row < 0 || row >= len(s.rows) || col < 0 || col >= len(s.rows[row]) {
		return &emptyCell
	}
	return &s.rows[row][col]
}

var emptyCell Cell

// RowLen is the number of cells stored in row r (trailing empty cells
// included); cells past it are empty.
func (s *Sheet) RowLen(r int) int {
	if r < 0 || r >= len(s.rows) {
		return 0
	}
	return len(s.rows[r])
}

// UsedRows and UsedCols give the extent of stored cells.
func (s *Sheet) UsedRows() int { return len(s.rows) }

func (s *Sheet) UsedCols() int {
	n := 0
	for _, r := range s.rows {
		n = max(n, len(r))
	}
	return n
}

// DefaultColWidthPx is the width of columns without their own width, for
// a default font with digit width mdw.
func (s *Sheet) DefaultColWidthPx(mdw float64) float32 {
	if s.DefaultColWidth > 0 {
		return ColWidthToPx(float64(s.DefaultColWidth), mdw)
	}
	return BaseColWidthToPx(float64(s.BaseColWidth), mdw)
}

// ColWidthPx is the pixel width of col for a default font with digit
// width mdw.
func (s *Sheet) ColWidthPx(col int, mdw float64) float32 {
	if w, ok := s.ColWidth[col]; ok {
		return ColWidthToPx(float64(w), mdw)
	}
	return s.DefaultColWidthPx(mdw)
}

func (s *Sheet) RowHeightPx(row int) float32 {
	if h, ok := s.RowHeight[row]; ok {
		return h
	}
	return s.DefaultRowHeight
}

// MergeAt returns the merged range covering row, col, if any.
func (s *Sheet) MergeAt(row, col int) (Range, bool) {
	for _, m := range s.Merges {
		if m.Contains(row, col) {
			return m, true
		}
	}
	return Range{}, false
}

type Workbook struct {
	Name     string
	Sheets   []*Sheet
	Styles   []Style // Styles[0] is the workbook default
	Warnings []string
	Date1904 bool // dates count from 1904-01-01 (old Mac workbooks)
	Names    []DefinedName
}

// DefinedName is a workbook or sheet-level name such as TaxRate =
// Sheet1!$B$2.
type DefinedName struct {
	Name    string
	Sheet   int // -1 for workbook scope, else the sheet index it belongs to
	Formula string
}

// WarnDropped records that n cells of sheet fell outside the size limits.
func (wb *Workbook) WarnDropped(sheet string, n int) {
	if n > 0 {
		wb.Warnings = append(wb.Warnings, fmt.Sprintf(
			"%s: %d cells beyond %d rows × %d columns were not loaded", sheet, n, MaxRows, MaxCols))
	}
}

// AddStyle appends st and returns its index; identical styles share an index
// when the caller passes the same cache.
func (wb *Workbook) AddStyle(st Style, cache map[Style]uint32) uint32 {
	if i, ok := cache[st]; ok {
		return i
	}
	i := uint32(len(wb.Styles))
	wb.Styles = append(wb.Styles, st)
	if cache != nil {
		cache[st] = i
	}
	return i
}

// DefaultStyle is Excel 2007+'s Normal style.
func DefaultStyle() Style {
	return Style{Font: Font{Family: "Calibri", SizePt: 11, Color: color.NRGBA{A: 255}}}
}

// ColName converts a zero-based column index to letters: 0 -> A, 27 -> AB.
func ColName(col int) string {
	var b [4]byte
	i := len(b)
	for col++; col > 0; col = (col - 1) / 26 {
		i--
		b[i] = byte('A' + (col-1)%26)
	}
	return string(b[i:])
}

// CellName returns the A1-style reference for zero-based row, col.
func CellName(row, col int) string {
	return ColName(col) + itoa(row+1)
}

// ParseColName is the inverse of ColName; it returns -1 for invalid input.
func ParseColName(s string) int {
	s = strings.ToUpper(s)
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 'A' || c > 'Z' {
			return -1
		}
		n = n*26 + int(c-'A'+1)
	}
	return n - 1
}

func itoa(n int) string {
	var b [12]byte
	i := len(b)
	for {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
		if n == 0 {
			return string(b[i:])
		}
	}
}
