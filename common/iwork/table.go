// Numbers tables: decoding TST.TableModelArchive cell storage, headers,
// merges and banding.

package iwork

import (
	"encoding/binary"
	"errors"
	"math"
	"math/big"
)

// Table object types.
const (
	TypeTableInfo    = 6000 // TST.TableInfoArchive (the drawable)
	TypeTableModel   = 6001 // TST.TableModelArchive
	TypeTile         = 6002 // TST.Tile
	TypeDataList     = 6005 // TST.TableDataList
	TypeHeaderBucket = 6006 // TST.HeaderStorageBucket
	TypeDataSegment  = 6011 // TST.TableDataListSegment
	TypeMergeMap     = 6144 // TST.MergeRegionMapArchive
)

// CellType is a cell's value type (TST.CellType, plus currency).
type CellType uint8

const (
	CellEmpty    CellType = 0
	CellNumber   CellType = 2
	CellText     CellType = 3
	CellDate     CellType = 5
	CellBool     CellType = 6
	CellDuration CellType = 7
	CellError    CellType = 8
	CellRichText CellType = 9
	CellCurrency CellType = 10
)

// Format categories a cell can carry a format for (the storage keeps one
// per value type, the one used depends on the value).
const (
	FmtNumber = iota
	FmtCurrency
	FmtDate
	FmtDuration
	FmtText
	FmtBool
	numFmtKinds
)

// Cell is one decoded table cell.
type Cell struct {
	Type CellType
	// Num is the value of number, currency, bool (0/1) and duration
	// (seconds) cells; Seconds that of date cells (seconds since
	// 2001-01-01 00:00 UTC).
	Num     float64
	Seconds float64
	Text    string // text cells; the plain text of rich text cells
	Rich    uint64 // rich text: the TST.RichTextPayloadArchive
	// CellStyle (TST.CellStyleArchive) and TextStyle
	// (TSWP.ParagraphStyleArchive) set on the cell itself, 0 if none.
	CellStyle uint64
	TextStyle uint64
	// Formats are the cell's TSK.FormatStructArchive per category
	// (FmtNumber...), nil where none is set.
	Formats [numFmtKinds]Message
	// Formula is the TSCE.FormulaArchive computing the cell, nil for a
	// constant; the value fields hold its last result.
	Formula Message
	Comment bool
}

// Format is the format that applies to the cell's value, or nil.
func (c *Cell) Format() Message {
	switch c.Type {
	case CellNumber:
		if f := c.Formats[FmtNumber]; f != nil {
			return f
		}
		return c.Formats[FmtCurrency]
	case CellCurrency:
		if f := c.Formats[FmtCurrency]; f != nil {
			return f
		}
		return c.Formats[FmtNumber]
	case CellDate:
		return c.Formats[FmtDate]
	case CellDuration:
		return c.Formats[FmtDuration]
	case CellBool:
		return c.Formats[FmtBool]
	case CellText, CellRichText:
		return c.Formats[FmtText]
	}
	return nil
}

// Range is an inclusive, zero-based cell rectangle.
type Range struct{ R1, C1, R2, C2 int }

// Header holds a row's or column's size and style.
type Header struct {
	Size      float64 // points, 0 = default
	Hidden    bool
	CellStyle uint64
	TextStyle uint64
}

// Table is a decoded TST.TableModelArchive.
type Table struct {
	ID                                 uint64
	UUID                               string // the table's ID, as formulas in other tables refer to it
	Name                               string
	NameShown                          bool
	Rows, Cols                         int
	HeaderRows, HeaderCols, FooterRows int
	DefaultRowHeight, DefaultColWidth  float64 // points
	HeaderRowsFrozen, HeaderColsFrozen bool
	RowInfo, ColInfo                   map[int]Header
	Cells                              map[[2]int]*Cell // by row, column
	Merges                             []Range
	// Area styles: body, header row, header column, footer row — cell
	// styles (TST.CellStyleArchive) and text styles
	// (TSWP.ParagraphStyleArchive).
	AreaCellStyle, AreaTextStyle [4]uint64
	// BandFill is the fill of every other body row when the table style
	// has banded rows.
	BandFill Color
	Warnings []string
}

// Table areas, indexes into AreaCellStyle and AreaTextStyle.
const (
	AreaBody = iota
	AreaHeaderRow
	AreaHeaderCol
	AreaFooterRow
)

