package model

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Pos is a caret position: a paragraph, numbered in document order
// including the paragraphs inside table cells (see Document.Locate), and a
// rune offset into its text as Paragraph.Text returns it. Tabs, breaks and
// opaque runs (pictures, fields) count as one rune each, so they are a
// single caret step.
type Pos struct{ Para, Off int }

// Less reports whether p comes before q.
func (p Pos) Less(q Pos) bool {
	return p.Para < q.Para || p.Para == q.Para && p.Off < q.Off
}

// Selection is a range between an anchor (where it started) and the caret.
type Selection struct{ Anchor, Caret Pos }

// Empty reports whether nothing is selected.
func (s Selection) Empty() bool { return s.Anchor == s.Caret }

// Ordered returns the selection's start and end.
func (s Selection) Ordered() (Pos, Pos) {
	if s.Caret.Less(s.Anchor) {
		return s.Caret, s.Anchor
	}
	return s.Anchor, s.Caret
}

// ParaLoc says where a paragraph lives.
type ParaLoc struct {
	P      *Paragraph
	Blocks *[]Block   // its container: &Document.Body or &TableCell.Blocks
	Index  int        // index in *Blocks
	Top    int        // index of the enclosing block in Document.Body
	Cell   *TableCell // innermost cell, nil for body paragraphs
	// Hidden is set in a cell that continues a vertical merge (or one
	// inside such a cell): its text isn't shown, so the caret never goes
	// there.
	Hidden bool
}

// Locate lists every paragraph in document order with its location; the
// list index is Pos.Para.
func (d *Document) Locate() []ParaLoc {
	var out []ParaLoc
	var walkIn func(blocks *[]Block, top int, cell *TableCell, hidden bool)
	walkIn = func(blocks *[]Block, top int, cell *TableCell, hidden bool) {
		for i, b := range *blocks {
			if cell == nil {
				top = i
			}
			switch b := b.(type) {
			case *Paragraph:
				out = append(out, ParaLoc{P: b, Blocks: blocks, Index: i, Top: top, Cell: cell, Hidden: hidden})
			case *Table:
				for _, row := range b.Rows {
					for _, c := range row.Cells {
						if len(c.Blocks) == 0 {
							c.Blocks = []Block{&Paragraph{}}
						}
						walkIn(&c.Blocks, top, c, hidden || c.VMerge == 2)
					}
				}
			}
		}
	}
	if len(d.Body) == 0 {
		d.Body = []Block{&Paragraph{}}
	}
	walkIn(&d.Body, 0, nil, false)
	return out
}

// RunLen is the number of caret positions a run takes.
func RunLen(r *Run) int {
	if r.Kind == RunText {
		return utf8.RuneCountInString(r.Text)
	}
	return 1
}

// Len is the paragraph's length in caret positions.
func (p *Paragraph) Len() int {
	n := 0
	for i := range p.Runs {
		n += RunLen(&p.Runs[i])
	}
	return n
}

// RunAt returns the index of the run holding the character at off, or -1.
func (p *Paragraph) RunAt(off int) int {
	pos := 0
	for i := range p.Runs {
		n := RunLen(&p.Runs[i])
		if off < pos+n {
			return i
		}
		pos += n
	}
	return -1
}

// splitRuns makes off a run boundary and returns the index of the first
// run at or after it.
func splitRuns(p *Paragraph, off int) int {
	pos := 0
	for i := range p.Runs {
		if off <= pos {
			return i
		}
		n := RunLen(&p.Runs[i])
		if off < pos+n {
			r := p.Runs[i]
			rs := []rune(r.Text)
			a, b := r, r
			a.Text, b.Text = string(rs[:off-pos]), string(rs[off-pos:])
			p.Runs[i] = a
			p.Runs = slices.Insert(p.Runs, i+1, b)
			return i + 1
		}
		pos += n
	}
	return len(p.Runs)
}

// normalize joins neighboring text runs with the same formatting and drops
// empty ones.
func normalize(p *Paragraph) {
	out := p.Runs[:0]
	for _, r := range p.Runs {
		if r.Kind == RunText && r.Text == "" {
			continue
		}
		if n := len(out); n > 0 && r.Kind == RunText && out[n-1].Kind == RunText &&
			r.Opaque == nil && out[n-1].Opaque == nil && r.Span == out[n-1].Span && r.Style == out[n-1].Style && r.Props == out[n-1].Props {
			out[n-1].Text += r.Text
			continue
		}
		out = append(out, r)
	}
	clear(p.Runs[len(out):])
	p.Runs = out
}

// deleteRunes removes the characters [a, b) of p.
func deleteRunes(p *Paragraph, a, b int) {
	if a >= b {
		return
	}
	i := splitRuns(p, a)
	j := splitRuns(p, b)
	if i < len(p.Runs) && j-i == len(p.Runs) {
		// Emptied: the paragraph mark keeps the formatting for typing.
		p.Mark = p.Runs[i].Props
	}
	p.Runs = slices.Delete(p.Runs, i, j)
	normalize(p)
}

// insertRuns inserts runs at off.
func insertRuns(p *Paragraph, off int, runs ...Run) {
	i := splitRuns(p, off)
	p.Runs = slices.Insert(p.Runs, i, runs...)
	normalize(p)
}

// cutRuns returns copies of the runs covering [a, b).
func cutRuns(p *Paragraph, a, b int) []Run {
	var out []Run
	pos := 0
	for _, r := range p.Runs {
		n := RunLen(&r)
		lo, hi := max(a, pos), min(b, pos+n)
		if lo < hi {
			if r.Kind == RunText {
				rs := []rune(r.Text)
				r.Text = string(rs[lo-pos : hi-pos])
			}
			out = append(out, r)
		}
		pos += n
	}
	return out
}

// CopyBlock returns a deep copy of a paragraph or table (opaque payloads
// are shared; they are never modified).
func CopyBlock(b Block) Block {
	switch b := b.(type) {
	case *Paragraph:
		return copyPara(b)
	case *Table:
		t := *b
		t.Columns = slices.Clone(b.Columns)
		t.Rows = make([]*TableRow, len(b.Rows))
		for i, row := range b.Rows {
			r := *row
			r.Cells = make([]*TableCell, len(row.Cells))
			for j, c := range row.Cells {
				cc := *c
				cc.Blocks = CopyBlocks(c.Blocks)
				r.Cells[j] = &cc
			}
			t.Rows[i] = &r
		}
		return &t
	}
	return b
}

