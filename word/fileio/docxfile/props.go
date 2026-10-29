package docxfile

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"officelite/common/colors"
	"officelite/word/model"
)

// theme is what WordLite uses from theme1.xml: the major/minor Latin fonts
// and the color scheme.
type theme struct {
	major, minor string
	colors       map[string]string // "accent1" → "4472C4"; keys are the w:themeColor names
}

func (t *theme) font(name string) string {
	if t == nil {
		return ""
	}
	switch {
	case strings.HasPrefix(name, "major"):
		return t.major
	case strings.HasPrefix(name, "minor"):
		return t.minor
	}
	return ""
}

func (t *theme) color(name, tint, shade string) string {
	if t == nil {
		return ""
	}
	hex := t.colors[name]
	if hex == "" {
		return ""
	}
	c, ok := colors.ParseHex(hex)
	if !ok {
		return ""
	}
	if v, err := strconv.ParseUint(shade, 16, 8); err == nil && shade != "" {
		c = colors.Mix(colors.Black, c, float64(v)/255)
	}
	if v, err := strconv.ParseUint(tint, 16, 8); err == nil && tint != "" {
		c = colors.Mix(colors.White, c, float64(v)/255)
	}
	return strings.ToUpper(strconv.FormatUint(uint64(c.R)<<16|uint64(c.G)<<8|uint64(c.B)|1<<24, 16)[1:])
}

// ---- units ----

func twipsPt(s string) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return v / 20, true
}

func ptTwips(v float64) string { return strconv.Itoa(int(math.Round(v * 20))) }

