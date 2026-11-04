// The Format Page Numbers dialog (Header & Footer bar): the number format
// of a section's PAGE fields and whether its numbering continues from the
// previous section or starts again at a number.

package ui

import (
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/widget"

	"officelite/common/gui"
	"officelite/word"
)

// pageNumFormats are the number formats the dialog offers: the model's
// format name (see model.FormatNumber) and how it numbers pages.
var pageNumFormats = [...]struct{ format, label string }{
	{"", "1, 2, 3, …"},
	{"lowerRoman", "i, ii, iii, …"},
	{"upperRoman", "I, II, III, …"},
	{"lowerLetter", "a, b, c, …"},
	{"upperLetter", "A, B, C, …"},
}

// pageNumDialog is the Format Page Numbers dialog's state.
type pageNumDialog struct {
	open     bool
	sec      int // the section it formats (model.Document.Sections)
	format   int // index into pageNumFormats
	restart  bool
	start    widget.Editor
	formats  [len(pageNumFormats)]widget.Clickable
	cont, at widget.Clickable // Continue from previous section / Start at
	ok       widget.Clickable
	cancel   widget.Clickable
	err      string
	tag      int
}

// openPageNumbers shows the Format Page Numbers dialog for the section of
// the page whose header is edited (or the caret's).
func (d *window) openPageNumbers() {
	p := &d.hf.nums
	p.sec = d.pageSection(d.hfPage())
	secs := d.doc.Sections()
	if p.sec >= len(secs) {
		p.sec = len(secs) - 1
	}
	s := secs[p.sec]
	p.format = 0
	for i, f := range pageNumFormats {
		if f.format == s.PageNumFormat || f.format == "" && s.PageNumFormat == "decimal" {
			p.format = i
		}
	}
	p.restart = s.PageNumRestart
	start := s.PageNumStart
	if !p.restart || start <= 0 {
		start = 1
	}
	p.start.SingleLine, p.start.Submit = true, true
	p.start.SetText(strconv.Itoa(start))
	p.err, p.open = "", true
}

// applyPageNumbers sets the dialog's section's page numbering, undoably,
// and reports whether the start number was valid.
func (d *window) applyPageNumbers() bool {
	p := &d.hf.nums
	start := 1
	if p.restart {
		n, err := strconv.Atoi(strings.TrimSpace(p.start.Text()))
		if err != nil || n < 0 || n > word.UIMaxStartPage {
			p.err = "Start at must be a whole number from 0 to " + strconv.Itoa(word.UIMaxStartPage) + "."
			return false
		}
		start = n
	}
	format, restart, sec := pageNumFormats[p.format].format, p.restart, p.sec
	d.hfStructure("Format Page Numbers", func() {
		secs := d.doc.Sections()
		if sec >= len(secs) {
			return
		}
		s := secs[sec]
		s.PageNumFormat, s.PageNumRestart = format, restart
		if restart {
			s.PageNumStart = start
		}
	})
	return true
}

// layout draws the dialog and handles its input.
func (p *pageNumDialog) layout(gtx layout.Context, d *window) {
	for i := range p.formats {
		for p.formats[i].Clicked(gtx) {
			p.format = i
		}
	}
	for p.cont.Clicked(gtx) {
		p.restart = false
	}
	for p.at.Clicked(gtx) {
		p.restart = true
	}
	submit := submitted(gtx, &p.start)
	for p.ok.Clicked(gtx) {
		submit = true
	}
	if submit && d.applyPageNumbers() {
		p.open = false
		d.v.focusPending = true
		return
	}
	for p.cancel.Clicked(gtx) {
		p.open = false
		d.v.focusPending = true
		return
	}
	th := d.th
	gui.Modal(gtx, &p.tag, 420, func(gtx layout.Context) layout.Dimensions {
		kids := []layout.FlexChild{dialogTitle(th, "Page Number Format"),
			layout.Rigid(th.Label(13, "Number format:", gui.MutedText, font.Normal).Layout)}
		for i, f := range pageNumFormats {
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return th.Check(gtx, &p.formats[i], f.label, i == p.format)
			}))
		}
		kids = append(kids, layout.Rigid(layout.Spacer{Height: 10}.Layout),
			layout.Rigid(th.Label(13, "Page numbering:", gui.MutedText, font.Normal).Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := "Continue from previous section"
				if p.sec == 0 {
					label = "Start at 1"
				}
				return th.Check(gtx, &p.cont, label, !p.restart)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return th.Check(gtx, &p.at, "Start at:", p.restart)
			}),
			field(th, "", &p.start, 80))
		if p.err != "" {
			kids = append(kids, layout.Rigid(th.Label(12, p.err, gui.ErrorText, font.Normal).Layout))
		}
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.Button(gtx, &p.ok, "OK") }),
					layout.Rigid(layout.Spacer{Width: 8}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.SecondaryButton(gtx, &p.cancel, "Cancel") }),
				)
			})
		}))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
	})
}