// CopyBlocks deep-copies a block list.
func CopyBlocks(bs []Block) []Block {
	out := make([]Block, len(bs))
	for i, b := range bs {
		out[i] = CopyBlock(b)
	}
	return out
}

func copyPara(p *Paragraph) *Paragraph {
	q := *p
	q.Runs = slices.Clone(p.Runs)
	q.Props = copyParaProps(p.Props)
	if p.Sect != nil {
		s := *p.Sect // edits replace its parts, never change them
		q.Sect = &s
	}
	return &q
}

func copyParaProps(pp ParaProps) ParaProps {
	pp.Tabs = slices.Clone(pp.Tabs)
	if pp.List != nil {
		l := *pp.List
		pp.List = &l
	}
	return pp
}

// CopyStyles deep-copies a style sheet.
func CopyStyles(s StyleSheet) StyleSheet {
	out := s
	out.DefaultPara = copyParaProps(s.DefaultPara)
	out.Styles = make([]*Style, len(s.Styles))
	for i, st := range s.Styles {
		c := *st
		c.Para = copyParaProps(st.Para)
		out.Styles[i] = &c
	}
	return out
}

func copyLists(m map[int]*List) map[int]*List {
	out := make(map[int]*List, len(m))
	for k, l := range m {
		c := *l
		c.Levels = slices.Clone(l.Levels)
		out[k] = &c
	}
	return out
}

// step is one undoable edit: the body blocks [start, start+afterLen) were
// before[...] before the edit (and swap back and forth on undo/redo).
type step struct {
	kind     string
	label    string // the name in the Undo list; see Op and Label
	typed    string // a typing step's text, for its label
	start    int
	before   []Block
	afterLen int
	sel      Selection // selection to restore
	styles   *StyleSheet
	lists    map[int]*List
	hf       *HeadersFooters // the headers and footers, for steps that change them
	sect     *Section        // the last section, for steps that change it
}

// Editor edits a document: selection, text and structure changes,
// formatting and styles, with undo and redo.
type Editor struct {
	Doc *Document
	Sel Selection
	// Pending is formatting chosen with nothing selected (Ctrl+B before
	// typing); it applies to the next typed text and is dropped when the
	// caret moves.
	Pending *Run
	// Version increases with every change to the document.
	Version int

	locs  []ParaLoc
	undo  []*step
	redo  []*step
	typed *step  // the typing step that further typing extends
	label string // names the steps made now; see Op
	dirty map[*Paragraph]bool
	all   bool
}

// NewEditor returns an editor with the caret at the start of doc.
func NewEditor(doc *Document) *Editor {
	if doc.Lists == nil {
		doc.Lists = map[int]*List{}
	}
	e := &Editor{Doc: doc, dirty: map[*Paragraph]bool{}, all: true}
	e.reindex()
	return e
}

func (e *Editor) reindex() { e.locs = e.Doc.Locate() }

// Locs returns the paragraph locations (index = Pos.Para). The slice is
// replaced after every structural change.
func (e *Editor) Locs() []ParaLoc { return e.locs }

// Para returns paragraph i.
func (e *Editor) Para(i int) *Paragraph { return e.locs[i].P }

// NumParas is the number of paragraphs, table cells included.
func (e *Editor) NumParas() int { return len(e.locs) }

// TakeDirty returns the paragraphs changed since the last call, or all ==
// true when everything must be laid out again (styles or lists changed).
func (e *Editor) TakeDirty() (paras []*Paragraph, all bool) {
	for p := range e.dirty {
		paras = append(paras, p)
	}
	all = e.all
	clear(e.dirty)
	e.all = false
	return paras, all
}

// CanUndo and CanRedo report whether there is something to undo/redo.
func (e *Editor) CanUndo() bool { return len(e.undo) > 0 }
func (e *Editor) CanRedo() bool { return len(e.redo) > 0 }

// clamp keeps p inside the document and out of hidden paragraphs (see
// ParaLoc.Hidden): such a position moves to the next shown paragraph,
// or the end of the previous one.
func (e *Editor) clamp(p Pos) Pos {
	p.Para = min(max(p.Para, 0), len(e.locs)-1)
	if e.locs[p.Para].Hidden {
		if i := e.shown(p.Para, 1); i >= 0 {
			p = Pos{i, 0}
		} else if i := e.shown(p.Para, -1); i >= 0 {
			p = Pos{i, e.locs[i].P.Len()}
		}
	}
	p.Off = min(max(p.Off, 0), e.locs[p.Para].P.Len())
	return p
}

// shown returns the first paragraph from i in direction dir (±1) that
// isn't hidden, or -1.
func (e *Editor) shown(i, dir int) int {
	for ; i >= 0 && i < len(e.locs); i += dir {
		if !e.locs[i].Hidden {
			return i
		}
	}
	return -1
}

// edit runs f as one undoable step touching body blocks [from, to] (none
// when to < from; docLevel steps also save the styles and lists). Typing
// steps extend the previous typing step while nothing else happens in
// between, as Word groups typed words.
func (e *Editor) edit(kind string, from, to int, docLevel bool, f func()) {
	body := &e.Doc.Body
	oldLen := len(*body)
	sel := e.Sel
	if kind == "typing" && e.typed != nil && len(e.undo) > 0 && e.undo[len(e.undo)-1] == e.typed &&
		from >= e.typed.start && to < e.typed.start+e.typed.afterLen && !docLevel {
		f()
		e.typed.afterLen += len(*body) - oldLen
		e.finish(e.typed.start, e.typed.afterLen)
		return
	}
	s := &step{kind: kind, label: e.label, start: from, before: CopyBlocks((*body)[from : to+1]), sel: sel}
	if docLevel {
		st := CopyStyles(e.Doc.Styles)
		s.styles, s.lists = &st, copyLists(e.Doc.Lists)
		e.all = true
	}
	f()
	s.afterLen = to + 1 - from + len(*body) - oldLen
	e.undo = append(e.undo, s)
	e.redo = nil
	e.typed = nil
	if kind == "typing" {
		e.typed = s
	}
	e.finish(from, s.afterLen)
}

func (e *Editor) finish(from, n int) {
	if len(e.Doc.Body) == 0 {
		e.Doc.Body = []Block{&Paragraph{}}
	}
	e.reindex()
	WalkBlocks(e.Doc.Body[from:min(from+n, len(e.Doc.Body))], func(p *Paragraph) bool {
		e.dirty[p] = true
		return true
	})
	e.Sel.Anchor, e.Sel.Caret = e.clamp(e.Sel.Anchor), e.clamp(e.Sel.Caret)
	e.Version++
}

