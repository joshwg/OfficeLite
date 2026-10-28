package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"officelite/common/colors"
	"officelite/common/gui"
	wl "officelite/word/layout"
	"officelite/word/model"
)

// Find and Replace (Ctrl+F, Ctrl+H) and Go To (Ctrl+G): a bar over the
// page view, as Word's navigation search. Every match is highlighted in
// the pages; Enter / Shift+Enter (or F3 / Shift+F3) select the next or
// previous one and the bar counts them ("3 of 12"). Replace replaces the
// selected match and moves on; Replace All is one undoable step.

type findMode int8

const (
	findOnly findMode = iota
	findReplace
	findGoTo
)

type findBar struct {
	open  bool
	mode  findMode
	focus *widget.Editor // editor to focus at the next frame

	what, with, page widget.Editor
	matchCase, whole bool

	caseBtn, wholeBtn, prev, next, replOne, replAll, goBtn, closeBtn widget.Clickable

	// The matches of the current search, and what they were found for.
	matches []model.Match
	key     findKey
	msg     string
}

type findKey struct {
	q       string
	opts    model.FindOptions
	ed      *model.Editor
	version int
}

// openFind shows the bar in a mode and focuses its first box with its
// text selected; the selection (a single line) becomes the search text.
func (d *window) openFind(mode findMode) {
	f := &d.find
	f.open, f.mode, f.msg = true, mode, ""
	d.menu.Close()
	if mode == findGoTo {
		f.focus = &f.page
		f.page.SetCaret(f.page.Len(), 0)
		return
	}
	if !d.ed.Sel.Empty() {
		if t := d.ed.Copy().Text(); !strings.ContainsAny(t, "\r\n") && len([]rune(t)) <= 255 {
			f.what.SetText(t)
		}
	}
	f.what.SetCaret(f.what.Len(), 0)
	f.focus = &f.what
}

// closeFind hides the bar and gives the keyboard back to the page.
func (d *window) closeFind() {
	d.find.open = false
	d.find.matches = nil
	d.v.focusPending = true
}

func (f *findBar) options() model.FindOptions {
	return model.FindOptions{MatchCase: f.matchCase, WholeWords: f.whole}
}

// findMatches returns the matches of the current search, searching again
// when the text, the options or the document changed.
func (d *window) findMatches() []model.Match {
	f := &d.find
	q := f.what.Text()
	if q == "" || f.mode == findGoTo {
		f.matches = nil
		return nil
	}
	k := findKey{q, f.options(), d.ed, d.ed.Version}
	if k != f.key {
		f.key = k
		f.matches = d.ed.FindAll(q, k.opts)
	}
	return f.matches
}

// current returns the index of the match the selection is on, or -1.
func (d *window) currentMatch() int {
	a, b := d.ed.Sel.Ordered()
	for i, m := range d.findMatches() {
		if m.A == a && m.B == b {
			return i
		}
	}
	return -1
}

// findNext selects the next (or previous) match after the selection.
func (d *window) findNext(back bool) {
	ms := d.findMatches()
	f := &d.find
	if len(ms) == 0 {
		if f.what.Text() != "" {
			f.msg = "No results"
		}
		return
	}
	a, b := d.ed.Sel.Ordered()
	from := b
	if back {
		from = a
	}
	i := model.MatchIndex(ms, from, back)
	d.ed.Select(ms[i].A, ms[i].B)
	f.msg = ""
	d.edited()
	d.v.focusPending = false
}

// replaceOne replaces the selected match (if the selection is one) and
// selects the next.
func (d *window) replaceOne() {
	if i := d.currentMatch(); i >= 0 {
		m := d.find.matches[i]
		d.ed.Replace(m, d.find.with.Text())
		d.ed.SetCaret(d.ed.Sel.Caret, false)
	}
	d.findNext(false)
}

func (d *window) replaceAll() {
	f := &d.find
	n := d.ed.ReplaceAll(f.what.Text(), f.with.Text(), f.options())
	switch n {
	case 0:
		f.msg = "No results"
	case 1:
		f.msg = "Replaced 1 occurrence"
	default:
		f.msg = fmt.Sprintf("Replaced %d occurrences", n)
	}
	d.edited()
	d.v.focusPending = false
}