func atoi(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

func onOff(n *node) model.Tri {
	if n == nil {
		return model.Unset
	}
	v, ok := n.attrOK("w:val")
	if !ok {
		return model.On
	}
	switch strings.ToLower(v) {
	case "0", "false", "off":
		return model.Off
	}
	return model.On
}

func hexColor(v string) string {
	if strings.EqualFold(strings.TrimSpace(v), "auto") {
		return ""
	}
	c, ok := colors.ParseHex(v)
	if !ok {
		return ""
	}
	return colors.Hex(c)
}

// shdColor is the visible color of a <w:shd>: the pattern color for solid
// shading, otherwise the fill.
func shdColor(n *node, th *theme) string {
	if n == nil {
		return ""
	}
	if n.attr("w:val") == "solid" {
		if c := hexColor(n.attr("w:color")); c != "" {
			return c
		}
	}
	if c := hexColor(n.attr("w:fill")); c != "" {
		return c
	}
	if tc := n.attr("w:themeFill"); tc != "" {
		return th.color(tc, n.attr("w:themeFillTint"), n.attr("w:themeFillShade"))
	}
	return ""
}

// ---- run properties ----

var underlineByVal = map[string]model.Underline{
	"none": model.UnderlineNone, "single": model.UnderlineSingle, "words": model.UnderlineWords,
	"double": model.UnderlineDouble, "thick": model.UnderlineThick,
	"dotted": model.UnderlineDotted, "dottedHeavy": model.UnderlineDotted,
	"dash": model.UnderlineDashed, "dashedHeavy": model.UnderlineDashed, "dashLong": model.UnderlineDashed,
	"dashLongHeavy": model.UnderlineDashed, "dotDash": model.UnderlineDashed, "dashDotHeavy": model.UnderlineDashed,
	"dotDotDash": model.UnderlineDashed, "dashDotDotHeavy": model.UnderlineDashed,
	"wave": model.UnderlineWave, "wavyHeavy": model.UnderlineWave, "wavyDouble": model.UnderlineWave,
}

var valByUnderline = map[model.Underline]string{
	model.UnderlineNone: "none", model.UnderlineSingle: "single", model.UnderlineWords: "words",
	model.UnderlineDouble: "double", model.UnderlineThick: "thick", model.UnderlineDotted: "dotted",
	model.UnderlineDashed: "dash", model.UnderlineWave: "wave",
}

// parseRPr reads the children of a <w:rPr>. It returns the character style
// (w:rStyle) separately; DocxXML is left empty.
func parseRPr(kids []*node, th *theme) (model.RunProps, string) {
	var p model.RunProps
	var style string
	var hl, shd string
	for _, k := range kids {
		switch k.name {
		case "w:rStyle":
			style = k.attr("w:val")
		case "w:rFonts":
			p.Font = rFont(k, th)
		case "w:sz":
			if v, err := strconv.ParseFloat(k.attr("w:val"), 64); err == nil {
				p.Size = v / 2
			}
		case "w:b":
			p.Bold = onOff(k)
		case "w:i":
			p.Italic = onOff(k)
		case "w:u":
			v, ok := k.attrOK("w:val")
			if !ok {
				p.Underline = model.UnderlineSingle
			} else if u, ok := underlineByVal[v]; ok {
				p.Underline = u
			} else {
				p.Underline = model.UnderlineSingle
			}
		case "w:strike", "w:dstrike":
			if t := onOff(k); t == model.On || p.Strike == model.Unset {
				p.Strike = t
			}
		case "w:caps":
			p.Caps = onOff(k)
		case "w:smallCaps":
			p.SmallCaps = onOff(k)
		case "w:vanish":
			p.Hidden = onOff(k)
		case "w:color":
			c := hexColor(k.attr("w:val"))
			if c == "" {
				if tc := k.attr("w:themeColor"); tc != "" && !strings.EqualFold(k.attr("w:val"), "auto") {
					c = th.color(tc, k.attr("w:themeTint"), k.attr("w:themeShade"))
				}
			}
			p.Color = c
		case "w:highlight":
			if v := k.attr("w:val"); v != "none" {
				hl = v
			}
		case "w:shd":
			shd = shdColor(k, th)
		case "w:vertAlign":
			switch k.attr("w:val") {
			case "superscript":
				p.Vert = model.VertSuper
			case "subscript":
				p.Vert = model.VertSub
			case "baseline":
				p.Vert = model.VertBaseline
			}
		}
	}
	p.Highlight = hl
	if p.Highlight == "" {
		p.Highlight = shd
	}
	return p, style
}

func rFont(k *node, th *theme) string {
	for _, a := range []string{"w:asciiTheme", "w:hAnsiTheme"} {
		if v := k.attr(a); v != "" {
			if f := th.font(v); f != "" {
				return f
			}
		}
	}
	for _, a := range []string{"w:ascii", "w:hAnsi"} {
		if v := k.attr(a); v != "" {
			return v
		}
	}
	return ""
}

// rPrOrder is CT_ParaRPr/CT_RPr's element sequence.
var rPrOrder = order("ins del moveFrom moveTo rStyle rFonts b bCs i iCs caps smallCaps strike dstrike outline shadow emboss imprint noProof snapToGrid vanish webHidden color spacing w kern position sz szCs highlight u effect bdr shd fitText vertAlign rtl cs em lang eastAsianLayout specVanish oMath rPrChange")

var pPrOrder = order("pStyle keepNext keepLines pageBreakBefore framePr widowControl numPr suppressLineNumbers pBdr shd tabs suppressAutoHyphens kinsoku wordWrap overflowPunct topLinePunct autoSpaceDE autoSpaceDN bidi adjustRightInd snapToGrid spacing ind contextualSpacing mirrorIndents suppressOverlap jc textDirection textAlignment textboxTightWrap outlineLvl divId cnfStyle rPr sectPr pPrChange")

func order(s string) map[string]float64 {
	m := map[string]float64{}
	for i, f := range strings.Fields(s) {
		m["w:"+f] = float64(i)
	}
	return m
}

// propGroup is a set of XML elements that together give some model fields.
type propGroup struct {
	names []string
	same  bool   // the model fields equal those derived from the original XML
	gen   string // XML generated from the model (used when !same)
}

// mergeProps writes a property element's children: original elements are
// kept unless the model changed the fields they map to, in which case the
// group's generated XML replaces them; everything is put in schema order.
func mergeProps(orig []*node, groups []propGroup, ord map[string]float64) string {
	type item struct {
		key float64
		xml string
	}
	var items []item
	groupOf := map[string]int{}
	for gi, g := range groups {
		for _, n := range g.names {
			groupOf[n] = gi
		}
	}
	last := -1.0
	for i, k := range orig {
		key, ok := ord[k.name]
		if ok {
			last = key
		} else {
			key = last + 0.001*float64(i+1)/float64(len(orig)+1)
		}
		if gi, ok := groupOf[k.name]; ok && !groups[gi].same {
			continue
		}
		items = append(items, item{key, string(k.raw())})
	}
	for _, g := range groups {
		if g.same || g.gen == "" {
			continue
		}
		key, ok := ord[genName(g.gen)]
		if !ok {
			key = ord[g.names[0]]
		}
		items = append(items, item{key, g.gen})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].key < items[j].key })
	var b strings.Builder
	for _, it := range items {
		b.WriteString(it.xml)
	}
	return b.String()
}