// Undo reverts the last step.
func (e *Editor) Undo() bool { return e.swap(&e.undo, &e.redo) }

// Redo repeats the last undone step.
func (e *Editor) Redo() bool { return e.swap(&e.redo, &e.undo) }

// UndoN reverts the last n steps (fewer if there aren't as many), as
// choosing an entry of Word's Undo list does; RedoN repeats n undone
// steps. They report whether anything changed.
func (e *Editor) UndoN(n int) bool { return e.swapN(&e.undo, &e.redo, n) }
func (e *Editor) RedoN(n int) bool { return e.swapN(&e.redo, &e.undo, n) }

func (e *Editor) swapN(from, to *[]*step, n int) bool {
	did := false
	for ; n > 0 && e.swap(from, to); n-- {
		did = true
	}
	return did
}

// UndoLabels and RedoLabels name the steps Undo and Redo would revert or
// repeat, the next one first, for the Undo and Redo lists.
func (e *Editor) UndoLabels() []string { return stepLabels(e.undo) }
func (e *Editor) RedoLabels() []string { return stepLabels(e.redo) }

func stepLabels(steps []*step) []string {
	out := make([]string, len(steps))
	for i, s := range steps {
		out[len(steps)-1-i] = s.Label()
	}
	return out
}

// Op names the steps made until the returned function runs, for the
// Undo list ("Bold"): a caller that knows the command better than the
// method it calls names it first, and the outermost name wins.
//
//	defer e.Op("Bold")()
func (e *Editor) Op(label string) func() {
	if e.label != "" {
		return func() {}
	}
	e.label = label
	return func() { e.label = "" }
}

// either is yes when c is set, else no.
func either(c bool, yes, no string) string {
	if c {
		return yes
	}
	return no
}

// kindLabels name steps no command named, by kind.
var kindLabels = map[string]string{
	"typing": "Typing", "delete": "Delete", "enter": "Typing", "format": "Formatting", "insert": "Insert",
	"link": "Hyperlink", "paste": "Paste", "replace": "Replace", "style": "Style",
	"table": "Table", "move": "Move Text", "copy": "Copy Text",
}

// Label is the step's name in the Undo list: typed text shows as Word
// shows it ("Typing 'hello'").
func (s *step) Label() string {
	switch {
	case s.label != "":
		return s.label
	case s.typed != "":
		t := strings.Join(strings.Fields(s.typed), " ")
		if r := []rune(t); len(r) > 20 {
			t = string(r[:20]) + "…"
		}
		return "Typing '" + t + "'"
	case kindLabels[s.kind] != "":
		return kindLabels[s.kind]
	}
	return "Edit"
}

func (e *Editor) swap(from, to *[]*step) bool {
	if len(*from) == 0 {
		return false
	}
	s := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	body := e.Doc.Body
	cur := CopyBlocks(body[s.start : s.start+s.afterLen])
	e.Doc.Body = slices.Concat(body[:s.start], s.before, body[s.start+s.afterLen:])
	s.before, s.afterLen = cur, len(s.before)
	if s.styles != nil {
		st, ls := e.Doc.Styles, e.Doc.Lists
		e.Doc.Styles, e.Doc.Lists = *s.styles, s.lists
		s.styles, s.lists = &st, ls
		e.all = true
	}
	if s.hf != nil {
		hf := e.Doc.HF
		e.Doc.HF, *s.hf = *s.hf, hf
		e.all = true
	}
	if s.sect != nil {
		sect := e.Doc.Section
		e.Doc.Section, *s.sect = *s.sect, sect
		e.all = true
	}
	sel := e.Sel
	e.Sel, s.sel = s.sel, sel
	*to = append(*to, s)
	e.typed = nil
	e.Pending = nil
	e.finish(s.start, s.afterLen)
	return true
}

// Caret movement.

// SetCaret moves the caret, extending the selection if extend is set.
func (e *Editor) SetCaret(p Pos, extend bool) {
	p = e.clamp(p)
	e.Sel.Caret = p
	if !extend {
		e.Sel.Anchor = p
	}
	e.Pending = nil
	e.typed = nil
}

// Select sets the selection.
func (e *Editor) Select(anchor, caret Pos) {
	e.Sel = Selection{e.clamp(anchor), e.clamp(caret)}
	e.Pending = nil
	e.typed = nil
}

// SelectAll selects the whole document.
func (e *Editor) SelectAll() {
	last := len(e.locs) - 1
	e.Select(Pos{}, Pos{last, e.locs[last].P.Len()})
}

// invisible reports whether the character at off is an invisible marker
// (an opaque run without text: bookmarks, comment ranges, field codes),
// which the caret steps over.
func invisible(p *Paragraph, off int) bool {
	i := p.RunAt(off)
	return i >= 0 && p.Runs[i].Kind == RunOpaque && p.Runs[i].Text == ""
}

// Next returns the caret position one step right of p.
func (e *Editor) Next(p Pos) Pos {
	para := e.locs[p.Para].P
	if n := para.Len(); p.Off < n {
		off := p.Off + 1
		for off < n && invisible(para, off-1) {
			off++
		}
		return Pos{p.Para, off}
	}
	if i := e.shown(p.Para+1, 1); i >= 0 {
		return Pos{i, 0}
	}
	return p
}

// Prev returns the caret position one step left of p.
func (e *Editor) Prev(p Pos) Pos {
	if p.Off > 0 {
		para := e.locs[p.Para].P
		off := p.Off - 1
		for off > 0 && invisible(para, off) {
			off--
		}
		return Pos{p.Para, off}
	}
	if i := e.shown(p.Para-1, -1); i >= 0 {
		return Pos{i, e.locs[i].P.Len()}
	}
	return p
}

// Move moves the caret one step (dir -1 or +1), or one word with word
// set. Without extend, a selection collapses to its edge first.
func (e *Editor) Move(dir int, word, extend bool) {
	if !extend && !e.Sel.Empty() && !word {
		a, b := e.Sel.Ordered()
		if dir < 0 {
			e.SetCaret(a, false)
		} else {
			e.SetCaret(b, false)
		}
		return
	}
	p := e.Sel.Caret
	switch {
	case word && dir < 0:
		p = e.WordLeft(p)
	case word:
		p = e.WordRight(p)
	case dir < 0:
		p = e.Prev(p)
	default:
		p = e.Next(p)
	}
	e.SetCaret(p, extend)
}

