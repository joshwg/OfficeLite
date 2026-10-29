package gui

import (
	"image"
	"reflect"
	"runtime"
	"sync"
	"time"
	"weak"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"officelite/common/colors"
)

// Tooltips, as in Office: resting the pointer on a ribbon button for
// tipDelay shows a box under it with the command's name and shortcut in
// bold and a one-line description below. Moving to another button while
// one shows switches at once; leaving, pressing or tipTimeout hides it.
// Menu items show their MenuItem.Tip beside the item the same way.
//
// Callers attach text once, keyed by the button's *widget.Clickable (or
// the *DropDown of a drop or split button's arrow):
//
//	gui.SetTip(&h.bold, "Bold", "Ctrl+B", "Make your text bold.")

const (
	tipDelay   = 500 * time.Millisecond
	tipTimeout = 10 * time.Second
	tipWarm    = 300 * time.Millisecond // switch without delay within this of a shown tip
)

// Tip is the text of a tooltip.
type Tip struct {
	Name, Keys, Desc string
}

// tips holds each button's tip. Its keys are pointers into a window's
// state (&h.bold, &p.cells[i], ...), so they are held weakly: a strong
// key would keep every closed window's App reachable for good. A cleanup
// drops the entry once its button's memory is collected.
var (
	tipsMu sync.RWMutex
	tips   = map[tipKey]Tip{}
)

// tipKey identifies a button: its pointer, held weakly, and type (a
// *DropDown and its first field share an address). Keys that aren't
// pointers are kept as they are in v.
type tipKey struct {
	t reflect.Type
	p weak.Pointer[byte]
	v any
}

// tipKeyOf returns btn's key and, for a pointer, the pointer itself.
func tipKeyOf(btn any) (tipKey, *byte) {
	v := reflect.ValueOf(btn)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return tipKey{v: btn}, nil
	}
	p := (*byte)(v.UnsafePointer())
	return tipKey{t: v.Type(), p: weak.Make(p)}, p
}

// SetTip attaches a tooltip to a button, a *widget.Clickable or a
// *DropDown. keys and desc may be empty.
func SetTip(btn any, name, keys, desc string) {
	if d, ok := btn.(*DropDown); ok {
		btn = &d.btn
	}
	k, p := tipKeyOf(btn)
	tipsMu.Lock()
	_, had := tips[k]
	tips[k] = Tip{name, keys, desc}
	tipsMu.Unlock()
	if !had && p != nil {
		runtime.AddCleanup(p, dropTip, k)
	}
}

func dropTip(k tipKey) {
	tipsMu.Lock()
	delete(tips, k)
	tipsMu.Unlock()
}

func tipFor(btn any) (Tip, bool) {
	k, _ := tipKeyOf(btn)
	tipsMu.RLock()
	t, ok := tips[k]
	tipsMu.RUnlock()
	return t, ok
}

// tipTag is a button's own pointer tag (the button itself is its
// Clickable's tag).
type tipTag struct{ key any }

// tipper tracks the hovered button of one area (a ribbon or a menu) and
// draws its tip. Buttons report themselves each frame with hover.
type tipper struct {
	key      any
	rect     image.Rectangle // the hovered button, in the area's coordinates
	tip      Tip
	since    time.Time // when the pointer came to rest on key
	pressed  bool
	seen     bool // key reported this frame
	lastShow time.Time

	ptr      image.Point // the pointer, in the area's coordinates
	local    image.Point // the pointer, in localKey's coordinates
	localKey any

	force bool // tests: show at once and never time out
}

const tipperKey = "gui.tipper"

func tipperOf(gtx layout.Context) *tipper {
	t, _ := gtx.Values[tipperKey].(*tipper)
	return t
}

// withTipper returns gtx carrying t for the buttons laid out with it.
func withTipper(gtx layout.Context, t *tipper) layout.Context {
	v := make(map[string]any, len(gtx.Values)+1)
	for k, x := range gtx.Values {
		v[k] = x
	}
	v[tipperKey] = t
	gtx.Values = v
	return gtx
}

// track follows the pointer over the area; call it with the area's clip
// pushed, before laying out its buttons.
func (t *tipper) track(gtx layout.Context) {
	event.Op(gtx.Ops, t)
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: t, Kinds: pointer.Enter | pointer.Move | pointer.Leave})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			t.ptr = e.Position.Round()
		}
	}
}

