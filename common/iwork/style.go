// Paragraph, character, cell and list styles: decoding their properties and
// resolving them through their parent chains.

package iwork

import (
	"image/color"
	"math"

	"officelite/common/colors"
)

// Style object types.
const (
	TypeCharStyle  = 2021 // TSWP.CharacterStyleArchive
	TypeParaStyle  = 2022 // TSWP.ParagraphStyleArchive
	TypeListStyle  = 2023 // TSWP.ListStyleArchive
	TypeShapeStyle = 2025 // TSWP.ShapeStyleArchive
	TypeCellStyle  = 6004 // TST.CellStyleArchive
)

// Tri is an on/off property that may be unset.
type Tri int8

const (
	Unset Tri = iota
	On
	Off
)

// triOf returns On for true and Off for false.
func triOf(b bool) Tri {
	if b {
		return On
	}
	return Off
}

// Color is a TSP.Color; Set is false where the color is not given.
type Color struct {
	color.NRGBA
	Set bool
}

// Hex is the color as "RRGGBB".
func (c Color) Hex() string { return colors.Hex(c.NRGBA) }

// ReadColor decodes a TSP.Color message (RGB, white or CMYK models).
func ReadColor(m Message) Color {
	if m == nil {
		return Color{}
	}
	to8 := func(v float64) uint8 { return uint8(math.Round(math.Max(0, math.Min(1, v)) * 255)) }
	a := 1.0
	if v, ok := m.FloatOK(6); ok {
		a = v
	}
	var r, g, b float64
	switch m.Uint(1) {
	case 3: // white: a gray level
		w := m.Float(11)
		r, g, b = w, w, w
	case 2: // cmyk
		c, mg, y, k := m.Float(7), m.Float(8), m.Float(9), m.Float(10)
		r, g, b = (1-c)*(1-k), (1-mg)*(1-k), (1-y)*(1-k)
	default:
		r, g, b = m.Float(3), m.Float(4), m.Float(5)
	}
	return Color{NRGBA: color.NRGBA{R: to8(r), G: to8(g), B: to8(b), A: to8(a)}, Set: true}
}

// TextProps is character formatting (TSWP.CharacterStylePropertiesArchive);
// zero values are unset.
type TextProps struct {
	Font      string
	Size      float64 // points
	Color     Color
	Bold      Tri
	Italic    Tri
	Underline Tri
	Strike    Tri
	Script    int // 0 unset, 1 superscript, 2 subscript, 3 baseline
	Caps      int // 0 unset, 1 none, 2 all caps, 3 small caps
	Highlight Color
}

// Merge returns p with the properties set in over replacing it.
func (p TextProps) Merge(over TextProps) TextProps {
	if over.Font != "" {
		p.Font = over.Font
	}
	if over.Size != 0 {
		p.Size = over.Size
	}
	if over.Color.Set {
		p.Color = over.Color
	}
	for _, t := range []struct{ dst, src *Tri }{
		{&p.Bold, &over.Bold}, {&p.Italic, &over.Italic},
		{&p.Underline, &over.Underline}, {&p.Strike, &over.Strike},
	} {
		if *t.src != Unset {
			*t.dst = *t.src
		}
	}
	if over.Script != 0 {
		p.Script = over.Script
	}
	if over.Caps != 0 {
		p.Caps = over.Caps
	}
	if over.Highlight.Set {
		p.Highlight = over.Highlight
	}
	return p
}

// readTextProps decodes a CharacterStylePropertiesArchive.
func readTextProps(m Message) TextProps {
	var p TextProps
	if v, ok := m.BoolOK(1); ok {
		p.Bold = triOf(v)
	}
	if v, ok := m.BoolOK(2); ok {
		p.Italic = triOf(v)
	}
	if v, ok := m.FloatOK(3); ok && v > 0 {
		p.Size = v
	}
	if !m.Bool(4) {
		p.Font = m.Str(5)
	}
	if m.Has(7) && !m.Bool(6) {
		p.Color = ReadColor(m.Msg(7))
	}
	if v, ok := m.UintOK(10); ok {
		p.Script = [...]int{3, 1, 2}[min(v, 2)]
	}
	if v, ok := m.UintOK(11); ok {
		p.Underline = triOf(v != 0)
	}
	if v, ok := m.UintOK(12); ok {
		p.Strike = triOf(v != 0)
	}
	if v, ok := m.UintOK(13); ok {
		p.Caps = [...]int{1, 2, 3, 1}[min(v, 3)]
	}
	if m.Has(26) && !m.Bool(25) {
		if c := ReadColor(m.Msg(26)); c.A > 0 {
			p.Highlight = c
		}
	}
	return p
}

