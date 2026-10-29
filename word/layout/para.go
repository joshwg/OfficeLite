package layout

import (
	"image/color"
	"math"
	"slices"
	"unicode"
	"unicode/utf8"

	"officelite/common/colors"
	"officelite/word/model"
)

// ItemKind is what an item on a line is.
type ItemKind int8

const (
	ItemText      ItemKind = iota
	ItemTab                // a tab; W is the space up to the stop
	ItemBreak              // a line break (Shift+Enter)
	ItemPageBreak          // a page break
	ItemObject             // an opaque run: picture, field...
	ItemLabel              // a list number or bullet (not part of the text)
)

// Item is a piece of a line: a word (with its trailing spaces), a tab, a
// break or an object.
type Item struct {
	Kind   ItemKind
	Off, N int    // first rune offset in the paragraph, caret positions taken
	Text   string // displayed text (caps applied); objects: placeholder label
	X, W   float64
	Adv    []float32 // per-rune advances (len N)
	St     *TextStyle
	Asc    float64
	Desc   float64
	wAsc   float64 // Word's ascent and descent (see measured)
	wDesc  float64
	Leader byte          // tab leader
	Obj    *model.Opaque // ItemObject
	trail  float64       // width of trailing spaces
	brk    bool          // a line may break after this item
	tab    model.TabStop // ItemTab: the stop it goes to
	spaces int           // trailing spaces
	// stretched is set on justified words whose spaces were widened.
	stretched bool
}

func (it *Item) special() bool { return it.Kind != ItemText }

// Line is one line of a paragraph. Item X positions are relative to the
// left edge of the paragraph's text column (the page margin or the cell's
// content box).
type Line struct {
	Start, End  int // rune offsets [Start, End)
	Items       []Item
	Label       *Item
	Left, Right float64 // indented text area
	X0          float64 // where the text starts after alignment
	Height      float64 // line pitch
	Baseline    float64 // from the top of the line
	Width       float64 // right edge of the text (no trailing spaces)
	Asc, Desc   float64 // tallest ascent and descent on the line
	PageBreak   bool    // ends with a page break
	HardBreak   bool    // ends with a line break
	Last        bool    // last line of the paragraph
}

// Para is a laid-out paragraph.
type Para struct {
	P             *model.Paragraph
	Props         model.ParaProps
	Lines         []*Line
	Left, Right   float64 // indents (Right from the right edge)
	Before, After float64
	Shading       color.NRGBA
	Height        float64 // sum of line heights

	width float64
	label string
	mark  *TextStyle   // the paragraph mark's style (for ¶)
	table *model.Table // the table the paragraph is in, or nil
}

// tabber finds tab stops: custom stops, the hanging indent's implicit stop
// and Word's default stops every half inch, measured from the left edge
// of the text column.
type tabber struct {
	stops []model.TabStop
	hang  float64 // implicit stop (the left indent with a hanging first line), or -1
	every float64 // default stop interval
}

func (t *tabber) next(x float64) model.TabStop {
	best := model.TabStop{Pos: math.Inf(1), Align: model.AlignLeft}
	for _, s := range t.stops {
		if !s.Clear && s.Pos > x+0.01 && s.Pos < best.Pos {
			best = s
		}
	}
	if t.hang > x+0.01 && t.hang < best.Pos {
		best = model.TabStop{Pos: t.hang, Align: model.AlignLeft}
	}
	if math.IsInf(best.Pos, 1) {
		best = model.TabStop{Pos: (math.Floor(x/t.every+0.001) + 1) * t.every, Align: model.AlignLeft}
	}
	return best
}