// charClass groups characters for word movement: 0 space, 1 word
// characters, 2 punctuation, 3 tabs/breaks/objects.
func charClass(r rune) int {
	switch {
	case r == '\t' || r == '\n' || r == '\f' || r == '￼':
		return 3
	case unicode.IsSpace(r):
		return 0
	case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '\'' || r == '’':
		return 1
	}
	return 2
}

// WordRight returns the start of the next word (Ctrl+Right).
func (e *Editor) WordRight(p Pos) Pos {
	rs := []rune(e.locs[p.Para].P.Text())
	if p.Off >= len(rs) {
		return e.Next(p)
	}
	i := p.Off
	c := charClass(rs[i])
	if c != 0 {
		for i < len(rs) && charClass(rs[i]) == c && c != 3 {
			i++
		}
		if c == 3 {
			i++
		}
	}
	for i < len(rs) && charClass(rs[i]) == 0 {
		i++
	}
	return Pos{p.Para, i}
}

// WordLeft returns the start of the word before p (Ctrl+Left).
func (e *Editor) WordLeft(p Pos) Pos {
	if p.Off == 0 {
		return e.Prev(p)
	}
	rs := []rune(e.locs[p.Para].P.Text())
	i := p.Off
	for i > 0 && charClass(rs[i-1]) == 0 {
		i--
	}
	if i > 0 {
		c := charClass(rs[i-1])
		if c == 3 {
			return Pos{p.Para, i - 1}
		}
		for i > 0 && charClass(rs[i-1]) == c {
			i--
		}
	}
	return Pos{p.Para, i}
}

// WordAt returns the word (with its trailing spaces, as Word selects on a
// double-click) around p.
func (e *Editor) WordAt(p Pos) (Pos, Pos) {
	rs := []rune(e.locs[p.Para].P.Text())
	if len(rs) == 0 {
		return p, p
	}
	i := min(p.Off, len(rs)-1)
	c := charClass(rs[i])
	if c == 0 && i > 0 && charClass(rs[i-1]) != 0 {
		i--
		c = charClass(rs[i])
	}
	a, b := i, i+1
	if c != 3 {
		for a > 0 && charClass(rs[a-1]) == c {
			a--
		}
		for b < len(rs) && charClass(rs[b]) == c {
			b++
		}
	}
	for b < len(rs) && charClass(rs[b]) == 0 && c != 0 {
		b++
	}
	return Pos{p.Para, a}, Pos{p.Para, b}
}

// SelectWord selects the word at p (double-click).
func (e *Editor) SelectWord(p Pos) {
	a, b := e.WordAt(e.clamp(p))
	e.Select(a, b)
}

// SelectParagraph selects paragraph i (triple-click).
func (e *Editor) SelectParagraph(i int) {
	p := e.clamp(Pos{Para: i})
	e.Select(p, Pos{p.Para, e.locs[p.Para].P.Len()})
}

// Text editing.

// caretRun is the formatting typed text gets at p: that of the character
// before it, or the first character of an empty-start paragraph, or the
// paragraph mark.
func (e *Editor) caretRun(p Pos) Run {
	if e.Pending != nil {
		r := *e.Pending
		r.Kind, r.Text, r.Opaque, r.Span = RunText, "", nil, nil
		return r
	}
	para := e.locs[p.Para].P
	i := -1
	if p.Off > 0 {
		i = para.RunAt(p.Off - 1)
	} else if len(para.Runs) > 0 {
		i = 0
	}
	if i < 0 {
		return Run{Props: para.Mark}
	}
	r := para.Runs[i]
	out := Run{Style: r.Style, Props: r.Props}
	// Inside a hyperlink or other span, typed text joins it.
	if r.Span != nil && p.Off > 0 {
		if j := para.RunAt(p.Off); j >= 0 && para.Runs[j].Span == r.Span {
			out.Span = r.Span
		}
	}
	if out.Span == nil && linkOf(r.Span) != nil {
		out.Style = e.unlinkedStyle(r.Style) // text typed after a link isn't link-styled
	}
	return out
}

// CaretFormat returns the resolved formatting at the caret (or the start
// of the selection), for the toolbar.
func (e *Editor) CaretFormat() RunProps {
	a, _ := e.Sel.Ordered()
	if !e.Sel.Empty() && a.Off < e.locs[a.Para].P.Len() {
		a.Off++ // the first selected character
	}
	r := e.caretRun(a)
	return e.Doc.ResolveRun(e.locs[a.Para].P, &r)
}

// span returns the top-level block range touched by the selection.
func (e *Editor) span() (int, int) {
	a, b := e.Sel.Ordered()
	return e.locs[a.Para].Top, e.locs[b.Para].Top
}

// InsertText types s at the caret, replacing the selection. "\n" starts a
// new paragraph and "\t" inserts a tab.
func (e *Editor) InsertText(s string) {
	if s == "" {
		return
	}
	kind := "typing"
	if !e.Sel.Empty() || strings.ContainsAny(s, "\n\t") || utf8.RuneCountInString(s) > 1 {
		kind = "insert"
	}
	from, to := e.span()
	e.edit(kind, from, to, false, func() {
		fmtRun := e.caretRun(e.Sel.Caret)
		if !e.Sel.Empty() {
			fmtRun = e.caretRun(e.selStartPlusOne())
		}
		p := e.deleteSelection()
		for i, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
			if i > 0 {
				p = e.splitPara(p)
			}
			var runs []Run
			for j, part := range strings.Split(line, "\t") {
				if j > 0 {
					r := fmtRun
					r.Kind = RunTab
					runs = append(runs, r)
				}
				if part != "" {
					r := fmtRun
					r.Text = part
					runs = append(runs, r)
				}
			}
			n := 0
			for k := range runs {
				n += RunLen(&runs[k])
			}
			insertRuns(e.locs[p.Para].P, p.Off, runs...)
			p.Off += n
		}
		e.Sel = Selection{p, p}
	})
	if n := len(e.undo); n > 0 {
		e.undo[n-1].typed += s // what the step's label shows
	}
	if kind == "typing" {
		// Pending formatting now lives in the typed run.
		e.Pending = nil
	}
}

// selStartPlusOne is the position just inside the start of the selection,
// whose character's formatting replaced text takes.
func (e *Editor) selStartPlusOne() Pos {
	a, _ := e.Sel.Ordered()
	if a.Off < e.locs[a.Para].P.Len() {
		a.Off++
	}
	return a
}

