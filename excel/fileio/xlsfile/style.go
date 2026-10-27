package xlsfile

import (
	"image/color"

	"github.com/xuri/excelize/v2"

	"officelite/common/colors"
	"officelite/excel/model"
)

// Palette indexes with special meaning.
const (
	icvSysText = 0x40 // system window text (fonts, borders)
	icvSysBack = 0x41 // system window background
	icvAuto    = 0x7FFF
)

// color resolves a BIFF palette index. Indexes 8..63 can be redefined by the
// PALETTE record; the defaults match OOXML's indexed colors.
func (p *parser) color(icv int, fallback color.NRGBA) color.NRGBA {
	if v, ok := p.palette[icv]; ok {
		return colors.RGB(v)
	}
	if icv >= icvSysText || icv >= len(excelize.IndexedColorMapping) {
		return fallback
	}
	return colors.HexOr(excelize.IndexedColorMapping[icv], fallback)
}

// fontAt maps an XF font index to the FONT table; index 4 is never written,
// so later fonts are shifted by one.
func (p *parser) fontAt(i int) (font, bool) {
	if i >= 4 {
		i--
	}
	if i < 0 || i >= len(p.fonts) {
		return font{}, false
	}
	return p.fonts[i], true
}

// buildStyles converts every XF into a model style. Styles[0] is the Normal
// style built from font 0.
func (p *parser) buildStyles() {
	def := model.DefaultStyle()
	if f, ok := p.fontAt(0); ok {
		def.Font = p.modelFont(f, def.Font)
	}
	p.wb.Styles = []model.Style{def}
	cache := map[model.Style]uint32{def: 0}
	p.xfStyle = make([]uint32, len(p.xfs))
	for i, x := range p.xfs {
		st := model.Style{Font: def.Font}
		if f, ok := p.fontAt(x.font); ok {
			st.Font = p.modelFont(f, def.Font)
		}
		st.NumFmtID, st.NumFmtCode = x.format, p.formats[x.format]
		st.HAlign = model.HAlign(x.hAlign) // BIFF order matches model.HAlign
		st.VAlign = map[int]model.VAlign{
			0: model.VAlignTop, 1: model.VAlignCenter, 2: model.VAlignBottom,
			3: model.VAlignJustify, 4: model.VAlignDistributed,
		}[x.vAlign]
		st.Wrap = x.wrap != 0
		st.Indent = x.indent
		st.Rotation = x.rotation
		for side := range 4 {
			if x.border[side] != 0 {
				st.Border[side] = model.Edge{
					Style: model.BorderStyle(x.border[side]),
					Color: p.color(x.borderColor[side], colors.Black),
				}
			}
		}
		switch {
		case x.pattern == 1:
			st.Fill = p.color(x.fore, color.NRGBA{})
		case x.pattern > 1:
			// Hatch patterns: approximate with the blend of both colors.
			fg := p.color(x.fore, colors.Black)
			bg := p.color(x.back, colors.White)
			st.Fill = colors.Mix(bg, fg, 0.5)
		}
		p.xfStyle[i] = p.wb.AddStyle(st, cache)
	}
}

func (p *parser) modelFont(f font, def model.Font) model.Font {
	mf := model.Font{
		Family:    f.name,
		SizePt:    float32(f.heightTw) / 20,
		Bold:      f.bold,
		Italic:    f.italic,
		Strike:    f.strike,
		Underline: f.underline,
		Color:     p.color(f.color, colors.Black),
	}
	if mf.Family == "" {
		mf.Family = def.Family
	}
	if mf.SizePt <= 0 {
		mf.SizePt = def.SizePt
	}
	return mf
}

// cellFormat returns the model style index and number format of an XF.
func (p *parser) cellFormat(ixfe int) (style uint32, fmtID int, code string) {
	if ixfe < 0 || ixfe >= len(p.xfs) {
		return 0, 0, ""
	}
	x := p.xfs[ixfe]
	return p.xfStyle[ixfe], x.format, p.formats[x.format]
}