// atoms splits a paragraph's runs into measured items, words split after
// spaces and hyphens.
func (e *Engine) atoms(p *model.Paragraph) []Item {
	var out []Item
	off := 0
	for ri := range p.Runs {
		r := &p.Runs[ri]
		rp := e.Doc.ResolveRun(p, r)
		if isLink(r.Span) && r.Style == "" {
			// A hyperlink without a character style looks like Word's.
			if r.Props.Color == "" {
				rp.Color = "0563C1"
			}
			if r.Props.Underline == model.UnderlineUnset {
				rp.Underline = model.UnderlineSingle
			}
		}
		st := e.style(rp)
		n := model.RunLen(r)
		switch r.Kind {
		case model.RunText:
			txt := mapSymbols(r.Text)
			if rp.Caps.Bool(false) {
				txt = upper(txt)
			}
			for _, w := range splitWords(txt) {
				if rp.SmallCaps.Bool(false) && !rp.Caps.Bool(false) {
					for _, seg := range splitCase(w) {
						s := st
						if seg.lower {
							c := *st
							c.Size = st.Size * 0.8
							s = e.intern(&c)
						}
						out = append(out, e.textItem(s, upper(seg.text), off))
						off += utf8.RuneCountInString(seg.text)
					}
					out[len(out)-1].brk = endsBreakable(w)
					continue
				}
				out = append(out, e.textItem(st, w, off))
				off += utf8.RuneCountInString(w)
			}
		case model.RunTab:
			it := Item{Kind: ItemTab, Off: off, N: 1, St: st, brk: true, Adv: []float32{0}}
			m := e.m.measure(st, "")
			it.Asc, it.Desc, it.wAsc, it.wDesc = m.ascent, m.desc, m.wasc, m.wdesc
			out = append(out, it)
			off += n
		case model.RunBreak, model.RunPageBreak:
			k := ItemBreak
			if r.Kind == model.RunPageBreak {
				k = ItemPageBreak
			}
			out = append(out, Item{Kind: k, Off: off, N: 1, St: st, brk: true, Adv: []float32{0}})
			off += n
		case model.RunOpaque:
			it := Item{Kind: ItemObject, Off: off, N: 1, St: st, Obj: r.Opaque, brk: true, Text: e.fieldText(r)}
			m := e.m.measure(st, "")
			it.Asc, it.Desc, it.wAsc, it.wDesc = m.ascent, m.desc, m.wasc, m.wdesc
			if r.Opaque != nil && r.Opaque.Width > 0 {
				it.W = r.Opaque.Width
				if r.Opaque.Height > 0 {
					it.Asc, it.Desc, it.wAsc, it.wDesc = r.Opaque.Height, 0, r.Opaque.Height, 0
				}
			} else if it.Text != "" {
				it.W = e.m.measure(st, it.Text).width
			}
			if st.Hidden {
				it.W = 0
			}
			it.Adv = []float32{float32(it.W)}
			out = append(out, it)
			off += n
		}
	}
	return out
}

func (e *Engine) textItem(st *TextStyle, w string, off int) Item {
	m := e.m.measure(st, w)
	it := Item{Kind: ItemText, Off: off, N: len(m.adv), Text: w, St: st, Adv: m.adv, W: m.width,
		trail: m.trailingSpace, Asc: m.ascent, Desc: m.desc, wAsc: m.wasc, wDesc: m.wdesc, brk: endsBreakable(w)}
	for i := len(w) - 1; i >= 0 && w[i] == ' '; i-- {
		it.spaces++
	}
	if st.Hidden {
		it.Adv = make([]float32, it.N)
		it.W, it.trail, it.spaces = 0, 0, 0 // no width to widen when justified
	}
	if st.Shift > 0 {
		it.Asc += st.Shift
		it.wAsc += st.Shift
	} else {
		it.Desc -= st.Shift
		it.wDesc -= st.Shift
	}
	return it
}

func endsBreakable(w string) bool {
	r, _ := utf8.DecodeLastRuneInString(w)
	return r == ' ' || r == '-' || r == '­' || r == '​'
}

func isLink(s *model.Span) bool {
	for ; s != nil; s = s.Parent {
		if s.Kind == model.SpanLink {
			return true
		}
	}
	return false
}