// InsertSpecial inserts a tab, line break or page break at the caret.
func (e *Editor) InsertSpecial(kind RunKind) {
	from, to := e.span()
	e.edit("insert", from, to, false, func() {
		r := e.caretRun(e.Sel.Caret)
		p := e.deleteSelection()
		r.Kind = kind
		insertRuns(e.locs[p.Para].P, p.Off, r)
		p.Off++
		e.Sel = Selection{p, p}
	})
}

// Enter splits the paragraph at the caret (or, in an empty list
// paragraph, ends the list as Word does).
func (e *Editor) Enter() {
	from, to := e.span()
	e.edit("enter", from, to, false, func() {
		p := e.deleteSelection()
		para := e.locs[p.Para].P
		if para.Len() == 0 && e.inList(para) {
			e.setList(para, nil)
			e.Sel = Selection{p, p}
			return
		}
		p = e.splitPara(p)
		e.Sel = Selection{p, p}
	})
}

func (e *Editor) inList(p *Paragraph) bool {
	l := e.Doc.ResolvePara(p).List
	return l != nil && l.NumID != 0 && e.Doc.Lists[l.NumID] != nil
}

// setList puts p in a list, or takes it out (ref nil) even when its style
// has one.
func (e *Editor) setList(p *Paragraph, ref *ListRef) {
	p.Props.List = nil
	if ref == nil {
		if e.inList(p) {
			p.Props.List = &ListRef{}
		}
		return
	}
	l := *ref
	p.Props.List = &l
}

// splitPara splits the paragraph at p and returns the start of the new
// one. At the end of a paragraph the new one takes the style's Next style.
func (e *Editor) splitPara(p Pos) Pos {
	loc := e.locs[p.Para]
	para := loc.P
	q := &Paragraph{Style: para.Style, Props: copyParaProps(para.Props), Mark: para.Mark, Wrap: para.Wrap}
	if p.Off >= para.Len() {
		if st := e.Doc.Styles.Find(styleOrDefault(e.Doc, para.Style)); st != nil && st.Next != "" && st.Next != st.ID {
			q.Style, q.Props = st.Next, ParaProps{}
			if st.Next == normalID(e.Doc) {
				q.Style = ""
			}
		}
		if len(para.Runs) > 0 {
			q.Mark = para.Runs[len(para.Runs)-1].Props
		}
	}
	q.Props.PageBreakBefore = Unset
	q.Sect, para.Sect = para.Sect, nil // the section break is the mark, which q takes
	i := splitRuns(para, p.Off)
	q.Runs = slices.Clone(para.Runs[i:])
	para.Runs = para.Runs[:i]
	if len(para.Runs) == 0 && len(q.Runs) > 0 {
		para.Mark = q.Runs[0].Props
	}
	*loc.Blocks = slices.Insert(*loc.Blocks, loc.Index+1, Block(q))
	e.reindex()
	return Pos{p.Para + 1, 0}
}

func styleOrDefault(d *Document, id string) string {
	if id != "" {
		return id
	}
	return normalID(d)
}

func normalID(d *Document) string {
	if st := d.Styles.Default(ParagraphStyle); st != nil {
		return st.ID
	}
	return "Normal"
}

// Backspace deletes the selection or the character before the caret; at
// the start of a paragraph it ends a list or joins the paragraph to the
// previous one.
func (e *Editor) Backspace() {
	if !e.Sel.Empty() {
		e.DeleteSelection()
		return
	}
	p := e.Sel.Caret
	loc := e.locs[p.Para]
	k := p.Off - 1
	for k >= 0 && invisible(loc.P, k) {
		k-- // markers stay; the visible character before them goes
	}
	if k >= 0 {
		e.edit("typing", loc.Top, loc.Top, false, func() {
			deleteRunes(loc.P, k, k+1)
			q := Pos{p.Para, k}
			e.Sel = Selection{q, q}
		})
		if s := e.typed; s != nil {
			_, n := utf8.DecodeLastRuneInString(s.typed)
			s.typed = s.typed[:len(s.typed)-n] // a typo taken back
		}
		return
	}
	if e.inList(loc.P) {
		e.edit("format", loc.Top, loc.Top, false, func() { e.setList(loc.P, nil) })
		return
	}
	e.joinPrev(p.Para)
}

// Delete deletes the selection or the character after the caret; at the
// end of a paragraph it joins the next paragraph.
func (e *Editor) Delete() {
	if !e.Sel.Empty() {
		e.DeleteSelection()
		return
	}
	p := e.Sel.Caret
	loc := e.locs[p.Para]
	k := p.Off
	for k < loc.P.Len() && invisible(loc.P, k) {
		k++
	}
	if k < loc.P.Len() {
		e.edit("delete", loc.Top, loc.Top, false, func() { deleteRunes(loc.P, k, k+1) })
		return
	}
	// Only a following paragraph in the same container joins: at the end
	// of a cell, or before or after a table, Delete does nothing.
	if p.Para+1 < len(e.locs) && e.locs[p.Para+1].Blocks == loc.Blocks {
		e.joinPrev(p.Para + 1)
		e.Sel = Selection{p, p}
	}
}

// joinPrev merges paragraph i into the one before it in the same
// container. After a table an empty paragraph is removed instead, unless
// it is the last block of its container (a document or cell always ends
// in a paragraph), when the caret just moves into the table; at the start
// of a cell nothing happens.
func (e *Editor) joinPrev(i int) {
	if i <= 0 {
		return
	}
	loc := e.locs[i]
	if loc.Index == 0 {
		return
	}
	prevLoc := e.locs[i-1]
	if _, ok := (*loc.Blocks)[loc.Index-1].(*Paragraph); !ok {
		if loc.P.Len() > 0 || loc.Index == len(*loc.Blocks)-1 {
			e.SetCaret(e.Prev(Pos{Para: i}), false)
			return
		}
		from := loc.Top - 1 // the table and the paragraph
		if loc.Cell != nil {
			from = loc.Top // the body table holding both
		}
		e.edit("delete", from, loc.Top, false, func() {
			*loc.Blocks = slices.Delete(*loc.Blocks, loc.Index, loc.Index+1)
			e.reindex()
			q := Pos{i - 1, e.locs[i-1].P.Len()}
			e.Sel = Selection{q, q}
		})
		return
	}
	from := e.locs[i-1].Top
	e.edit("delete", from, loc.Top, false, func() {
		q := Pos{i - 1, prevLoc.P.Len()}
		e.join(prevLoc, loc)
		e.reindex()
		e.Sel = Selection{q, q}
	})
}