// goToPage moves the caret to the start of a page.
func (d *window) goToPage() {
	f := &d.find
	n, err := strconv.Atoi(strings.TrimSpace(f.page.Text()))
	if err != nil || n < 1 || n > len(d.eng.Pages) {
		f.msg = fmt.Sprintf("Enter a page number from 1 to %d", len(d.eng.Pages))
		return
	}
	d.exitHF()
	d.relayout()
	pos := d.eng.HitTest(n-1, 0, 0)
	d.ed.SetCaret(pos, false)
	f.msg = ""
	d.edited()
	d.v.scrollY = d.eng.Pages[n-1].Top - wl.PageGap
	d.v.ensureCaret = false
}

// findKeys handles Enter and Shift+Enter in the bar's boxes and the
// SubmitEvent of the page box.
func (d *window) findEvents(gtx layout.Context) {
	f := &d.find
	for _, ed := range []*widget.Editor{&f.what, &f.with, &f.page} {
		for {
			ev, ok := gtx.Event(key.Filter{Focus: ed, Name: key.NameReturn, Optional: key.ModShift},
				key.Filter{Focus: ed, Name: key.NameEnter, Optional: key.ModShift})
			if !ok {
				break
			}
			e, ok := ev.(key.Event)
			if !ok || e.State != key.Press {
				continue
			}
			switch {
			case ed == &f.page:
				d.goToPage()
			case ed == &f.with:
				d.replaceOne()
			default:
				d.findNext(e.Modifiers.Contain(key.ModShift))
			}
		}
		for {
			if _, ok := ed.Update(gtx); !ok {
				break
			}
		}
	}
}

// findBarLayout draws the bar (nothing when it is closed).
func (d *window) findBarLayout(gtx layout.Context) layout.Dimensions {
	f := &d.find
	if !f.open {
		return layout.Dimensions{}
	}
	th := d.th
	if f.focus != nil {
		// Repeated until it takes: the box may be new this frame.
		if gtx.Focused(f.focus) {
			f.focus = nil
		} else {
			gtx.Execute(key.FocusCmd{Tag: f.focus})
		}
	}
	d.findEvents(gtx)
	for f.caseBtn.Clicked(gtx) {
		f.matchCase = !f.matchCase
	}
	for f.wholeBtn.Clicked(gtx) {
		f.whole = !f.whole
	}
	for f.prev.Clicked(gtx) {
		d.findNext(true)
	}
	for f.next.Clicked(gtx) {
		d.findNext(false)
	}
	for f.replOne.Clicked(gtx) {
		d.replaceOne()
	}
	for f.replAll.Clicked(gtx) {
		d.replaceAll()
	}
	for f.goBtn.Clicked(gtx) {
		d.goToPage()
	}
	for f.closeBtn.Clicked(gtx) {
		d.closeFind()
	}
	if !f.open {
		return layout.Dimensions{}
	}

	rowH := gtx.Dp(34)
	rows := 1
	if f.mode == findReplace {
		rows = 2
	}
	w := gtx.Constraints.Max.X
	h := rowH * rows
	gtx.Constraints = layout.Exact(image.Pt(w, h))
	gui.FillRect(gtx.Ops, image.Rect(0, 0, w, h), gui.ChromeBG)
	gui.HLine(gtx.Ops, 0, w, h-1, 1, gui.ChromeLine)

	box := func(ed *widget.Editor, width unit.Dp, hint string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
				return d.textBox(gtx, ed, width, hint)
			})
		})
	}
	label := func(s string, width unit.Dp) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(width)
			return gui.Middle(gtx, th.Label(13, s, gui.ChromeText, font.Normal).Layout)
		})
	}
	text := func(b *widget.Clickable, s string, checked bool) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return gui.Middle(gtx, func(gtx layout.Context) layout.Dimensions {
				return gui.ToolButton(gtx, th, b, nil, s, checked, color.NRGBA{})
			})
		})
	}
	gap := layout.Rigid(layout.Spacer{Width: 6}.Layout)
	status := f.msg
	if status == "" && f.mode != findGoTo {
		if ms := d.findMatches(); len(ms) > 0 {
			if i := d.currentMatch(); i >= 0 {
				status = fmt.Sprintf("%d of %d", i+1, len(ms))
			} else if len(ms) == 1 {
				status = "1 result"
			} else {
				status = fmt.Sprintf("%d results", len(ms))
			}
		} else if f.what.Text() != "" {
			status = "No results"
		}
	}
	row := func(children ...layout.FlexChild) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints = layout.Exact(image.Pt(w, rowH))
			return layout.Inset{Left: 10, Right: 6}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
			})
		}
	}
	closer := text(&f.closeBtn, "✕", false)
	var first layout.Widget
	if f.mode == findGoTo {
		first = row(label("Go to page", 90), box(&f.page, 80, ""), gap, text(&f.goBtn, "Go To", false), gap,
			layout.Rigid(th.Label(12, fmt.Sprintf("1 to %d   %s", len(d.eng.Pages), status), gui.MutedText, font.Normal).Layout),
			layout.Flexed(1, layout.Spacer{}.Layout), closer)
	} else {
		first = row(label("Find", 70), box(&f.what, 240, "Search for"), gap,
			text(&f.prev, "▲", false), text(&f.next, "▼", false), gap,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(110)
				return gui.Middle(gtx, th.Label(12, status, gui.MutedText, font.Normal).Layout)
			}),
			text(&f.caseBtn, "Match case", f.matchCase), gap,
			text(&f.wholeBtn, "Whole words", f.whole),
			layout.Flexed(1, layout.Spacer{}.Layout), closer)
	}
	children := []layout.FlexChild{layout.Rigid(first)}
	if f.mode == findReplace {
		children = append(children, layout.Rigid(row(label("Replace", 70), box(&f.with, 240, "Replace with"), gap,
			text(&f.replOne, "Replace", false), gap, text(&f.replAll, "Replace All", false))))
	}
	layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// textBox draws a single-line editor with a border, width wide.