// genName is the name of the first element in generated XML ("w:shd").
func genName(x string) string {
	x = strings.TrimPrefix(x, "<")
	if i := strings.IndexAny(x, " />"); i >= 0 {
		return x[:i]
	}
	return x
}

func triXML(name string, t model.Tri) string {
	switch t {
	case model.On:
		return "<w:" + name + "/>"
	case model.Off:
		return "<w:" + name + ` w:val="0"/>`
	}
	return ""
}

func valXML(name, v string) string {
	if v == "" {
		return ""
	}
	return "<w:" + name + ` w:val="` + escAttr(v) + `"/>`
}

// rPrXML returns the content of a <w:rPr> for style and p.
func rPrXML(style string, p model.RunProps, th *theme) string {
	orig := parseFragment(p.DocxXML)
	o, ostyle := parseRPr(orig, th)
	var fontsGen string
	if p.Font != "" {
		attrs := ` w:ascii="` + escAttr(p.Font) + `" w:hAnsi="` + escAttr(p.Font) + `"`
		for _, k := range orig {
			if k.name == "w:rFonts" {
				for _, a := range []string{"eastAsia", "cs", "hint", "eastAsiaTheme", "cstheme"} {
					if v := k.attr("w:" + a); v != "" {
						attrs += ` w:` + a + `="` + escAttr(v) + `"`
					}
				}
			}
		}
		fontsGen = "<w:rFonts" + attrs + "/>"
	}
	var sizeGen string
	if p.Size > 0 {
		sizeGen = valXML("sz", strconv.FormatFloat(math.Round(p.Size*2), 'f', -1, 64))
	}
	var hlGen string
	if p.Highlight != "" {
		if hexColor(p.Highlight) != "" {
			hlGen = `<w:shd w:val="clear" w:color="auto" w:fill="` + strings.ToUpper(p.Highlight) + `"/>`
		} else {
			hlGen = valXML("highlight", p.Highlight)
		}
	}
	var vertGen string
	switch p.Vert {
	case model.VertSuper:
		vertGen = valXML("vertAlign", "superscript")
	case model.VertSub:
		vertGen = valXML("vertAlign", "subscript")
	case model.VertBaseline:
		vertGen = valXML("vertAlign", "baseline")
	}
	groups := []propGroup{
		{[]string{"w:rStyle"}, style == ostyle, valXML("rStyle", style)},
		{[]string{"w:rFonts"}, p.Font == o.Font, fontsGen},
		{[]string{"w:b"}, p.Bold == o.Bold, triXML("b", p.Bold)},
		{[]string{"w:i"}, p.Italic == o.Italic, triXML("i", p.Italic)},
		{[]string{"w:caps"}, p.Caps == o.Caps, triXML("caps", p.Caps)},
		{[]string{"w:smallCaps"}, p.SmallCaps == o.SmallCaps, triXML("smallCaps", p.SmallCaps)},
		{[]string{"w:strike", "w:dstrike"}, p.Strike == o.Strike, triXML("strike", p.Strike)},
		{[]string{"w:vanish"}, p.Hidden == o.Hidden, triXML("vanish", p.Hidden)},
		{[]string{"w:color"}, p.Color == o.Color, valXML("color", p.Color)},
		{[]string{"w:sz"}, p.Size == o.Size, sizeGen},
		{[]string{"w:highlight", "w:shd"}, p.Highlight == o.Highlight, hlGen},
		{[]string{"w:u"}, p.Underline == o.Underline, valXML("u", valByUnderline[p.Underline])},
		{[]string{"w:vertAlign"}, p.Vert == o.Vert, vertGen},
	}
	return mergeProps(orig, groups, rPrOrder)
}

// ---- paragraph properties ----