// join appends paragraph b to a (its predecessor in the same container)
// and removes b. An empty a takes b's formatting.
func (e *Editor) join(a, b ParaLoc) {
	if a.P.Len() == 0 {
		a.P.Style, a.P.Props, a.P.Mark = b.P.Style, b.P.Props, b.P.Mark
	}
	a.P.Runs = append(a.P.Runs, b.P.Runs...)
	a.P.Before = append(a.P.Before, b.P.Before...)
	// b's mark survives: a section break at a's end goes (the sections
	// merge, the following one's properties winning) and b's stays.
	a.P.Sect = b.P.Sect
	normalize(a.P)
	*b.Blocks = slices.Delete(*b.Blocks, b.Index, b.Index+1)
}

// DeleteSelection deletes the selected content.
func (e *Editor) DeleteSelection() {
	if e.Sel.Empty() {
		return
	}
	from, to := e.span()
	e.edit("delete", from, to, false, func() {
		p := e.deleteSelection()
		e.Sel = Selection{p, p}
	})
}

// deleteSelection removes the selection's content and returns where it
// started. Within one container paragraphs are joined; across table cells
// the covered text is cleared and the table structure kept, as in Word.
func (e *Editor) deleteSelection() Pos {
	s, t := e.Sel.Ordered()
	if s == t {
		return s
	}
	// Split the range into runs of consecutive paragraphs that share a
	// container (the body, or one table cell).
	type seg struct{ a, b Pos }
	var segs []seg
	start := s
	for i := s.Para; i <= t.Para; i++ {
		if i == t.Para {
			segs = append(segs, seg{start, t})
			break
		}
		cur, next := e.locs[i], e.locs[i+1]
		if next.Blocks == cur.Blocks && next.Index == cur.Index+1 {
			continue
		}
		segs = append(segs, seg{start, Pos{i, cur.P.Len()}})
		start = Pos{i + 1, 0}
	}
	// Body paragraphs around whole tables share a container: delete from s
	// to t directly so the tables in between go too.
	if e.locs[s.Para].Blocks == e.locs[t.Para].Blocks {
		segs = []seg{{s, t}}
	}
	for k := len(segs) - 1; k >= 0; k-- {
		e.deleteInContainer(segs[k].a, segs[k].b)
	}
	return s
}

// deleteInContainer deletes [a, b) where both are in the same container.
func (e *Editor) deleteInContainer(a, b Pos) {
	la, lb := e.locs[a.Para], e.locs[b.Para]
	if a.Para == b.Para {
		deleteRunes(la.P, a.Off, b.Off)
		return
	}
	deleteRunes(la.P, a.Off, la.P.Len())
	deleteRunes(lb.P, 0, b.Off)
	blocks := la.Blocks
	*blocks = slices.Delete(*blocks, la.Index+1, lb.Index)
	e.reindex()
	// With the blocks in between gone, b's paragraph follows a's.
	e.join(e.locs[a.Para], e.locs[a.Para+1])
	e.reindex()
}

// Clipboard.

// Fragment is copied content: whole or partial paragraphs (tables are
// flattened to their cell paragraphs).
type Fragment struct {
	Paras []*Paragraph
}

// Copy returns the selection as a fragment.
func (e *Editor) Copy() *Fragment {
	s, t := e.Sel.Ordered()
	f := &Fragment{}
	for i := s.Para; i <= t.Para; i++ {
		p := e.locs[i].P
		a, b := 0, p.Len()
		if i == s.Para {
			a = s.Off
		}
		if i == t.Para {
			b = t.Off
		}
		q := &Paragraph{Style: p.Style, Props: copyParaProps(p.Props), Mark: p.Mark, Runs: cutRuns(p, a, b)}
		f.Paras = append(f.Paras, q)
	}
	return f
}

// Text is the fragment's plain text, paragraphs separated by CRLF.
func (f *Fragment) Text() string {
	var parts []string
	for _, p := range f.Paras {
		parts = append(parts, strings.NewReplacer("\n", "\r\n", "\f", "\r\n", "￼", "").Replace(p.Text()))
	}
	return strings.Join(parts, "\r\n")
}

// Paste inserts a fragment at the selection, keeping its formatting.
func (e *Editor) Paste(f *Fragment) {
	defer e.Op("Paste")()
	if f == nil || len(f.Paras) == 0 {
		return
	}
	from, to := e.span()
	e.edit("paste", from, to, false, func() {
		p := e.pasteAt(e.deleteSelection(), f)
		e.Sel = Selection{p, p}
	})
}

// pasteAt inserts a fragment at p and returns the end of what it
// inserted.
func (e *Editor) pasteAt(p Pos, f *Fragment) Pos {
	for i, src := range f.Paras {
		if i > 0 {
			p = e.splitPara(p)
			dst := e.locs[p.Para].P
			dst.Style, dst.Props = src.Style, copyParaProps(src.Props)
		}
		dst := e.locs[p.Para].P
		if i == 0 && len(f.Paras) > 1 && p.Off == 0 && dst.Len() == 0 {
			dst.Style, dst.Props = src.Style, copyParaProps(src.Props)
		}
		runs := slices.Clone(src.Runs)
		insertRuns(dst, p.Off, runs...)
		for k := range runs {
			p.Off += RunLen(&runs[k])
		}
	}
	return p
}

// PasteText inserts plain text with the caret's formatting; lines become
// paragraphs.
func (e *Editor) PasteText(s string) {
	defer e.Op("Paste")()
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	if s == "" {
		return
	}
	e.InsertText(s)
	e.typed = nil
}

// Formatting.

// SelectedParas returns the indexes of the paragraphs the selection
// touches.
func (e *Editor) SelectedParas() []int {
	s, t := e.Sel.Ordered()
	var out []int
	for i := s.Para; i <= t.Para; i++ {
		if i == t.Para && t.Off == 0 && i > s.Para {
			break // a selection ending at a paragraph start excludes it
		}
		out = append(out, i)
	}
	return out
}

// eachSelectedRun calls f on every run inside the selection, after
// splitting runs at its edges.
func (e *Editor) eachSelectedRun(f func(p *Paragraph, r *Run)) {
	s, t := e.Sel.Ordered()
	for i := s.Para; i <= t.Para; i++ {
		p := e.locs[i].P
		a, b := 0, p.Len()
		if i == s.Para {
			a = s.Off
		}
		if i == t.Para {
			b = t.Off
		}
		if a >= b {
			continue
		}
		ri := splitRuns(p, a)
		rj := splitRuns(p, b)
		for k := ri; k < rj; k++ {
			f(p, &p.Runs[k])
		}
	}
}

