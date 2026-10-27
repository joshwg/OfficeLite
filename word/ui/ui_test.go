package ui

import (
	"time"

	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"officelite/common/gui"
	"officelite/common/recent"
	"officelite/word/model"
)

type harness struct {
	t   *testing.T
	d   *window
	r   input.Router
	ops op.Ops
}

func newHarness(t *testing.T) *harness {
	fonts := gui.NewFontLibrary(gui.DefaultFontDirs()...)
	d := newWindow(nil, Config{Fonts: fonts, Recent: recent.Open("")})
	doc := model.New("Test")
	d.setDoc(doc, fonts.Shaper(docFonts(doc)...), docFonts(doc))
	h := &harness{t: t, d: d}
	h.frame()
	h.frame()
	return h
}

func (h *harness) frame() {
	h.ops.Reset()
	gtx := layout.Context{Ops: &h.ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1100, 760)), Source: h.r.Source()}
	h.d.layout(gtx)
	h.r.Frame(gtx.Ops)
}

func (h *harness) typeText(s string) {
	h.r.Queue(key.EditEvent{Text: s})
	h.frame()
}

func (h *harness) press(name key.Name, mods key.Modifiers) {
	h.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	h.frame()
}

func (h *harness) para(i int) *model.Paragraph { return h.d.ed.Para(i) }

func TestTypingAndStyles(t *testing.T) {
	h := newHarness(t)
	d := h.d
	if !d.v.focused {
		t.Fatal("page view not focused")
	}
	h.typeText("Report")
	h.press(key.NameReturn, 0)
	h.typeText("Body text")
	if got := d.doc.Body; len(got) != 2 || h.para(0).Text() != "Report" || h.para(1).Text() != "Body text" {
		t.Fatalf("paragraphs: %q, %q", h.para(0).Text(), h.para(1).Text())
	}
	// Ctrl+Alt+1 on the first paragraph applies Heading 1.
	h.press(key.NameUpArrow, 0)
	h.press("1", key.ModCtrl|key.ModAlt)
	if s := h.para(0).Style; s != "Heading1" {
		t.Fatalf("style %q", s)
	}
	if st := d.toolState(); st.style != "Heading 1" {
		t.Errorf("style box shows %q", st.style)
	}
	// Select "Body" with Shift+Ctrl+Right and make it bold.
	h.press(key.NameDownArrow, 0)
	h.press(key.NameHome, 0)
	h.press(key.NameRightArrow, key.ModCtrl|key.ModShift)
	h.press("B", key.ModCtrl)
	p := h.para(1)
	if len(p.Runs) != 2 || p.Runs[0].Text != "Body " || p.Runs[0].Props.Bold != model.On {
		t.Fatalf("bold runs: %+v", p.Runs)
	}
	if !d.toolState().bold {
		t.Error("Bold button not pressed")
	}
	if !d.modified() {
		t.Error("not marked modified")
	}
	// Undo the bold, then the heading.
	h.press("Z", key.ModCtrl)
	if p := h.para(1); len(p.Runs) != 1 {
		t.Fatalf("undo bold: %+v", p.Runs)
	}
	h.press("Z", key.ModCtrl)
	if s := h.para(0).Style; s != "" {
		t.Fatalf("undo heading: %q", s)
	}
	h.press("Y", key.ModCtrl)
	if s := h.para(0).Style; s != "Heading1" {
		t.Fatalf("redo heading: %q", s)
	}
	// Ctrl+Shift+N goes back to Normal; Ctrl+E centers.
	h.press(key.NameUpArrow, 0)
	h.press("N", key.ModCtrl|key.ModShift)
	h.press("E", key.ModCtrl)
	if p := h.para(0); p.Style != "" || p.Props.Align != model.AlignCenter {
		t.Fatalf("normal/center: %q %v", p.Style, p.Props.Align)
	}
	// Backspace at the start of paragraph 2 joins it.
	h.press(key.NameDownArrow, 0)
	h.press(key.NameHome, 0)
	h.press(key.NameDeleteBackward, 0)
	if len(d.doc.Body) != 1 || h.para(0).Text() != "ReportBody text" {
		t.Fatalf("join: %q", h.para(0).Text())
	}
	// Ctrl+Space then a space: the space from Ctrl+Space is not typed.
	h.press(key.NameSpace, key.ModCtrl)
	h.typeText(" ")
	if h.para(0).Text() != "ReportBody text" {
		t.Errorf("Ctrl+Space typed a space: %q", h.para(0).Text())
	}
}

func TestClickPlacesCaret(t *testing.T) {
	h := newHarness(t)
	d := h.d
	h.typeText("Hello world, this is a test")
	h.frame()
	// Find where "world" is drawn and click it twice to select the word.
	c, ok := d.eng.CaretAt(model.Pos{Para: 0, Off: 8})
	if !ok {
		t.Fatal("no caret")
	}
	viewTop := 36 + 30 + 38 // title, menu bar, toolbar
	x := float32(d.pageX()) + float32(c.X*d.v.scale)
	y := float32(viewTop) + float32((d.eng.Pages[0].Top+c.Y+c.H/2-d.v.scrollY)*d.v.scale)
	pt := f32.Pt(x, y)
	click := func() {
		h.r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pt})
		h.r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: pt})
		h.frame()
	}
	click()
	if got := d.ed.Sel.Caret; got != (model.Pos{Para: 0, Off: 8}) {
		t.Fatalf("click caret %v", got)
	}
	click()
	a, b := d.ed.Sel.Ordered()
	if a.Off != 6 || b.Off != 11 {
		t.Fatalf("double-click selected %v..%v", a, b)
	}
	h.typeText("there")
	if got := h.para(0).Text(); got != "Hello there, this is a test" {
		t.Errorf("replace word: %q", got)
	}
}

// TestTypingSpeed types into the middle of a 50+ page document and
// reports the time per keystroke frame (edit, relayout, draw).
func TestTypingSpeed(t *testing.T) {
	h := newHarness(t)
	d := h.d
	const lorem = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. "
	doc := model.New("Long")
	doc.Body = nil
	for i := range 700 {
		p := &model.Paragraph{Runs: []model.Run{{Text: lorem}}}
		if i%10 == 0 {
			p.Style, p.Runs = "Heading2", []model.Run{{Text: "Section heading"}}
		}
		doc.Body = append(doc.Body, p)
	}
	d.setDoc(doc, d.eng.Shaper(), d.fams)
	h.frame()
	h.frame()
	t.Logf("%d pages, full layout %v", len(d.eng.Pages), d.eng.LastDuration)
	if len(d.eng.Pages) < 50 {
		t.Fatalf("only %d pages", len(d.eng.Pages))
	}
	d.ed.SetCaret(model.Pos{Para: 350, Off: 20}, false)
	d.edited()
	h.frame()
	start := time.Now()
	const n = 30
	var worst time.Duration
	for range n {
		t0 := time.Now()
		h.typeText("x")
		worst = max(worst, time.Since(t0))
	}
	avg := time.Since(start) / n
	t.Logf("keystroke frame: avg %v, worst %v (last relayout %v, %d paragraph)", avg, worst, d.eng.LastDuration, d.eng.LastRelaid)
	if avg > 50*time.Millisecond {
		t.Errorf("typing too slow: %v per keystroke", avg)
	}
}
