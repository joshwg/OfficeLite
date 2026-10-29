// Package layout lays out a Word document into pages: paragraphs broken
// into lines with each run's resolved formatting, lists, tabs, tables and
// pagination, plus caret and hit-testing queries and drawing.
//
// Lengths are points (1/72 inch) throughout; drawing scales them to
// pixels. Text is measured and drawn with a Gio text.Shaper (which must
// only be used from one goroutine). Paragraph layouts are cached and only
// the paragraphs reported changed are laid out again; pagination over the
// cached lines is cheap and runs in full.
package layout

import (
	"image/color"
	"strings"
	"unicode"

	"gioui.org/font"
	"gioui.org/text"
	"golang.org/x/image/math/fixed"

	"officelite/common/colors"
	"officelite/common/gui"
	"officelite/word/model"
)

// measureScale is the pixels-per-point text is shaped at for measuring;
// shaping is linear (no hinting), so a larger size only adds precision.
const measureScale = 4

// TextStyle is the display formatting of a piece of text.
type TextStyle struct {
	Font      font.Font
	Family    string
	Size      float64 // points, after super/subscript or small-caps scaling
	Color     color.NRGBA
	Underline bool
	Strike    bool
	Highlight color.NRGBA // highlight or character shading; A == 0: none
	Shift     float64     // baseline shift in points, up positive
	Hidden    bool
	// Revealed is hidden text laid out and shown (SetShowHidden).
	Revealed bool
}

type measureKey struct {
	font font.Font
	size float64
	text string
}

// measured is the shaped width of a string: one advance per rune (a
// cluster's advance is shared by its runes) and the font's line metrics:
// Gio's, and Word's (wasc, wdesc; see gui.WordMetrics), which decide how
// tall a line mixing fonts is.
type measured struct {
	adv           []float32
	ascent, desc  float64
	wasc, wdesc   float64
	width         float64
	trailingSpace float64
}

type measurer struct {
	shaper *text.Shaper
	cache  map[measureKey]*measured
	covers map[measureKey]bool
}

func (m *measurer) measure(st *TextStyle, s string) *measured {
	k := measureKey{st.Font, st.Size, s}
	if r, ok := m.cache[k]; ok {
		return r
	}
	n := 0
	for range s {
		n++
	}
	r := &measured{adv: make([]float32, n)}
	m.shaper.LayoutString(text.Parameters{
		Font:             st.Font,
		PxPerEm:          fixed.Int26_6(st.Size * measureScale * 64),
		MaxWidth:         1 << 24,
		MaxLines:         1,
		DisableSpaceTrim: true,
	}, s)
	i := 0
	var cluster float64
	first := true
	for {
		g, ok := m.shaper.NextGlyph()
		if !ok {
			break
		}
		if first {
			r.ascent = float64(g.Ascent) / 64 / measureScale
			r.desc = float64(g.Descent) / 64 / measureScale
			first = false
		}
		if a, d, ok := gui.WordMetrics(m.shaper, g.ID); ok {
			r.wasc, r.wdesc = max(r.wasc, a*st.Size), max(r.wdesc, d*st.Size)
		}
		cluster += float64(g.Advance) / 64 / measureScale
		if g.Flags&text.FlagClusterBreak != 0 && g.Runes > 0 {
			per := cluster / float64(g.Runes)
			for j := 0; j < int(g.Runes) && i < n; j++ {
				r.adv[i] = float32(per)
				i++
			}
			cluster = 0
		}
	}
	if r.wasc == 0 && r.wdesc == 0 {
		r.wasc, r.wdesc = r.ascent, r.desc
	}
	rs := []rune(s)
	for _, a := range r.adv {
		r.width += float64(a)
	}
	for j := len(rs) - 1; j >= 0 && rs[j] == ' '; j-- {
		r.trailingSpace += float64(r.adv[j])
	}
	if m.cache == nil {
		m.cache = map[measureKey]*measured{}
	}
	m.cache[k] = r
	return r
}