// AllSelected reports whether pred holds for the resolved formatting of
// every selected character (or of the caret with nothing selected).
func (e *Editor) AllSelected(pred func(RunProps) bool) bool {
	if e.Sel.Empty() {
		return pred(e.CaretFormat())
	}
	all := true
	s, t := e.Sel.Ordered()
	for i := s.Para; i <= t.Para && all; i++ {
		p := e.locs[i].P
		a, b := 0, p.Len()
		if i == s.Para {
			a = s.Off
		}
		if i == t.Para {
			b = t.Off
		}
		pos := 0
		for k := range p.Runs {
			n := RunLen(&p.Runs[k])
			if pos < b && pos+n > a && !pred(e.Doc.ResolveRun(p, &p.Runs[k])) {
				all = false
				break
			}
			pos += n
		}
	}
	return all
}

// FormatRuns applies f to the selected text. With nothing selected it
// applies to the word around the caret, or else to the next typed text.
func (e *Editor) FormatRuns(f func(r *Run)) {
	if e.Sel.Empty() {
		c := e.Sel.Caret
		a, b := e.WordAt(c)
		rs := []rune(e.locs[c.Para].P.Text())
		inWord := c.Off > 0 && c.Off < len(rs) && charClass(rs[c.Off-1]) == 1 && charClass(rs[c.Off]) == 1
		if !inWord {
			r := e.caretRun(c)
			f(&r)
			e.Pending = &r
			return
		}
		// Trailing spaces stay as they were.
		for b.Off > a.Off && charClass(rs[b.Off-1]) == 0 {
			b.Off--
		}
		sel := e.Sel
		e.Sel = Selection{a, b}
		e.FormatRuns(f)
		e.Sel = sel
		return
	}
	from, to := e.span()
	e.edit("format", from, to, false, func() {
		touched := map[*Paragraph]bool{}
		e.eachSelectedRun(func(p *Paragraph, r *Run) {
			f(r)
			touched[p] = true
		})
		for p := range touched {
			normalize(p)
			if p.Len() == 0 {
				r := Run{Props: p.Mark}
				f(&r)
				p.Mark = r.Props
			}
		}
		// Empty selected paragraphs: format their marks.
		for _, i := range e.SelectedParas() {
			if p := e.locs[i].P; p.Len() == 0 {
				r := Run{Props: p.Mark}
				f(&r)
				p.Mark = r.Props
			}
		}
	})
}

// ToggleBold, ToggleItalic, ToggleUnderline and ToggleStrike switch the
// property off when the whole selection has it, otherwise on (Ctrl+B/I/U).
func (e *Editor) ToggleBold() {
	defer e.Op("Bold")()
	on := !e.AllSelected(func(p RunProps) bool { return p.Bold.Bool(false) })
	e.FormatRuns(func(r *Run) { r.Props.Bold = TriOf(on) })
}

func (e *Editor) ToggleItalic() {
	defer e.Op("Italic")()
	on := !e.AllSelected(func(p RunProps) bool { return p.Italic.Bool(false) })
	e.FormatRuns(func(r *Run) { r.Props.Italic = TriOf(on) })
}

func (e *Editor) ToggleUnderline() {
	defer e.Op("Underline")()
	on := !e.AllSelected(func(p RunProps) bool { return hasUnderline(p.Underline) })
	e.FormatRuns(func(r *Run) {
		r.Props.Underline = UnderlineNone
		if on {
			r.Props.Underline = UnderlineSingle
		}
	})
}

func hasUnderline(u Underline) bool { return u != UnderlineUnset && u != UnderlineNone }

func (e *Editor) ToggleStrike() {
	defer e.Op("Strikethrough")()
	on := !e.AllSelected(func(p RunProps) bool { return p.Strike.Bool(false) })
	e.FormatRuns(func(r *Run) { r.Props.Strike = TriOf(on) })
}

// SetFont, SetSize and SetColor set character properties of the selection.
func (e *Editor) SetFont(name string) {
	defer e.Op("Font")()
	e.FormatRuns(func(r *Run) { r.Props.Font = name })
}

func (e *Editor) SetSize(pt float64) {
	defer e.Op("Font Size")()
	e.FormatRuns(func(r *Run) { r.Props.Size = pt })
}

func (e *Editor) SetColor(hex string) {
	defer e.Op("Font Color")()
	e.FormatRuns(func(r *Run) { r.Props.Color = hex })
}

// SetCharStyle applies a character style ("" removes it).
func (e *Editor) SetCharStyle(id string) {
	defer e.Op("Style")()
	e.FormatRuns(func(r *Run) { r.Style = id })
}

// ClearCharFormatting removes direct character formatting and character
// styles (Ctrl+Space).
func (e *Editor) ClearCharFormatting() {
	defer e.Op("Clear Formatting")()
	e.FormatRuns(func(r *Run) { r.Props, r.Style = RunProps{}, "" })
}

// FormatParas applies f to every selected paragraph.
func (e *Editor) FormatParas(f func(p *Paragraph)) {
	defer e.Op("Paragraph Formatting")()
	e.formatParas(false, f)
}

func (e *Editor) formatParas(docLevel bool, f func(p *Paragraph)) {
	from, to := e.span()
	e.edit("format", from, to, docLevel, func() {
		for _, i := range e.SelectedParas() {
			f(e.locs[i].P)
		}
	})
}

// SetParaStyle applies a paragraph style to the selected paragraphs,
// replacing their direct paragraph formatting (list membership stays).
func (e *Editor) SetParaStyle(id string) {
	defer e.Op("Style")()
	if id == normalID(e.Doc) {
		id = ""
	}
	e.FormatParas(func(p *Paragraph) {
		list := p.Props.List
		p.Style = id
		p.Props = ParaProps{List: list}
	})
}

// ApplyStyle applies a paragraph or character style by ID.
func (e *Editor) ApplyStyle(id string) {
	defer e.Op("Style")()
	st := e.Doc.Styles.Find(id)
	if st != nil && st.Type == CharacterStyle {
		if st.Default {
			id = ""
		}
		e.SetCharStyle(id)
		return
	}
	e.SetParaStyle(id)
}