// Area returns the area of a cell.
func (t *Table) Area(row, col int) int {
	switch {
	case row < t.HeaderRows:
		return AreaHeaderRow
	case row >= t.Rows-t.FooterRows:
		return AreaFooterRow
	case col < t.HeaderCols:
		return AreaHeaderCol
	}
	return AreaBody
}

// Cell returns the cell at row, col, or nil.
func (t *Table) Cell(row, col int) *Cell { return t.Cells[[2]int{row, col}] }

// dataList reads a TST.TableDataList (and its segments) by key.
func (d *Document) dataList(id uint64) map[uint64]Message {
	out := map[uint64]Message{}
	m := d.Msg(id)
	add := func(entries []Message) {
		for _, e := range entries {
			out[e.Uint(1)] = e
		}
	}
	add(m.Msgs(3))
	for _, seg := range m.Refs(4) {
		add(d.Msg(seg).Msgs(3))
	}
	return out
}

// TableModel returns the table a TST.TableInfoArchive (or the
// TST.TableModelArchive itself) identifies.
func (d *Document) TableModel(id uint64) (*Table, error) {
	if d.Type(id) == TypeTableInfo {
		id = d.Msg(id).Ref(2)
	}
	if t, ok := d.tables[id]; ok {
		return t, nil
	}
	if d.Type(id) != TypeTableModel {
		return nil, errors.New("iwork: not a table")
	}
	t, err := d.readTable(id)
	if err != nil {
		return nil, err
	}
	if d.tables == nil {
		d.tables = map[uint64]*Table{}
	}
	d.tables[id] = t
	return t, nil
}

// readTable decodes the TST.TableModelArchive id: its dimensions, area
// styles, row and column headers, cells (from the tiles' version-5 storage),
// merges and warnings for what isn't read.
func (d *Document) readTable(id uint64) (*Table, error) {
	m := d.Msg(id)
	t := &Table{
		ID:               id,
		UUID:             m.Str(1),
		Name:             m.Str(8),
		NameShown:        m.Bool(22),
		Rows:             int(m.Uint(6)),
		Cols:             int(m.Uint(7)),
		HeaderRows:       int(m.Uint(9)),
		HeaderCols:       int(m.Uint(10)),
		FooterRows:       int(m.Uint(11)),
		DefaultRowHeight: m.Double(16),
		DefaultColWidth:  m.Double(17),
		HeaderRowsFrozen: m.Bool(12),
		HeaderColsFrozen: m.Bool(13),
		RowInfo:          map[int]Header{},
		ColInfo:          map[int]Header{},
		Cells:            map[[2]int]*Cell{},
		AreaCellStyle:    [4]uint64{m.Ref(18), m.Ref(19), m.Ref(20), m.Ref(21)},
		AreaTextStyle:    [4]uint64{m.Ref(24), m.Ref(25), m.Ref(26), m.Ref(27)},
	}
	t.BandFill = d.bandFill(m.Ref(3))
	ds := m.Msg(4)
	readHeaders := func(bucket uint64, into map[int]Header) {
		for _, h := range d.Msg(bucket).Msgs(2) {
			into[int(h.Uint(1))] = Header{
				Size:      h.Float(2),
				Hidden:    h.Uint(3) != 0,
				CellStyle: h.Ref(5),
				TextStyle: h.Ref(6),
			}
		}
	}
	for _, b := range ds.Msg(1).Refs(2) {
		readHeaders(b, t.RowInfo)
	}
	readHeaders(ds.Ref(2), t.ColInfo)

	strs := d.dataList(ds.Ref(4))
	styles := d.dataList(ds.Ref(5))
	formulas := d.dataList(ds.Ref(6))
	formats := d.dataList(ds.Ref(22))
	if len(formats) == 0 {
		formats = d.dataList(ds.Ref(11))
	}
	rich := d.dataList(ds.Ref(17))

	tiles := ds.Msg(3)
	tileSize := int(tiles.Uint(2))
	if tileSize == 0 {
		tileSize = 256
	}
	unsupported := 0
	for _, tr := range tiles.Msgs(1) {
		tile := d.Msg(tr.Ref(2))
		base := int(tr.Uint(1)) * tileSize
		for _, ri := range tile.Msgs(5) {
			row := base + int(ri.Uint(1))
			buf, offs := ri.Bytes(6), ri.Bytes(7)
			if buf == nil {
				unsupported++ // pre-BNC (Numbers 3) storage
				continue
			}
			cells, bad := decodeRow(buf, offs, ri.Bool(8))
			unsupported += bad
			for col, c := range cells {
				d.resolveCell(c, strs, styles, formulas, formats, rich)
				t.Cells[[2]int{row, col}] = c.Cell
			}
		}
	}
	if unsupported > 0 {
		t.Warnings = append(t.Warnings, "some cells are stored in an older Numbers format and were not read")
	}
	t.Merges = d.tableMerges(m, ds)
	if len(d.dataList(ds.Ref(18))) > 0 {
		t.Warnings = append(t.Warnings, "conditional highlighting was not imported")
	}
	return t, nil
}