// Paragraph alignments (TSWP.ParagraphStylePropertiesArchive alignment).
const (
	AlignLeft    = 0
	AlignRight   = 1
	AlignCenter  = 2
	AlignJustify = 3
	AlignNatural = 4
)

// Line spacing modes (TSWP.LineSpacingArchive).
const (
	LineRelative = 0 // Amount is a multiple of single spacing
	LineAtLeast  = 1 // points
	LineExact    = 2 // points
	LineMaximum  = 3
	LineBetween  = 4 // points added between lines
)

// ParaProps is paragraph formatting (TSWP.ParagraphStylePropertiesArchive);
// nil pointers are unset.
type ParaProps struct {
	Align           *int
	SpaceBefore     *float64
	SpaceAfter      *float64
	LeftIndent      *float64
	RightIndent     *float64
	FirstIndent     *float64 // from the margin, like LeftIndent
	LineMode        int
	LineAmount      float64 // 0 = unset
	KeepNext        Tri
	KeepLines       Tri
	PageBreakBefore Tri
	Fill            Color
	Tabs            []float64 // left tab stops, points
	TabsSet         bool
}

// ptr returns a pointer to a copy of v.
func ptr[T any](v T) *T { return &v }

// Merge returns p with the properties set in over replacing it.
func (p ParaProps) Merge(over ParaProps) ParaProps {
	for _, f := range []struct{ dst, src **float64 }{
		{&p.SpaceBefore, &over.SpaceBefore}, {&p.SpaceAfter, &over.SpaceAfter},
		{&p.LeftIndent, &over.LeftIndent}, {&p.RightIndent, &over.RightIndent},
		{&p.FirstIndent, &over.FirstIndent},
	} {
		if *f.src != nil {
			*f.dst = *f.src
		}
	}
	if over.Align != nil {
		p.Align = over.Align
	}
	if over.LineAmount != 0 {
		p.LineMode, p.LineAmount = over.LineMode, over.LineAmount
	}
	for _, t := range []struct{ dst, src *Tri }{
		{&p.KeepNext, &over.KeepNext}, {&p.KeepLines, &over.KeepLines},
		{&p.PageBreakBefore, &over.PageBreakBefore},
	} {
		if *t.src != Unset {
			*t.dst = *t.src
		}
	}
	if over.Fill.Set {
		p.Fill = over.Fill
	}
	if over.TabsSet {
		p.Tabs, p.TabsSet = over.Tabs, true
	}
	return p
}

// readParaProps decodes a ParagraphStylePropertiesArchive.
func readParaProps(m Message) ParaProps {
	var p ParaProps
	if v, ok := m.UintOK(1); ok {
		p.Align = ptr(int(v))
	}
	if m.Has(6) && !m.Bool(5) {
		if c := ReadColor(m.Msg(6)); c.A > 0 {
			p.Fill = c
		}
	}
	for _, f := range []struct {
		n   int
		dst **float64
	}{{7, &p.FirstIndent}, {11, &p.LeftIndent}, {19, &p.RightIndent}, {20, &p.SpaceAfter}, {21, &p.SpaceBefore}} {
		if v, ok := m.FloatOK(f.n); ok {
			*f.dst = ptr(v)
		}
	}
	if ls := m.Msg(13); ls != nil && !m.Bool(12) {
		if v, ok := ls.FloatOK(2); ok && v > 0 {
			p.LineMode, p.LineAmount = int(ls.Uint(1)), v
		}
	}
	if v, ok := m.BoolOK(9); ok {
		p.KeepLines = triOf(v)
	}
	if v, ok := m.BoolOK(10); ok {
		p.KeepNext = triOf(v)
	}
	if v, ok := m.BoolOK(14); ok {
		p.PageBreakBefore = triOf(v)
	}
	if m.Has(25) && !m.Bool(24) {
		p.TabsSet = true
		for _, t := range m.Msg(25).Msgs(1) {
			p.Tabs = append(p.Tabs, t.Float(1))
		}
	}
	return p
}

// Style is a resolved paragraph, character or cell style: its own
// properties merged over those of its parents.
type Style struct {
	ID    uint64
	Name  string // the user-visible name ("Heading 1"), "" for variations
	Ident string // the style identifier ("text-3-paragraphstyle-Heading 1")
	// Parent is the style this one is based on; a Variation is an unnamed
	// style holding overrides of its parent (direct formatting).
	Parent    uint64
	Variation bool
	Text      TextProps
	Para      ParaProps
	Cell      CellProps
}

// CellProps is table cell formatting (TST.CellStylePropertiesArchive).
type CellProps struct {
	Fill   Color
	Wrap   Tri
	VAlign int // 0 unset, 1 top, 2 middle, 3 bottom
}