// splitWords splits text after each run of spaces and after hyphens.
func splitWords(s string) []string {
	var out []string
	start := 0
	prevSpace := false
	for i, r := range s {
		if prevSpace && r != ' ' {
			out = append(out, s[start:i])
			start = i
		}
		prevSpace = r == ' '
		if r == '-' && i+1 < len(s) && s[i+1] != ' ' {
			out = append(out, s[start:i+1])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

type caseSeg struct {
	text  string
	lower bool
}

// splitCase splits text into lowercase and other stretches, for small caps.
func splitCase(s string) []caseSeg {
	var out []caseSeg
	start := 0
	cur := false
	for i, r := range s {
		l := unicode.IsLower(r)
		if unicode.IsSpace(r) && i > start {
			l = cur // spaces take the size of the text they follow
		}
		if i > start && l != cur {
			out = append(out, caseSeg{s[start:i], cur})
			start = i
		}
		cur = l
	}
	return append(out, caseSeg{s[start:], cur})
}

// paraGeometry returns a paragraph's indents, list level and spacing.
func (e *Engine) paraGeometry(p *model.Paragraph, t *model.Table) (pp model.ParaProps, left, right, first float64, lvl *model.ListLevel) {
	pp = e.Doc.ResolvePara(p)
	if t != nil {
		// Table styles (Table Grid: single spacing, no space after)
		// override the document defaults but not paragraph styles.
		ts := e.tableStylePara(t)
		if ts.SpaceAfter != nil && p.Props.SpaceAfter == nil && !e.styleSets(p.Style, func(q model.ParaProps) bool { return q.SpaceAfter != nil }) {
			pp.SpaceAfter = ts.SpaceAfter
		}
		if ts.LineRule != model.LineUnset && p.Props.LineRule == model.LineUnset && !e.styleSets(p.Style, func(q model.ParaProps) bool { return q.LineRule != model.LineUnset }) {
			pp.Line, pp.LineRule = ts.Line, ts.LineRule
		}
	}
	if pp.IndentLeft != nil {
		left = *pp.IndentLeft
	}
	if pp.IndentRight != nil {
		right = *pp.IndentRight
	}
	if pp.IndentFirst != nil {
		first = *pp.IndentFirst
	}
	if l := pp.List; l != nil && l.NumID != 0 {
		if list := e.Doc.Lists[l.NumID]; list != nil && l.Level < len(list.Levels) {
			lvl = &list.Levels[l.Level]
			// Numbering indents override the style's, not direct ones.
			if p.Props.IndentLeft == nil && (lvl.Indent != 0 || lvl.Hang != 0) {
				left = lvl.Indent
			}
			if p.Props.IndentFirst == nil && (lvl.Indent != 0 || lvl.Hang != 0) {
				first = -lvl.Hang
			}
		}
	}
	return
}

// tableStylePara returns the paragraph properties of a table's style (the
// default table style if it has none) and the styles it is based on. A
// document without table styles gets Table Grid's.
func (e *Engine) tableStylePara(t *model.Table) model.ParaProps {
	st := e.Doc.Styles.Find(t.Style)
	if st == nil || st.Type != model.TableStyle {
		st = e.Doc.Styles.Default(model.TableStyle)
	}
	if st == nil {
		return model.ParaProps{SpaceAfter: model.F(0), Line: 1, LineRule: model.LineAuto}
	}
	var chain []*model.Style
	seen := map[string]bool{}
	for ; st != nil && !seen[st.ID]; st = e.Doc.Styles.Find(st.BasedOn) {
		seen[st.ID] = true
		chain = append(chain, st)
	}
	var pp model.ParaProps
	for i := len(chain) - 1; i >= 0; i-- {
		pp = pp.Merge(chain[i].Para)
	}
	return pp
}

// styleSets reports whether the paragraph style (not the default style it
// may be based on) sets a property.
func (e *Engine) styleSets(id string, has func(model.ParaProps) bool) bool {
	seen := map[string]bool{}
	for st := e.Doc.Styles.Find(id); st != nil && !st.Default && !seen[st.ID]; st = e.Doc.Styles.Find(st.BasedOn) {
		seen[st.ID] = true
		if st.Type == model.ParagraphStyle && has(st.Para) {
			return true
		}
	}
	return false
}

// layoutPara breaks a paragraph into lines for a text column width wide.
func (e *Engine) layoutPara(p *model.Paragraph, width float64, label string, t *model.Table) *Para {
	pp, left, right, first, lvl := e.paraGeometry(p, t)
	out := &Para{P: p, Props: pp, Left: left, Right: right, width: width, label: label, table: t}
	if pp.SpaceBefore != nil {
		out.Before = *pp.SpaceBefore
	}
	if pp.SpaceAfter != nil {
		out.After = *pp.SpaceAfter
	}
	if pp.Shading != "" {
		if c, ok := colors.ParseHex(pp.Shading); ok {
			out.Shading = c
		}
	}
	markRun := model.Run{Props: p.Mark}
	markStyle := e.style(e.Doc.ResolveRun(p, &markRun))
	out.mark = markStyle
	mark := e.m.measure(markStyle, "")

	tabs := &tabber{stops: pp.Tabs, hang: -1, every: e.Doc.DefaultTabStop}
	if tabs.every <= 0 {
		tabs.every = 36
	}
	if first < 0 {
		tabs.hang = left
	}
	rightEdge := max(width-right, left+1)
	atoms := e.atoms(p)

	var lines []*Line
	cur := &Line{Left: left, Right: rightEdge}
	x := left + first
	if label != "" && lvl != nil {
		ls := markStyle
		if lvl.Font != "" {
			c := *markStyle
			c.Font.Typeface = styleOf(model.RunProps{Font: lvl.Font}).Font.Typeface
			c.Family = lvl.Font
			ls = e.intern(&c)
		}
		// A Symbol bullet is drawn from Symbol, as Word does; the Unicode
		// stand-in comes from another font, with a smaller dot and the
		// taller line of that font's ascent.
		if m := mapSymbols(label); m != label && (lvl.Font == "" || !e.m.has(ls, lvl.Font, label)) {
			label = m
		}
		lit := e.textItem(ls, label, 0)
		lit.Kind, lit.N, lit.Adv = ItemLabel, 0, nil
		lit.X = x
		cur.Label = &lit
		x += lit.W
		x = tabs.next(x).Pos
	}
	lineStart := x
	finish := func(last bool) {
		cur.Last = last
		e.finishLine(cur, lineStart, pp, mark, tabs)
		lines = append(lines, cur)
		end := cur.End
		cur = &Line{Left: left, Right: rightEdge, Start: end, End: end}
		x = left
		lineStart = x
	}
	place := func(it Item) {
		if len(cur.Items) == 0 {
			cur.Start = it.Off
		}
		cur.Items = append(cur.Items, it)
		cur.End = it.Off + it.N
		x += it.W
	}
	for i := 0; i < len(atoms); {
		// A word is a run of text atoms up to a break opportunity.
		j := i + 1
		if !atoms[i].special() {
			for j = i; j < len(atoms) && !atoms[j].special(); {
				j++
				if atoms[j-1].brk {
					break
				}
			}
		}
		word := atoms[i:j]
		i = j
		switch a := word[0]; a.Kind {
		case ItemBreak, ItemPageBreak:
			place(a)
			cur.HardBreak = a.Kind == ItemBreak
			cur.PageBreak = a.Kind == ItemPageBreak
			finish(false)
			continue
		case ItemTab:
			stop := tabs.next(x)
			if stop.Pos > width+0.01 && len(cur.Items) > 0 {
				finish(false)
				stop = tabs.next(x)
			}
			a.tab, a.Leader = stop, stop.Leader
			a.W = max(stop.Pos-x, 0)
			if stop.Align == model.AlignRight || stop.Align == model.AlignCenter || stop.Decimal {
				// The text up to the next tab ends at (or centers on) the
				// stop; finishLine places it exactly.
				end := i
				seg := 0.0
				for end < len(atoms) && atoms[end].Kind != ItemTab && atoms[end].Kind != ItemBreak && atoms[end].Kind != ItemPageBreak {
					seg += atoms[end].W
					end++
				}
				if stop.Decimal {
					// Only the text before the decimal point lies left of the
					// stop, as finishLine places it.
					if d := decimalAt(atoms[i:end]); d >= 0 {
						seg = d
					}
				}
				if stop.Align == model.AlignCenter {
					seg /= 2
				}
				a.W = max(stop.Pos-x-seg, 0)
			}
			place(a)
			continue
		}
		w := 0.0
		for _, a := range word {
			w += a.W
		}
		w -= word[len(word)-1].trail
		if len(cur.Items) > 0 && x+w > rightEdge+0.01 {
			finish(false)
		}
		if x+w <= rightEdge+0.01 {
			for _, a := range word {
				place(a)
			}
			continue
		}
		// The word is wider than the line: break it anywhere.
		for _, a := range word {
			for a.N > 0 {
				k, used := 0, 0.0
				for k < a.N && (x+used+float64(a.Adv[k]) <= rightEdge+0.01 || k == 0 && len(cur.Items) == 0) {
					used += float64(a.Adv[k])
					k++
				}
				if k == a.N {
					place(a)
					break
				}
				if k > 0 {
					head, tail := splitItem(a, k)
					place(head)
					a = tail
				}
				finish(false)
			}
		}
	}
	if len(cur.Items) > 0 || len(lines) == 0 || lines[len(lines)-1].HardBreak || lines[len(lines)-1].PageBreak {
		if len(lines) > 0 && len(cur.Items) == 0 {
			cur.Start, cur.End = lines[len(lines)-1].End, lines[len(lines)-1].End
		}
		finish(true)
	} else {
		lines[len(lines)-1].Last = true
	}
	out.Lines = lines
	for _, l := range lines {
		out.Height += l.Height
	}
	return out
}

// splitItem splits a text item after k runes.
func splitItem(a Item, k int) (Item, Item) {
	rs := []rune(a.Text)
	h, t := a, a
	h.Text, t.Text = string(rs[:k]), string(rs[k:])
	h.N, t.N = k, a.N-k
	h.Adv, t.Adv = a.Adv[:k:k], a.Adv[k:]
	h.W, t.W = 0, 0
	for _, v := range h.Adv {
		h.W += float64(v)
	}
	for _, v := range t.Adv {
		t.W += float64(v)
	}
	h.trail, h.spaces, h.brk = 0, 0, false
	if k > a.N-a.spaces {
		// Split inside the trailing spaces: the tail keeps only the ones
		// after the split.
		t.spaces = t.N
		t.trail = 0
		for _, v := range t.Adv {
			t.trail += float64(v)
		}
	}
	t.Off = a.Off + k
	return h, t
}

// decimalAt is the width of items before the first decimal point, or -1
// when there is none.
func decimalAt(items []Item) float64 {
	w := 0.0
	for _, it := range items {
		if i := slices.Index([]rune(it.Text), '.'); i >= 0 && i < len(it.Adv) {
			for _, a := range it.Adv[:i] {
				w += float64(a)
			}
			return w
		}
		w += it.W
	}
	return -1
}

// finishLine positions a line's items (tab alignment, paragraph alignment
// and justification) and computes its height and baseline.
func (e *Engine) finishLine(l *Line, start float64, pp model.ParaProps, mark *measured, tabs *tabber) {
	x := start
	items := l.Items
	for k := range items {
		it := &items[k]
		if it.Kind == ItemTab {
			// Right, center and decimal stops align the text up to the
			// next tab against the stop.
			seg, dec := 0.0, -1.0
			for m := k + 1; m < len(items) && items[m].Kind != ItemTab && items[m].Kind != ItemBreak && items[m].Kind != ItemPageBreak; m++ {
				if dec < 0 && it.tab.Decimal {
					if d := decimalAt(items[m : m+1]); d >= 0 {
						dec = seg + d
					}
				}
				seg += items[m].W
				if m == len(items)-1 {
					seg -= items[m].trail
				}
			}
			switch {
			case it.tab.Decimal:
				if dec < 0 {
					dec = seg
				}
				it.W = max(it.tab.Pos-x-dec, 0)
			case it.tab.Align == model.AlignRight:
				it.W = max(it.tab.Pos-x-seg, 0)
			case it.tab.Align == model.AlignCenter:
				it.W = max(it.tab.Pos-x-seg/2, 0)
			default:
				it.W = max(it.tab.Pos-x, 0)
			}
			it.Adv = []float32{float32(it.W)}
		}
		it.X = x
		x += it.W
	}
	width := x
	if n := len(items); n > 0 {
		width -= items[n-1].trail
	}
	free := l.Right - width
	shift := 0.0
	switch pp.Align {
	case model.AlignCenter:
		shift = free / 2
	case model.AlignRight:
		shift = free
	case model.AlignJustify:
		if !l.Last && !l.HardBreak && !l.PageBreak && free > 0 {
			first := 0
			for k := range items {
				if items[k].Kind == ItemTab {
					first = k + 1
				}
			}
			spaces := 0
			for k := first; k < len(items)-1; k++ {
				spaces += items[k].spaces
			}
			if spaces > 0 {
				extra := free / float64(spaces)
				add := 0.0
				for k := first; k < len(items); k++ {
					it := &items[k]
					it.X += add
					if k < len(items)-1 && it.spaces > 0 {
						it.Adv = slices.Clone(it.Adv)
						for s := it.N - it.spaces; s < it.N; s++ {
							it.Adv[s] += float32(extra)
						}
						it.W += extra * float64(it.spaces)
						it.stretched = true
						add += extra * float64(it.spaces)
					}
				}
				width += add
			}
		}
	}
	if shift < 0 {
		shift = 0
	}
	if shift != 0 {
		for k := range items {
			items[k].X += shift
		}
		width += shift
		if l.Label != nil && pp.Align != model.AlignCenter && pp.Align != model.AlignRight {
			l.Label.X += shift
		}
	}
	l.X0 = start + shift
	l.Width = width

	// The line is as tall as Word's tallest ascent plus deepest descent;
	// the baseline stays above Gio's deepest descent, which is where it is
	// on a line of one font.
	asc, desc, wasc, wdesc := 0.0, 0.0, 0.0, 0.0
	for k := range items {
		if it := &items[k]; !it.St.Hidden && it.Kind != ItemBreak && it.Kind != ItemPageBreak {
			asc, desc = max(asc, it.Asc), max(desc, it.Desc)
			wasc, wdesc = max(wasc, it.wAsc), max(wdesc, it.wDesc)
		}
	}
	if lb := l.Label; lb != nil {
		asc, desc = max(asc, lb.Asc), max(desc, lb.Desc)
		wasc, wdesc = max(wasc, lb.wAsc), max(wdesc, lb.wDesc)
	}
	if asc == 0 && desc == 0 {
		asc, desc, wasc, wdesc = mark.ascent, mark.desc, mark.wasc, mark.wdesc
	}
	l.Asc, l.Desc = asc, desc
	natural := wasc + wdesc
	if natural <= 0 {
		natural = asc + desc
	}
	h := natural
	switch pp.LineRule {
	case model.LineAuto:
		if pp.Line > 0 {
			h = natural * pp.Line
		}
	case model.LineExact:
		h = pp.Line
	case model.LineAtLeast:
		h = max(natural, pp.Line)
	}
	l.Height = h
	l.Baseline = h - desc
	if pp.LineRule == model.LineExact && h < natural {
		// Exact spacing clips: keep the baseline proportionally placed.
		l.Baseline = h * asc / (asc + desc)
	}
}

// intern returns a shared copy of st so equal styles share measurements.
func (e *Engine) intern(st *TextStyle) *TextStyle {
	if s, ok := e.styles[*st]; ok {
		return s
	}
	e.styles[*st] = st
	return st
}

func (e *Engine) style(rp model.RunProps) *TextStyle {
	st := styleOf(rp)
	if st.Hidden && e.showHidden {
		st.Hidden, st.Revealed = false, true
	}
	return e.intern(st)
}