// has reports whether the font family itself (not a fallback face) has a
// glyph for every rune of s. Without a FontLibrary shaper (tests) nothing
// is known and it reports false.
func (m *measurer) has(st *TextStyle, family string, s string) bool {
	k := measureKey{st.Font, 0, family + "\x00" + s}
	if v, ok := m.covers[k]; ok {
		return v
	}
	m.shaper.LayoutString(text.Parameters{Font: st.Font, PxPerEm: fixed.I(16), MaxWidth: 1 << 24, MaxLines: 1}, s)
	ok := true
	for {
		g, more := m.shaper.NextGlyph()
		if !more {
			break
		}
		if g.Flags&text.FlagParagraphBreak == 0 && (uint32(g.ID) == 0 || !strings.EqualFold(gui.GlyphFamily(m.shaper, g.ID), family)) {
			ok = false
		}
	}
	if m.covers == nil {
		m.covers = map[measureKey]bool{}
	}
	m.covers[k] = ok
	return ok
}

// highlightColors are Word's named highlight colors.
var highlightColors = map[string]uint32{
	"yellow": 0xFFFF00, "green": 0x00FF00, "cyan": 0x00FFFF, "magenta": 0xFF00FF,
	"blue": 0x0000FF, "red": 0xFF0000, "darkBlue": 0x000080, "darkCyan": 0x008080,
	"darkGreen": 0x008000, "darkMagenta": 0x800080, "darkRed": 0x800000,
	"darkYellow": 0x808000, "darkGray": 0x808080, "lightGray": 0xC0C0C0, "black": 0x000000,
	"white": 0xFFFFFF,
}

// styleOf converts resolved run properties into a display style.
func styleOf(rp model.RunProps) *TextStyle {
	st := &TextStyle{Family: rp.Font, Size: rp.Size, Color: colors.HexOr(rp.Color, colors.RGB(0))}
	st.Font = font.Font{Typeface: gui.Typeface(rp.Font)}
	if rp.Bold.Bool(false) {
		st.Font.Weight = font.Bold
	}
	if rp.Italic.Bool(false) {
		st.Font.Style = font.Italic
	}
	st.Underline = rp.Underline != model.UnderlineUnset && rp.Underline != model.UnderlineNone
	st.Strike = rp.Strike.Bool(false)
	st.Hidden = rp.Hidden.Bool(false)
	if h := rp.Highlight; h != "" && h != "none" {
		if v, ok := highlightColors[h]; ok {
			st.Highlight = colors.RGB(v)
		} else if c, ok := colors.ParseHex(h); ok {
			st.Highlight = c
		}
	}
	// Character shading fills the same box as highlighting, which covers
	// it when the text has both.
	if c, ok := colors.ParseHex(rp.Shading); ok && st.Highlight.A == 0 {
		st.Highlight = c
	}
	switch rp.Vert {
	case model.VertSuper:
		st.Shift = rp.Size * 0.33
		st.Size = rp.Size * 0.65
	case model.VertSub:
		st.Shift = -rp.Size * 0.14
		st.Size = rp.Size * 0.65
	}
	return st
}

// symbolMap translates the private-use characters of the Symbol and
// Wingdings fonts used for bullets into Unicode, for when the font isn't
// there (or the text isn't in it).
var symbolMap = map[rune]rune{
	0xF0B7: '•', 0xF0A7: '▪', 0xF06F: '○', 0xF0D8: '➢', 0xF0FC: '✓',
	0xF076: '❖', 0xF0E8: '➔', 0xF0A8: '□', 0xF06E: '■', 0xF071: '❑',
}

func mapSymbols(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r >= 0xF000 && r <= 0xF0FF }) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r < 0xF000 || r > 0xF0FF {
			return r
		}
		if m, ok := symbolMap[r]; ok {
			return m
		}
		return r - 0xF000
	}, s)
}

// upper uppercases rune by rune, keeping the rune count (caret offsets).
func upper(s string) string {
	return strings.Map(unicode.ToUpper, s)
}
