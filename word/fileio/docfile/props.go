package docfile

import (
	"math"
	"sort"

	"officelite/word/model"
)

// Toggle properties that can be relative to the style (0x80 / 0x81).
const (
	togBold = iota
	togItalic
	togStrike
	togSmallCaps
	togCaps
	togHidden
	nToggles
)

// chpInfo is decoded character formatting plus the special-character
// flags the reader needs.
type chpInfo struct {
	props   model.RunProps
	istd    int // character style, -1 = none
	toggles [nToggles]byte
	spec    bool
	picLoc  uint32
	hasPic  bool
	ole2    bool
	fData   bool
	rmDel   bool
	grpprl  []byte
}

func toggleField(p *model.RunProps, t int) *model.Tri {
	switch t {
	case togBold:
		return &p.Bold
	case togItalic:
		return &p.Italic
	case togStrike:
		return &p.Strike
	case togSmallCaps:
		return &p.SmallCaps
	case togCaps:
		return &p.Caps
	}
	return &p.Hidden
}

var toggleSprms = map[uint16]int{
	sprmCFBold: togBold, sprmCFItalic: togItalic, sprmCFStrike: togStrike,
	sprmCFSmallCaps: togSmallCaps, sprmCFCaps: togCaps, sprmCFVanish: togHidden,
}

// decodeChp applies character sprms. Toggles of 0x80/0x81 are recorded in
// c.toggles for resolveToggles.
func (r *reader) decodeChp(c *chpInfo, g []byte) {
	eachSprm(g, func(op uint16, a []byte) {
		if t, ok := toggleSprms[op]; ok {
			switch v := u8(a); v {
			case 0, 1:
				*toggleField(&c.props, t) = model.TriOf(v == 1)
				c.toggles[t] = 0
			case 0x80, 0x81:
				*toggleField(&c.props, t) = model.Unset
				c.toggles[t] = v
			}
			return
		}
		switch op {
		case sprmCRgFtc0, sprmCFtcDefault:
			if f := r.font(u16(a)); f != "" {
				c.props.Font = f
			}
		case sprmCHps:
			c.props.Size = float64(u16(a)) / 2
		case sprmCIco:
			c.props.Color = icoColor(u8(a))
			if u8(a) == 0 {
				c.props.Color = "auto"
			}
		case sprmCCv:
			c.props.Color = colorRef(a)
			if c.props.Color == "" {
				c.props.Color = "auto"
			}
		case sprmCKul:
			c.props.Underline = kulToModel(u8(a))
		case sprmCHighlight:
			if v := u8(a); v == 0 {
				c.props.Highlight = "none"
			} else if int(v) < len(highlightNames) {
				c.props.Highlight = highlightNames[v]
			}
		case sprmCShd:
			if h := shdColor(a); h != "" {
				c.props.Highlight = h
			}
		case sprmCShd80:
			if h := shd80Color(u16(a)); h != "" {
				c.props.Highlight = h
			}
		case sprmCIss:
			switch u8(a) {
			case 0:
				c.props.Vert = model.VertBaseline
			case 1:
				c.props.Vert = model.VertSuper
			case 2:
				c.props.Vert = model.VertSub
			}
		case sprmCIstd:
			c.istd = int(u16(a))
		case sprmCFSpec:
			c.spec = u8(a) != 0
		case sprmCPicLocation:
			c.picLoc, c.hasPic = u32(a), true
		case sprmCFOle2:
			c.ole2 = u8(a) != 0
		case sprmCFData:
			c.fData = u8(a) != 0
		case sprmCFRMarkDel:
			c.rmDel = u8(a) != 0
		}
	})
}

// fixColor turns the "auto" marker into "" (the model has no explicit
// automatic color); it only exists so a later sprm can reset an earlier one.
func fixColor(p *model.RunProps) {
	if p.Color == "auto" {
		p.Color = ""
	}
	if p.Highlight == "none" {
		p.Highlight = ""
	}
}