// tipButton reports a ribbon button of the given size, laid out at the
// current offset, that hovered says is under the pointer.
func tipButton(gtx layout.Context, key any, hovered bool, size image.Point) {
	t := tipperOf(gtx)
	if t == nil {
		return
	}
	tip, ok := tipFor(key)
	if !ok {
		return
	}
	tag := tipTag{key}
	area := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	area.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Enter | pointer.Move | pointer.Press})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		t.local, t.localKey = e.Position.Round(), key
		if e.Kind == pointer.Press && t.key == key {
			t.pressed = true
		}
	}
	if hovered && t.localKey == key {
		o := t.ptr.Sub(t.local)
		t.hover(gtx, key, tip, image.Rectangle{Min: o, Max: o.Add(size)})
	}
}

// hover reports that key (at rect) is under the pointer this frame.
func (t *tipper) hover(gtx layout.Context, key any, tip Tip, rect image.Rectangle) {
	t.seen = true
	t.rect, t.tip = rect, tip
	if key == t.key {
		return
	}
	t.key, t.pressed, t.since = key, false, gtx.Now
	if gtx.Now.Sub(t.lastShow) < tipWarm {
		t.since = gtx.Now.Add(-tipDelay)
	}
}

// layout ends the frame: it forgets a button no longer hovered and draws
// the tip of the one that is, once it is due, in the area's coordinates
// (under the button, or beside it when beside is set), kept within
// bounds.
func (t *tipper) layout(gtx layout.Context, th *Theme, bounds image.Rectangle, beside bool) {
	seen := t.seen
	t.seen = false
	if !seen || t.key == nil {
		t.key = nil
		return
	}
	if t.pressed || (t.tip.Name == "" && t.tip.Desc == "") {
		return
	}
	now := gtx.Now
	if t.force {
		t.since = now.Add(-tipDelay)
	}
	show, hide := t.since.Add(tipDelay), t.since.Add(tipDelay+tipTimeout)
	switch {
	case now.Before(show):
		gtx.Execute(op.InvalidateCmd{At: show})
		return
	case !now.Before(hide) && !t.force:
		return
	}
	t.lastShow = now
	if !t.force {
		gtx.Execute(op.InvalidateCmd{At: hide})
	}
	gtx.Execute(op.InvalidateCmd{At: now.Add(tipWarm)}) // let the warm window lapse

	// The box: the bold name (and shortcut) over the description.
	pad := gtx.Dp(6)
	m := op.Record(gtx.Ops)
	g := gtx
	g.Constraints = layout.Constraints{Max: image.Pt(gtx.Dp(400), gtx.Dp(40))}
	y, w := pad, 0
	if t.tip.Name != "" {
		s := t.tip.Name
		if t.tip.Keys != "" {
			s += " (" + t.tip.Keys + ")"
		}
		o := op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
		d := th.Label(12, s, ChromeText, font.Bold).Layout(g)
		o.Pop()
		y, w = y+d.Size.Y, d.Size.X
	}
	if t.tip.Desc != "" {
		if t.tip.Name != "" {
			y += gtx.Dp(3)
		}
		o := op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
		d := th.Label(12, t.tip.Desc, ChromeText, font.Normal).Layout(g)
		o.Pop()
		y, w = y+d.Size.Y, max(w, d.Size.X)
	}
	text := m.Stop()
	sz := image.Pt(w+2*pad, y+pad)

	var pos image.Point
	if beside {
		pos = image.Pt(t.rect.Max.X+gtx.Dp(2), t.rect.Min.Y)
		if pos.X+sz.X > bounds.Max.X {
			pos.X = t.rect.Min.X - gtx.Dp(2) - sz.X
		}
	} else {
		pos = image.Pt(t.rect.Min.X, t.rect.Max.Y+gtx.Dp(4))
		pos.X = min(pos.X, bounds.Max.X-sz.X-1)
	}
	pos.X = max(pos.X, bounds.Min.X)
	pos.Y = max(min(pos.Y, bounds.Max.Y-sz.Y-1), bounds.Min.Y)
	o := op.Offset(pos).Push(gtx.Ops)
	FillRect(gtx.Ops, image.Rect(2, 2, sz.X+2, sz.Y+2), colors.RGBA(0, 0, 0, 0x22)) // shadow
	FillRect(gtx.Ops, image.Rect(-1, -1, sz.X+1, sz.Y+1), ChromeLine)
	FillRect(gtx.Ops, image.Rectangle{Max: sz}, colors.White)
	text.Add(gtx.Ops)
	o.Pop()
}