func (d *window) textBox(gtx layout.Context, ed *widget.Editor, width unit.Dp, hint string) layout.Dimensions {
	ed.SingleLine = true
	w := gtx.Dp(width)
	gtx.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, gtx.Dp(26))}
	m := op.Record(gtx.Ops)
	dims := layout.Inset{Left: 5, Right: 5, Top: 4, Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		if ed.Len() == 0 && hint != "" && !gtx.Focused(ed) {
			m := op.Record(gtx.Ops)
			ed.Layout(gtx, d.th.Shaper, gui.UIFont, unit.Sp(13), gui.Material(gtx.Ops, gui.ChromeText), gui.Material(gtx.Ops, selColor))
			m.Stop()
			return d.th.Label(13, hint, gui.MutedText, font.Normal).Layout(gtx)
		}
		return ed.Layout(gtx, d.th.Shaper, gui.UIFont, unit.Sp(13), gui.Material(gtx.Ops, gui.ChromeText), gui.Material(gtx.Ops, selColor))
	})
	call := m.Stop()
	border := gui.ChromeLine
	if gtx.Focused(ed) {
		border = d.th.Accent
	}
	gui.FillRect(gtx.Ops, image.Rectangle{Max: dims.Size}.Inset(-1), border)
	gui.FillRect(gtx.Ops, image.Rectangle{Max: dims.Size}, colors.White)
	call.Add(gtx.Ops)
	return dims
}

// drawMatches highlights the find matches on page cpg of eng.
func (d *window) drawMatches(gtx layout.Context, eng *wl.Engine, cpg *wl.Page, rect func(wl.Rect) image.Rectangle) {
	if !d.find.open || d.find.mode == findGoTo || len(cpg.Lines) == 0 {
		return
	}
	ms := d.findMatches()
	if len(ms) == 0 {
		return
	}
	lo, hi := cpg.Lines[0].Para, cpg.Lines[0].Para
	for _, pl := range cpg.Lines {
		lo, hi = min(lo, pl.Para), max(hi, pl.Para)
	}
	a, b := d.ed.Sel.Ordered()
	for _, m := range ms {
		if m.A.Para < lo || m.A.Para > hi {
			continue
		}
		c := findColor
		if m.A == a && m.B == b {
			c = findCur
		}
		for _, r := range eng.SelectionRects(cpg, m.A, m.B) {
			gui.FillRect(gtx.Ops, rect(r), c)
		}
	}
}