// resolveToggles turns 0x80/0x81 toggles into absolute values given the
// value the style chain produces.
func resolveToggles(c *chpInfo, base model.RunProps) model.RunProps {
	p := c.props
	for t, v := range c.toggles {
		b := toggleField(&base, t).Bool(false)
		switch v {
		case 0x80:
			*toggleField(&p, t) = model.Unset
		case 0x81:
			*toggleField(&p, t) = model.TriOf(!b)
		}
	}
	if p.Color == "auto" && base.Color != "" {
		p.Color = "000000" // explicit automatic color over a colored style
	}
	if p.Highlight == "none" && base.Highlight != "" {
		p.Highlight = "" // can't express "no highlight" over a highlighted style
	}
	fixColor(&p)
	return p
}

func kulToModel(k byte) model.Underline {
	switch k {
	case 0:
		return model.UnderlineNone
	case 1:
		return model.UnderlineSingle
	case 2:
		return model.UnderlineWords
	case 3:
		return model.UnderlineDouble
	case 4, 20:
		return model.UnderlineDotted
	case 6:
		return model.UnderlineThick
	case 7, 9, 10, 23, 25, 26, 39, 55:
		return model.UnderlineDashed
	case 11, 27, 43:
		return model.UnderlineWave
	}
	return model.UnderlineSingle
}

func kulFromModel(u model.Underline) byte {
	switch u {
	case model.UnderlineSingle:
		return 1
	case model.UnderlineWords:
		return 2
	case model.UnderlineDouble:
		return 3
	case model.UnderlineDotted:
		return 4
	case model.UnderlineThick:
		return 6
	case model.UnderlineDashed:
		return 7
	case model.UnderlineWave:
		return 11
	}
	return 0
}

// papInfo is decoded paragraph formatting plus table information.
type papInfo struct {
	props      model.ParaProps
	inTable    bool
	ttp        bool
	itap       int
	hasItap    bool
	innerCell  bool
	innerTtp   bool
	ilfo, ilvl int
	hasIlfo    bool
}

func (r *reader) decodePap(p *papInfo, g []byte) {
	eachSprm(g, func(op uint16, a []byte) {
		pp := &p.props
		switch op {
		case sprmPJc80, sprmPJc:
			switch u8(a) {
			case 0:
				pp.Align = model.AlignLeft
			case 1:
				pp.Align = model.AlignCenter
			case 2:
				pp.Align = model.AlignRight
			default:
				pp.Align = model.AlignJustify
			}
		case sprmPDyaBefore:
			pp.SpaceBefore = model.F(twips(int(u16(a))))
		case sprmPDyaAfter:
			pp.SpaceAfter = model.F(twips(int(u16(a))))
		case sprmPDyaLine:
			dya, mult := int(i16(a)), i16(a[2:])
			switch {
			case mult != 0:
				// Multiples are kept to two decimals, as Word shows them.
				pp.Line, pp.LineRule = math.Round(float64(dya)/2.4)/100, model.LineAuto
			case dya < 0:
				pp.Line, pp.LineRule = twips(-dya), model.LineExact
			default:
				pp.Line, pp.LineRule = twips(dya), model.LineAtLeast
			}
		case sprmPDxaLeft, sprmPDxaLeft80:
			pp.IndentLeft = model.F(twips(int(i16(a))))
		case sprmPDxaRight, sprmPDxaRight80:
			pp.IndentRight = model.F(twips(int(i16(a))))
		case sprmPDxaLeft1, sprmPDxaLeft180:
			pp.IndentFirst = model.F(twips(int(i16(a))))
		case sprmPFKeep:
			pp.KeepLines = model.TriOf(u8(a) != 0)
		case sprmPFKeepFollow:
			pp.KeepNext = model.TriOf(u8(a) != 0)
		case sprmPFPageBreakBefore:
			pp.PageBreakBefore = model.TriOf(u8(a) != 0)
		case sprmPFWidowControl:
			pp.WidowControl = model.TriOf(u8(a) != 0)
		case sprmPShd:
			pp.Shading = shdColor(a)
		case sprmPShd80:
			pp.Shading = shd80Color(u16(a))
		case sprmPChgTabsPapx, sprmPChgTabs:
			pp.Tabs = mergeTabChanges(pp.Tabs, decodeTabs(op, a))
		case sprmPIlvl:
			p.ilvl = int(u8(a))
		case sprmPIlfo:
			p.ilfo, p.hasIlfo = int(i16(a)), true
		case sprmPOutLvl:
			if v := int(u8(a)); v < 9 {
				pp.OutlineLevel = v + 1
			} else {
				pp.OutlineLevel = 0
			}
		case sprmPFInTable:
			p.inTable = u8(a) != 0
		case sprmPFTtp:
			p.ttp = u8(a) != 0
		case sprmPItap:
			p.itap, p.hasItap = int(u32(a)), true
		case sprmPDtap:
			p.itap += int(int32(u32(a)))
			p.hasItap = true
		case sprmPFInnerTableCell:
			p.innerCell = u8(a) != 0
		case sprmPFInnerTtp:
			p.innerTtp = u8(a) != 0
		}
	})
}