// ClearAllFormatting resets the selection to Normal with no direct
// formatting (Home > Clear All Formatting).
func (e *Editor) ClearAllFormatting() {
	defer e.Op("Clear Formatting")()
	from, to := e.span()
	e.edit("format", from, to, false, func() {
		e.eachSelectedRun(func(_ *Paragraph, r *Run) { r.Props, r.Style = RunProps{}, "" })
		for _, i := range e.SelectedParas() {
			p := e.locs[i].P
			p.Style, p.Props, p.Mark = "", ParaProps{}, RunProps{}
			normalize(p)
		}
	})
}

// SetAlign aligns the selected paragraphs.
func (e *Editor) SetAlign(a Align) {
	defer e.Op("Paragraph Formatting")()
	e.FormatParas(func(p *Paragraph) { p.Props.Align = a })
}

// SetLineSpacing sets "multiple" line spacing (1, 1.15, 1.5, 2...).
func (e *Editor) SetLineSpacing(mult float64) {
	defer e.Op("Line Spacing")()
	e.FormatParas(func(p *Paragraph) { p.Props.Line, p.Props.LineRule = mult, LineAuto })
}

// SetSpacing sets space before and/or after (nil leaves it).
func (e *Editor) SetSpacing(before, after *float64) {
	defer e.Op("Paragraph Spacing")()
	e.FormatParas(func(p *Paragraph) {
		if before != nil {
			p.Props.SpaceBefore = F(*before)
		}
		if after != nil {
			p.Props.SpaceAfter = F(*after)
		}
	})
}

// Indent moves the selected paragraphs one step right (dir 1) or left
// (-1): list paragraphs change level, others move by half an inch.
func (e *Editor) Indent(dir int) {
	defer e.Op(either(dir > 0, "Increase Indent", "Decrease Indent"))()
	e.FormatParas(func(p *Paragraph) {
		if e.inList(p) {
			l := *e.Doc.ResolvePara(p).List
			l.Level = min(max(l.Level+dir, 0), 8)
			p.Props.List = &l
			return
		}
		left := 0.0
		if v := e.Doc.ResolvePara(p).IndentLeft; v != nil {
			left = *v
		}
		left = max(0, (float64(int(left/36+0.001))+float64(dir))*36)
		if dir < 0 && left == 0 && p.Props.IndentLeft == nil {
			return
		}
		p.Props.IndentLeft = F(left)
	})
}

// ListKind is the kind of list a paragraph is in: "", "bullet" or a
// number format.
func (e *Editor) ListKind(p *Paragraph) string {
	if !e.inList(p) {
		return ""
	}
	ref := e.Doc.ResolvePara(p).List
	l := e.Doc.Lists[ref.NumID]
	if ref.Level < len(l.Levels) {
		return l.Levels[ref.Level].Format
	}
	return "decimal"
}

// ToggleList puts the selected paragraphs in a bulleted (or numbered)
// list, or takes them out if they all are in one already. A list just
// above the selection of the same kind is continued.
func (e *Editor) ToggleList(bullet bool) {
	defer e.Op(either(bullet, "Bullets", "Numbering"))()
	isKind := func(p *Paragraph) bool {
		k := e.ListKind(p)
		return k != "" && (k == "bullet") == bullet
	}
	paras := e.SelectedParas()
	all := true
	for _, i := range paras {
		all = all && isKind(e.locs[i].P)
	}
	if all {
		e.FormatParas(func(p *Paragraph) { e.setList(p, nil) })
		return
	}
	// Continue the list of the paragraph above, if it is of this kind.
	id := 0
	if i := paras[0] - 1; i >= 0 && isKind(e.locs[i].P) {
		id = e.Doc.ResolvePara(e.locs[i].P).List.NumID
	}
	created := id == 0
	e.formatParas(created, func(p *Paragraph) {
		if id == 0 {
			id = e.newList(bullet)
		}
		lvl := 0
		if e.inList(p) {
			lvl = e.Doc.ResolvePara(p).List.Level
		}
		p.Props.IndentLeft, p.Props.IndentFirst = nil, nil
		e.setList(p, &ListRef{NumID: id, Level: lvl})
	})
}

// newList adds a list definition with Word's default bullets or numbers.
func (e *Editor) newList(bullet bool) int {
	id := 1
	for k := range e.Doc.Lists {
		id = max(id, k+1)
	}
	l := &List{}
	bullets := []string{"•", "o", "▪"}
	formats := []string{"decimal", "lowerLetter", "lowerRoman"}
	for i := range 9 {
		lv := ListLevel{Start: 1, Indent: 36 * float64(i+1), Hang: 18}
		if bullet {
			lv.Format, lv.Text = "bullet", bullets[i%3]
			if i%3 == 1 {
				lv.Font = "Courier New"
			}
		} else {
			lv.Format = formats[i%3]
			lv.Text = "%" + string(rune('1'+i)) + "."
		}
		l.Levels = append(l.Levels, lv)
	}
	e.Doc.Lists[id] = l
	return id
}

// ModifyStyle changes a style's definition (undoable).
func (e *Editor) ModifyStyle(id string, f func(st *Style)) {
	defer e.Op("Modify Style")()
	st := e.Doc.Styles.Find(id)
	if st == nil {
		return
	}
	e.edit("style", 0, -1, true, func() {
		f(e.Doc.Styles.Find(id))
	})
}

// ModifyStyleSheet changes the style sheet, e.g. adds a style (undoable).
func (e *Editor) ModifyStyleSheet(f func(ss *StyleSheet)) {
	e.edit("style", 0, -1, true, func() { f(&e.Doc.Styles) })
}

// ParaStyleID is the style ID of paragraph i ("" means the default).
func (e *Editor) ParaStyleID(i int) string {
	return styleOrDefault(e.Doc, e.locs[i].P.Style)
}

// Stats counts non-empty paragraphs, words and characters (spaces
// included), as Word's status bar and Info page do.
func (d *Document) Stats() (paras, words, chars int) {
	d.Paragraphs(func(p *Paragraph) bool {
		t := p.Text()
		if strings.TrimSpace(t) != "" {
			paras++
		}
		words += len(strings.Fields(t))
		for _, r := range t {
			if r != '\n' && r != '\f' && r != '￼' {
				chars++
			}
		}
		return true
	})
	return
}

// Clone returns a deep copy of the document, for writing it in the
// background while editing goes on.
func (d *Document) Clone() *Document {
	c := *d
	c.Styles = CopyStyles(d.Styles)
	c.Body = CopyBlocks(d.Body)
	c.Lists = copyLists(d.Lists)
	c.Warnings = slices.Clone(d.Warnings)
	c.HF = d.HF.Clone()
	return &c
}