// parsePPr reads the children of a <w:pPr> (rPr and sectPr are ignored).
// It returns the paragraph style separately.
func parsePPr(kids []*node, th *theme) (model.ParaProps, string) {
	var p model.ParaProps
	var style string
	for _, k := range kids {
		switch k.name {
		case "w:pStyle":
			style = k.attr("w:val")
		case "w:jc":
			switch k.attr("w:val") {
			case "left", "start":
				p.Align = model.AlignLeft
			case "center":
				p.Align = model.AlignCenter
			case "right", "end":
				p.Align = model.AlignRight
			case "both", "distribute", "thaiDistribute", "lowKashida", "mediumKashida", "highKashida":
				p.Align = model.AlignJustify
			}
		case "w:spacing":
			if v, ok := twipsPt(k.attr("w:before")); ok {
				p.SpaceBefore = model.F(v)
			}
			if v, ok := twipsPt(k.attr("w:after")); ok {
				p.SpaceAfter = model.F(v)
			}
			if lv, err := strconv.ParseFloat(k.attr("w:line"), 64); err == nil {
				switch k.attr("w:lineRule") {
				case "exact":
					p.Line, p.LineRule = lv/20, model.LineExact
				case "atLeast":
					p.Line, p.LineRule = lv/20, model.LineAtLeast
				default:
					p.Line, p.LineRule = lv/240, model.LineAuto
				}
			}
		case "w:ind":
			for _, a := range []string{"w:left", "w:start"} {
				if v, ok := twipsPt(k.attr(a)); ok {
					p.IndentLeft = model.F(v)
					break
				}
			}
			for _, a := range []string{"w:right", "w:end"} {
				if v, ok := twipsPt(k.attr(a)); ok {
					p.IndentRight = model.F(v)
					break
				}
			}
			if v, ok := twipsPt(k.attr("w:firstLine")); ok {
				p.IndentFirst = model.F(v)
			}
			if v, ok := twipsPt(k.attr("w:hanging")); ok {
				p.IndentFirst = model.F(-v)
			}
		case "w:keepNext":
			p.KeepNext = onOff(k)
		case "w:keepLines":
			p.KeepLines = onOff(k)
		case "w:pageBreakBefore":
			p.PageBreakBefore = onOff(k)
		case "w:widowControl":
			p.WidowControl = onOff(k)
		case "w:contextualSpacing":
			p.ContextualSpacing = onOff(k)
		case "w:pBdr":
			for _, b := range k.kids {
				if e := borderEdge(b.name); e >= 0 {
					p.Borders[e] = parseBorder(b)
				}
			}
		case "w:shd":
			p.Shading = shdColor(k, th)
		case "w:tabs":
			for _, t := range k.children("w:tab") {
				pos, ok := twipsPt(t.attr("w:pos"))
				if !ok {
					continue
				}
				ts := model.TabStop{Pos: pos, Align: model.AlignLeft}
				switch t.attr("w:val") {
				case "center":
					ts.Align = model.AlignCenter
				case "right", "end":
					ts.Align = model.AlignRight
				case "decimal":
					ts.Decimal = true
				case "clear":
					ts.Clear = true
				}
				switch t.attr("w:leader") {
				case "dot", "middleDot":
					ts.Leader = '.'
				case "hyphen":
					ts.Leader = '-'
				case "underscore", "heavy":
					ts.Leader = '_'
				}
				p.Tabs = append(p.Tabs, ts)
			}
		case "w:numPr":
			if id := k.child("w:numId"); id != nil {
				p.List = &model.ListRef{NumID: atoi(id.attr("w:val")), Level: atoi(k.child("w:ilvl").attr("w:val"))}
			}
		case "w:outlineLvl":
			if v := atoi(k.attr("w:val")); v >= 0 && v < 9 {
				p.OutlineLevel = v + 1
			}
		}
	}
	return p, style
}