// listRef converts the decoded ilfo/ilvl into the model's list reference.
func (p *papInfo) listRef() *model.ListRef {
	if !p.hasIlfo {
		return nil
	}
	if p.ilfo <= 0 || p.ilfo >= 2047 {
		return &model.ListRef{NumID: 0}
	}
	lvl := p.ilvl
	if lvl > 8 {
		lvl = 8
	}
	return &model.ListRef{NumID: p.ilfo, Level: lvl}
}

// decodeTabs reads a sprmPChgTabsPapx / sprmPChgTabs operand.
func decodeTabs(op uint16, a []byte) []model.TabStop {
	var out []model.TabStop
	if len(a) < 1 {
		return nil
	}
	nDel := int(a[0])
	p := 1
	if p+2*nDel > len(a) {
		return nil
	}
	for i := 0; i < nDel; i++ {
		out = append(out, model.TabStop{Pos: twips(int(i16(a[p+2*i:]))), Clear: true})
	}
	p += 2 * nDel
	if op == sprmPChgTabs {
		p += 2 * nDel // rgdxaClose
	}
	if p >= len(a) {
		return out
	}
	nAdd := int(a[p])
	p++
	if p+3*nAdd > len(a) {
		return out
	}
	for i := 0; i < nAdd; i++ {
		pos := twips(int(i16(a[p+2*i:])))
		tbd := a[p+2*nAdd+i]
		ts := model.TabStop{Pos: pos}
		switch tbd & 7 {
		case 0:
			ts.Align = model.AlignLeft
		case 1:
			ts.Align = model.AlignCenter
		case 2:
			ts.Align = model.AlignRight
		case 3:
			ts.Align, ts.Decimal = model.AlignLeft, true
		case 4:
			continue // bar tab: not modeled
		}
		switch tbd >> 3 & 7 {
		case 1, 5:
			ts.Leader = '.'
		case 2:
			ts.Leader = '-'
		case 3, 4:
			ts.Leader = '_'
		}
		out = append(out, ts)
	}
	return out
}

// mergeTabChanges applies tab changes to the stops set so far in the same
// grpprl (a paragraph's own list keeps its clears so they override the style).
func mergeTabChanges(cur, chg []model.TabStop) []model.TabStop {
	out := cur[:0:0]
	for _, t := range cur {
		keep := true
		for _, c := range chg {
			if c.Pos == t.Pos {
				keep = false
			}
		}
		if keep {
			out = append(out, t)
		}
	}
	out = append(out, chg...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pos < out[j].Pos })
	return out
}