// missingCell is the offset that marks a column with no cell.
const missingCell = 0xFFFF

// decodeRow decodes the cells of a row from its storage buf and its
// column offsets (16-bit little-endian, in units of 4 bytes when wide). It
// returns the cells by column and how many it couldn't read. Offsets are
// unsigned, with only 0xFFFF meaning no cell, and in no particular order;
// each cell tells its own length, so it is read from its offset onward.
func decodeRow(buf, offs []byte, wide bool) (cells map[int]rawCell, unsupported int) {
	cells = map[int]rawCell{}
	for col := range len(offs) / 2 {
		o := int(binary.LittleEndian.Uint16(offs[2*col:]))
		if o == missingCell {
			continue
		}
		if wide {
			o *= 4
		}
		if o >= len(buf) {
			continue
		}
		c, ok := decodeCell(buf[o:])
		if !ok {
			unsupported++
			continue
		}
		cells[col] = c
	}
	return cells, unsupported
}

// rawCell is a decoded cell storage record with the keys into the
// table's data lists.
type rawCell struct {
	*Cell
	str, rich, cellStyle, textStyle, formula int32
	formats                                  [numFmtKinds]int32
}

// decodeCell parses one cell's storage (format version 5): version, type,
// a flags word saying which fields follow, then the fields.
func decodeCell(b []byte) (rawCell, bool) {
	c := rawCell{Cell: &Cell{}, str: -1, rich: -1, cellStyle: -1, textStyle: -1, formula: -1}
	for i := range c.formats {
		c.formats[i] = -1
	}
	if len(b) < 12 || b[0] != 5 {
		return c, false
	}
	c.Type = CellType(b[1])
	flags := binary.LittleEndian.Uint32(b[8:])
	off := 12
	ok := true
	next := func(n int) []byte {
		if off+n > len(b) {
			ok = false
			return make([]byte, n)
		}
		v := b[off : off+n]
		off += n
		return v
	}
	i32 := func() int32 { return int32(binary.LittleEndian.Uint32(next(4))) }
	f64 := func() float64 { return math.Float64frombits(binary.LittleEndian.Uint64(next(8))) }
	var dec, dbl float64
	if flags&0x1 != 0 {
		dec = decimal128(next(16))
	}
	if flags&0x2 != 0 {
		dbl = f64()
	}
	if flags&0x4 != 0 {
		c.Seconds = f64()
	}
	fields := []struct {
		bit uint32
		dst *int32
	}{
		{0x8, &c.str}, {0x10, &c.rich}, {0x20, &c.cellStyle}, {0x40, &c.textStyle},
		{0x80, nil}, {0x100, nil}, {0x200, &c.formula}, {0x400, nil}, {0x800, nil}, {0x1000, nil},
		{0x2000, &c.formats[FmtNumber]}, {0x4000, &c.formats[FmtCurrency]},
		{0x8000, &c.formats[FmtDate]}, {0x10000, &c.formats[FmtDuration]},
		{0x20000, &c.formats[FmtText]}, {0x40000, &c.formats[FmtBool]},
		{0x80000, nil},
	}
	for _, f := range fields {
		if flags&f.bit == 0 {
			continue
		}
		v := i32()
		if f.dst != nil {
			*f.dst = v
		}
		if f.bit == 0x80000 {
			c.Comment = true
		}
	}
	switch c.Type {
	case CellNumber, CellCurrency:
		c.Num = dec
		if flags&0x1 == 0 {
			c.Num = dbl
		}
	case CellBool, CellDuration:
		c.Num = dbl
	}
	return c, ok
}

// decimal128 decodes an IEEE 754-2008 decimal128 in binary integer
// decimal encoding (little-endian), as Numbers stores numbers.
func decimal128(b []byte) float64 {
	const bias = 6176
	exp := int(b[15]&0x7f)<<7 | int(b[14]>>1)
	exp -= bias
	mant := new(big.Int)
	be := make([]byte, 15)
	for i := range 14 {
		be[14-i] = b[i]
	}
	be[0] = b[14] & 1
	mant.SetBytes(be)
	f, _ := new(big.Float).SetInt(mant).Float64()
	v := f * math.Pow10(exp)
	if exp < 0 { // divide for exactness: 5 * 10^-2 is better as 5 / 100
		v = f / math.Pow10(-exp)
	}
	if b[15]&0x80 != 0 {
		v = -v
	}
	return v
}