func eqF(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func eqTabs(a, b []model.TabStop) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func eqList(a, b *model.ListRef) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// pPrXML returns the content of a <w:pPr> for style and p; mark is the
// paragraph mark's <w:rPr> content ("" for none, e.g. in styles).
func pPrXML(style string, p model.ParaProps, mark string, th *theme) string {
	var orig []*node
	for _, k := range parseFragment(p.DocxXML) {
		if k.name != "w:rPr" {
			orig = append(orig, k)
		}
	}
	o, ostyle := parsePPr(orig, th)

	var align string
	switch p.Align {
	case model.AlignLeft:
		align = "left"
	case model.AlignCenter:
		align = "center"
	case model.AlignRight:
		align = "right"
	case model.AlignJustify:
		align = "both"
	}
	var spacing string
	if p.SpaceBefore != nil {
		spacing += ` w:before="` + ptTwips(*p.SpaceBefore) + `"`
	}
	if p.SpaceAfter != nil {
		spacing += ` w:after="` + ptTwips(*p.SpaceAfter) + `"`
	}
	switch p.LineRule {
	case model.LineAuto:
		spacing += ` w:line="` + strconv.Itoa(int(math.Round(p.Line*240))) + `" w:lineRule="auto"`
	case model.LineExact:
		spacing += ` w:line="` + ptTwips(p.Line) + `" w:lineRule="exact"`
	case model.LineAtLeast:
		spacing += ` w:line="` + ptTwips(p.Line) + `" w:lineRule="atLeast"`
	}
	if spacing != "" {
		spacing = "<w:spacing" + spacing + "/>"
	}
	var ind string
	if p.IndentLeft != nil {
		ind += ` w:left="` + ptTwips(*p.IndentLeft) + `"`
	}
	if p.IndentRight != nil {
		ind += ` w:right="` + ptTwips(*p.IndentRight) + `"`
	}
	if p.IndentFirst != nil {
		if *p.IndentFirst < 0 {
			ind += ` w:hanging="` + ptTwips(-*p.IndentFirst) + `"`
		} else {
			ind += ` w:firstLine="` + ptTwips(*p.IndentFirst) + `"`
		}
	}
	if ind != "" {
		ind = "<w:ind" + ind + "/>"
	}
	var shd string
	if p.Shading != "" {
		shd = `<w:shd w:val="clear" w:color="auto" w:fill="` + escAttr(p.Shading) + `"/>`
	}
	var tabs string
	for _, t := range p.Tabs {
		val := "left"
		switch {
		case t.Clear:
			val = "clear"
		case t.Decimal:
			val = "decimal"
		case t.Align == model.AlignCenter:
			val = "center"
		case t.Align == model.AlignRight:
			val = "right"
		}
		tabs += `<w:tab w:val="` + val + `"`
		switch t.Leader {
		case '.':
			tabs += ` w:leader="dot"`
		case '-':
			tabs += ` w:leader="hyphen"`
		case '_':
			tabs += ` w:leader="underscore"`
		}
		tabs += ` w:pos="` + ptTwips(t.Pos) + `"/>`
	}
	if tabs != "" {
		tabs = "<w:tabs>" + tabs + "</w:tabs>"
	}
	var num string
	if p.List != nil {
		num = `<w:numPr><w:ilvl w:val="` + strconv.Itoa(p.List.Level) + `"/><w:numId w:val="` + strconv.Itoa(p.List.NumID) + `"/></w:numPr>`
	}
	var outline string
	if p.OutlineLevel > 0 {
		outline = valXML("outlineLvl", strconv.Itoa(p.OutlineLevel-1))
	}
	groups := []propGroup{
		{[]string{"w:pStyle"}, style == ostyle, valXML("pStyle", style)},
		{[]string{"w:keepNext"}, p.KeepNext == o.KeepNext, triXML("keepNext", p.KeepNext)},
		{[]string{"w:keepLines"}, p.KeepLines == o.KeepLines, triXML("keepLines", p.KeepLines)},
		{[]string{"w:pageBreakBefore"}, p.PageBreakBefore == o.PageBreakBefore, triXML("pageBreakBefore", p.PageBreakBefore)},
		{[]string{"w:widowControl"}, p.WidowControl == o.WidowControl, triXML("widowControl", p.WidowControl)},
		{[]string{"w:numPr"}, eqList(p.List, o.List), num},
		{[]string{"w:shd"}, p.Shading == o.Shading, shd},
		{[]string{"w:tabs"}, eqTabs(p.Tabs, o.Tabs), tabs},
		{[]string{"w:spacing"}, eqF(p.SpaceBefore, o.SpaceBefore) && eqF(p.SpaceAfter, o.SpaceAfter) &&
			p.LineRule == o.LineRule && (p.LineRule == model.LineUnset || p.Line == o.Line), spacing},
		{[]string{"w:ind"}, eqF(p.IndentLeft, o.IndentLeft) && eqF(p.IndentRight, o.IndentRight) && eqF(p.IndentFirst, o.IndentFirst), ind},
		{[]string{"w:contextualSpacing"}, p.ContextualSpacing == o.ContextualSpacing, triXML("contextualSpacing", p.ContextualSpacing)},
		{[]string{"w:jc"}, p.Align == o.Align, valXML("jc", align)},
		{[]string{"w:outlineLvl"}, p.OutlineLevel == o.OutlineLevel, outline},
	}
	if mark != "" {
		groups = append(groups, propGroup{[]string{"w:rPr"}, false, "<w:rPr>" + mark + "</w:rPr>"})
	}
	return mergeProps(orig, groups, pPrOrder)
}
