package model

import (
	"strconv"
	"strings"
)

// Fields whose result spans paragraphs (a table of contents) keep their
// result as ordinary paragraphs between the field's begin and end runs
// (Opaque.FieldChar). UpdateTOC regenerates a table of contents from the
// document's headings, as Word's Update Field (F9) does.

// FieldRange is a field kept around its result: from its begin run to
// its end run (inclusive), with its instruction.
type FieldRange struct {
	From, To Pos
	Instr    string
}

// FieldRanges lists the fields of the body whose results span
// paragraphs, outermost first.
func (e *Editor) FieldRanges() []FieldRange { return FieldRangesIn(e.locs) }

// FieldRangesIn is FieldRanges for paragraphs located by Locate.
func FieldRangesIn(locs []ParaLoc) []FieldRange {
	var out []FieldRange
	var open []int // indexes into out
	for i, l := range locs {
		off := 0
		for k := range l.P.Runs {
			r := &l.P.Runs[k]
			if o := r.Opaque; o != nil {
				switch o.FieldChar {
				case FieldBegin, FieldStart:
					open = append(open, len(out))
					out = append(out, FieldRange{From: Pos{i, off}, Instr: o.Field})
				case FieldInstr:
					if n := len(open); n > 0 && out[open[n-1]].Instr == "" {
						out[open[n-1]].Instr = o.Field
					}
				case FieldEnd:
					if n := len(open); n > 0 {
						out[open[n-1]].To = Pos{i, off + 1}
						open = open[:n-1]
					}
				}
			}
			off += RunLen(r)
		}
	}
	// Drop fields left open.
	kept := out[:0]
	for _, f := range out {
		if f.To != (Pos{}) {
			kept = append(kept, f)
		}
	}
	return kept
}

// tocLevels reads the heading levels of a TOC field's \o "a-b" switch
// (all levels for \o alone, 1-3 without it).
func tocLevels(instr string) (int, int) {
	f := strings.Fields(instr)
	for i := 1; i < len(f); i++ {
		if f[i] != `\o` {
			continue
		}
		if i+1 < len(f) {
			a, b, ok := strings.Cut(strings.Trim(f[i+1], `"`), "-")
			lo, err1 := strconv.Atoi(a)
			hi, err2 := strconv.Atoi(b)
			if ok && err1 == nil && err2 == nil && lo >= 1 && hi >= lo {
				return lo, min(hi, 9)
			}
		}
		return 1, 9
	}
	return 1, 3
}

// tocStyleID returns the ID of the "toc n" style, adding Word's
// definition (Normal, indented 11 points per level, 5 points after) when
// the style sheet lacks it.
func tocStyleID(ss *StyleSheet, level int) string {
	n := strconv.Itoa(level)
	return ensureStyle(ss, "toc "+n, "TOC"+n, ParagraphStyle, func(st *Style) {
		st.Para = ParaProps{SpaceAfter: F(5), IndentLeft: F(float64(level-1) * 11)}
		st.Next = "Normal"
	})
}

// UpdateTOC regenerates the tables of contents the selection touches
// (or the first one, when it touches none) from the headings: a
// paragraph in the "toc n" style per heading of the field's levels with
// its text, a dotted right tab at width and its page number (pageOf).
// It reports whether there was a table of contents.
func (e *Editor) UpdateTOC(width float64, pageOf func(*Paragraph) int) bool {
	var toc *FieldRange
	s, t := e.Sel.Ordered()
	for _, f := range e.FieldRanges() {
		if FieldName(f.Instr) != "TOC" || e.locs[f.From.Para].Cell != nil || e.locs[f.To.Para].Cell != nil {
			continue
		}
		if toc == nil || !t.Less(f.From) && !f.To.Less(s) {
			f := f
			toc = &f
			if !t.Less(f.From) && !f.To.Less(s) {
				break
			}
		}
	}
	if toc == nil {
		return false
	}
	lo, hi := tocLevels(toc.Instr)
	bl, el := e.locs[toc.From.Para], e.locs[toc.To.Para]
	defer e.Op("Update Table")()
	e.edit("format", bl.Top, el.Top, true, func() {
		begin, end := bl.P, el.P
		// The runs that stay: up to the field's separator, and from its end.
		bi := begin.RunAt(toc.From.Off)
		prefix := []Run{}
		for k := 0; k < len(begin.Runs); k++ {
			prefix = append(prefix, begin.Runs[k])
			if o := begin.Runs[k].Opaque; k >= bi && o != nil && (o.FieldChar == FieldSep || o.FieldChar == FieldStart) {
				break
			}
		}
		ei := end.RunAt(toc.To.Off - 1)
		suffix := append([]Run(nil), end.Runs[ei:]...)
		keepEnd := ei == 0 && end != begin // the end has a paragraph of its own
		var entries []*Paragraph
		for i, l := range e.locs {
			if l.Cell != nil || i >= toc.From.Para && i <= toc.To.Para {
				continue
			}
			lv := e.Doc.ResolvePara(l.P).OutlineLevel
			if lv < lo || lv > hi {
				continue
			}
			var text strings.Builder
			for k := range l.P.Runs {
				if r := &l.P.Runs[k]; r.Kind == RunText {
					text.WriteString(r.Text)
				}
			}
			if strings.TrimSpace(text.String()) == "" {
				continue
			}
			p := &Paragraph{Style: tocStyleID(&e.Doc.Styles, lv)}
			p.Props.Tabs = []TabStop{{Pos: width, Align: AlignRight, Leader: '.'}}
			p.Runs = []Run{{Text: text.String()}, {Kind: RunTab}, {Text: strconv.Itoa(pageOf(l.P))}}
			entries = append(entries, p)
		}
		if len(entries) == 0 {
			entries = []*Paragraph{{Runs: []Run{{Text: "No table of contents entries found."}}}}
		}
		entries[0].Runs = append(prefix, entries[0].Runs...)
		if !keepEnd {
			last := entries[len(entries)-1]
			last.Runs = append(last.Runs, suffix...)
			last.Sect = end.Sect
		}
		var blocks []Block
		for _, p := range entries {
			blocks = append(blocks, p)
		}
		to := el.Top + 1
		if keepEnd {
			to = el.Top
		}
		body := e.Doc.Body
		e.Doc.Body = append(append(append([]Block(nil), body[:bl.Top]...), blocks...), body[to:]...)
		e.reindex()
		p := Pos{toc.From.Para, 0}
		e.Sel = Selection{p, p}
	})
	return true
}