// resolveCell looks the cell's keys up in the table's data lists.
func (d *Document) resolveCell(c rawCell, strs, styles, formulas, formats, rich map[uint64]Message) {
	key := func(k int32) uint64 { return uint64(uint32(k)) }
	if c.str >= 0 {
		c.Text = strs[key(c.str)].Str(3)
	}
	if c.rich >= 0 {
		c.Rich = rich[key(c.rich)].Ref(9)
		if c.Rich == 0 {
			c.Rich = rich[key(c.rich)].Ref(4)
		}
		c.Text = d.PlainText(c.Rich)
	}
	if c.cellStyle >= 0 {
		c.CellStyle = styles[key(c.cellStyle)].Ref(4)
	}
	if c.textStyle >= 0 {
		c.TextStyle = styles[key(c.textStyle)].Ref(4)
	}
	if c.formula >= 0 {
		if e, ok := formulas[key(c.formula)]; ok {
			c.Formula = e.Msg(5)
		}
	}
	for i, k := range c.formats {
		if k >= 0 {
			if e, ok := formats[key(k)]; ok {
				c.Formats[i] = e.Msg(6)
			}
		}
	}
}

// tableMerges reads the merged ranges, from the merge region map or the
// merge owner's formula store (newer files).
func (d *Document) tableMerges(m, ds Message) []Range {
	var out []Range
	if id := ds.Ref(13); id != 0 {
		for _, cr := range d.Msg(id).Msgs(1) {
			origin := uint32(cr.Msg(1).Uint(1))
			size := uint32(cr.Msg(2).Uint(1))
			r, c := int(origin&0xffff), int(origin>>16)
			nr, nc := int(size&0xffff), int(size>>16)
			if nr > 0 && nc > 0 {
				out = append(out, Range{r, c, r + nr - 1, c + nc - 1})
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	// TST.MergeOwnerArchive.formula_store: each merge is a formula whose
	// first AST node is a colon-tract node with absolute rows and
	// columns.
	for _, f := range m.Path(47, 2).Msgs(3) {
		nodes := f.Path(2, 1).Msgs(1)
		if len(nodes) == 0 || nodes[0].Uint(1) != astColonTract {
			continue
		}
		tract := nodes[0].Msg(astFieldColonTract)
		rows, cols := tract.Msgs(4), tract.Msgs(3)
		if len(rows) == 0 || len(cols) == 0 {
			continue
		}
		span := func(m Message) (int, int) {
			b := int(m.Uint(1))
			e := b
			if v, ok := m.UintOK(2); ok {
				e = int(v)
			}
			return b, e
		}
		r1, r2 := span(rows[0])
		c1, c2 := span(cols[0])
		if r2 > r1 || c2 > c1 {
			out = append(out, Range{r1, c1, r2, c2})
		}
	}
	return out
}

// AST node types and fields used here (TSCE.ASTNodeArrayArchive).
const (
	astColonTract      = 67 // COLON_TRACT_NODE
	astFieldColonTract = 40 // AST_colon_tract
)

// typeTableStyle is TST.TableStyleArchive.
const typeTableStyle = 6003

// bandFill returns the banded-row fill of a table style (following its
// parents), if banding is on.
func (d *Document) bandFill(id uint64) Color {
	var banded *bool
	var fill Color
	for seen := map[uint64]bool{}; id != 0 && !seen[id]; {
		seen[id] = true
		o := d.Objects[id]
		if o == nil || o.Type != typeTableStyle {
			break
		}
		props := o.Msg.Msg(11) // TST.TableStylePropertiesArchive
		if v, ok := props.BoolOK(1); ok && banded == nil {
			banded = &v
		}
		if props.Has(2) && !fill.Set {
			fill = Color{Set: true}
			if f := props.Msg(2); f.Has(1) {
				fill = ReadColor(f.Msg(1))
			}
		}
		id = o.Msg.Msg(1).Ref(3)
	}
	if banded == nil || !*banded {
		return Color{}
	}
	return fill
}

// Banded reports whether row gets the band fill.
func (t *Table) Banded(row int) bool {
	return t.BandFill.Set && t.BandFill.A > 0 && row >= t.HeaderRows && row < t.Rows-t.FooterRows && (row-t.HeaderRows)%2 == 1
}