// readCellProps decodes a CellStylePropertiesArchive.
func readCellProps(m Message) CellProps {
	var p CellProps
	if m.Has(1) { // an empty fill clears an inherited one
		p.Fill = Color{Set: true}
		if f := m.Msg(1); f.Has(1) {
			p.Fill = ReadColor(f.Msg(1))
		}
	}
	if v, ok := m.BoolOK(3); ok {
		p.Wrap = triOf(v)
	}
	if v, ok := m.UintOK(8); ok {
		p.VAlign = int(v) + 1
	}
	return p
}

// Style returns the style with identifier id resolved through its parent
// chain; nil if id isn't a paragraph, character or cell style.
func (d *Document) Style(id uint64) *Style {
	if st, ok := d.styles[id]; ok {
		return st
	}
	if d.styles == nil {
		d.styles = map[uint64]*Style{}
	}
	d.styles[id] = nil // guards against parent cycles
	o := d.Objects[id]
	if o == nil {
		return nil
	}
	var st Style
	switch o.Type {
	case TypeCharStyle, TypeParaStyle, TypeCellStyle:
	default:
		return nil
	}
	super := o.Msg.Msg(1)
	if parent := super.Ref(3); parent != 0 {
		if p := d.Style(parent); p != nil {
			st = *p
		}
	}
	st.ID = id
	st.Name, st.Ident = super.Str(1), super.Str(2)
	st.Parent, st.Variation = super.Ref(3), super.Bool(4)
	switch o.Type {
	case TypeCharStyle:
		st.Text = st.Text.Merge(readTextProps(o.Msg.Msg(11)))
	case TypeParaStyle:
		st.Text = st.Text.Merge(readTextProps(o.Msg.Msg(11)))
		st.Para = st.Para.Merge(readParaProps(o.Msg.Msg(12)))
	case TypeCellStyle:
		cp := readCellProps(o.Msg.Msg(11))
		if cp.Fill.Set {
			st.Cell.Fill = cp.Fill
		}
		if cp.Wrap != Unset {
			st.Cell.Wrap = cp.Wrap
		}
		if cp.VAlign != 0 {
			st.Cell.VAlign = cp.VAlign
		}
	}
	d.styles[id] = &st
	return &st
}

// ListLevel is one level of a list style.
type ListLevel struct {
	Label  int     // 0 none, 1 image, 2 string (bullet), 3 number
	Number int     // TSWP.ListStyleArchive.NumberType
	Bullet string  // the bullet text (Label 2)
	Indent float64 // left indent of the text, points
	Label0 float64 // indent of the label, points
	Tiered bool    // numbers include the outer levels' (1.2.3)
}

// ListLevels decodes a TSWP.ListStyleArchive's levels, following its
// parents for the arrays it doesn't set.
func (d *Document) ListLevels(id uint64) []ListLevel {
	var chain []Message
	seen := map[uint64]bool{}
	for id != 0 && !seen[id] {
		seen[id] = true
		o := d.Objects[id]
		if o == nil || o.Type != TypeListStyle {
			break
		}
		chain = append(chain, o.Msg)
		id = o.Msg.Msg(1).Ref(3)
	}
	// The nearest style that has an array wins.
	pick := func(has func(Message) bool) Message {
		for _, m := range chain {
			if has(m) {
				return m
			}
		}
		return nil
	}
	types := pick(func(m Message) bool { return m.Has(11) }).Uints(11)
	nums := pick(func(m Message) bool { return m.Has(15) }).Uints(15)
	strs := pick(func(m Message) bool { return m.Has(16) }).Strs(16)
	indents := pick(func(m Message) bool { return m.Has(13) }).Floats(13)
	textIndents := pick(func(m Message) bool { return m.Has(12) }).Floats(12)
	tiered := pick(func(m Message) bool { return m.Has(25) }).Uints(25)
	n := max(len(types), len(nums), len(strs))
	levels := make([]ListLevel, n)
	for i := range levels {
		l := &levels[i]
		if i < len(types) {
			l.Label = int(types[i])
		}
		if i < len(nums) {
			l.Number = int(nums[i])
		}
		if i < len(strs) {
			l.Bullet = strs[i]
		}
		if i < len(indents) {
			l.Label0 = indents[i]
		}
		if i < len(textIndents) {
			l.Indent = l.Label0 + textIndents[i]
		}
		if i < len(tiered) {
			l.Tiered = tiered[i] != 0
		}
	}
	return levels
}

// NamedStyle returns the nearest named, non-variation style among id and
// its ancestors (0 if none).
func (d *Document) NamedStyle(id uint64) uint64 {
	for seen := map[uint64]bool{}; id != 0 && !seen[id]; {
		seen[id] = true
		st := d.Style(id)
		if st == nil {
			return 0
		}
		if !st.Variation && st.Name != "" {
			return id
		}
		id = st.Parent
	}
	return 0
}
