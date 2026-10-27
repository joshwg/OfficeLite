package gui

import (
	"image"
	"image/color"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/colors"
	"officelite/common/recent"
	"officelite/common/version"
)

// Page is a page of the backstage, the full-window view behind the File
// menu, as in Excel.
type Page int

const (
	PageHome Page = iota
	PageNew
	PageOpen
	PageInfo
	PageSave
	PageSaveAs
	PagePrint
	PageExport
	PageClose
	numPages
)

var pageNames = [numPages]string{"Home", "New", "Open", "Info", "Save", "Save As", "Print", "Export", "Close"}

func (p Page) String() string { return pageNames[p] }

// Template is a tile in the New section.
type Template struct {
	Name string
	Kind recent.Kind
}

// Action is what a backstage event asks the app to do.
type Action int

const (
	ActBack       Action = iota // the backstage was dismissed
	ActNew                      // Template holds the chosen tile's index
	ActOpenRecent               // Entry holds the file
	ActBrowse                   // show the Open dialog
	ActSave
	ActClose // close the window
)

// BackstageEvent is a request from the backstage.
type BackstageEvent struct {
	Action   Action
	Template int
	Entry    recent.Entry
}

// BackstageConfig supplies the app-specific parts of the backstage.
type BackstageConfig struct {
	Templates []Template
	Recent    []recent.Entry
	// Content returns the rows of the app's own pages (Info, Save As,
	// Print, Export), below the page title.
	Content func(p Page) []layout.Widget
}

// Backstage is the full-window File view: a navigation column on the left
// (Home, New, Open, Info, Save, Save As, Print, Export, Close) and the
// selected page on the right.
type Backstage struct {
	Visible bool
	Page    Page

	back    widget.Clickable
	nav     [numPages]widget.Clickable
	browse  widget.Clickable
	tiles   []widget.Clickable
	recents []widget.Clickable
	list    widget.List
}

// Show opens the backstage at page p.
func (b *Backstage) Show(p Page) {
	b.Visible = true
	b.Page = p
	b.list.Position = layout.Position{}
}

// Hide dismisses the backstage.
func (b *Backstage) Hide() { b.Visible = false }

// Layout fills the constraints with the backstage and calls handle for
// each request made since the last frame.
func (b *Backstage) Layout(gtx layout.Context, th *Theme, cfg BackstageConfig, handle func(BackstageEvent)) layout.Dimensions {
	b.update(gtx, cfg, handle)
	size := gtx.Constraints.Max
	FillRect(gtx.Ops, image.Rectangle{Max: size}, colors.White)
	return layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return b.navColumn(gtx, th) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return b.content(gtx, th, cfg) }),
	)
}

func (b *Backstage) update(gtx layout.Context, cfg BackstageConfig, handle func(BackstageEvent)) {
	for b.back.Clicked(gtx) {
		b.Visible = false
		handle(BackstageEvent{Action: ActBack})
	}
	for p := range b.nav {
		for b.nav[p].Clicked(gtx) {
			switch Page(p) {
			case PageSave:
				handle(BackstageEvent{Action: ActSave})
			case PageClose:
				handle(BackstageEvent{Action: ActClose})
			default:
				b.Show(Page(p))
			}
		}
	}
	for b.browse.Clicked(gtx) {
		handle(BackstageEvent{Action: ActBrowse})
	}
	if len(b.tiles) != len(cfg.Templates) {
		b.tiles = make([]widget.Clickable, len(cfg.Templates))
	}
	for i := range b.tiles {
		for b.tiles[i].Clicked(gtx) {
			handle(BackstageEvent{Action: ActNew, Template: i})
		}
	}
	if len(b.recents) < len(cfg.Recent) {
		b.recents = append(b.recents, make([]widget.Clickable, len(cfg.Recent)-len(b.recents))...)
	}
	for i := range cfg.Recent {
		for b.recents[i].Clicked(gtx) {
			handle(BackstageEvent{Action: ActOpenRecent, Entry: cfg.Recent[i]})
		}
	}
}

func (b *Backstage) navColumn(gtx layout.Context, th *Theme) layout.Dimensions {
	w := gtx.Dp(180)
	h := gtx.Constraints.Max.Y
	gtx.Constraints = layout.Exact(image.Pt(w, h))
	FillRect(gtx.Ops, image.Rect(0, 0, w, h), th.Accent)
	hover := colors.RGBA(255, 255, 255, 0x30)
	rows := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 12, Bottom: 12, Left: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return Hoverable(gtx, &b.back, colors.Transparent, hover, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(6).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(24)
						return IconBack.Layout(gtx, colors.White)
					})
				})
			})
		}),
	}
	for p := Page(0); p < numPages; p++ {
		if p == PageInfo {
			rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				m := gtx.Dp(16)
				HLine(gtx.Ops, m, w-m, gtx.Dp(8), 1, colors.RGBA(255, 255, 255, 0x50))
				return layout.Dimensions{Size: image.Pt(w, gtx.Dp(17))}
			}))
		}
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bg := colors.Transparent
			if p == b.Page {
				bg = th.AccentDark
			}
			return Hoverable(gtx, &b.nav[p], bg, colors.Mix(th.Accent, th.AccentDark, 0.6), func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(w, gtx.Dp(38)))
				return layout.Inset{Left: 18}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return Middle(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Dp(18)
								return pageIcon(p).Layout(gtx, colors.White)
							}),
							layout.Rigid(layout.Spacer{Width: 12}.Layout),
							layout.Rigid(th.Label(14, p.String(), colors.White, font.Normal).Layout),
						)
					})
				})
			})
		}))
	}
	rows = append(rows,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Min} }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 18, Bottom: 14}.Layout(gtx,
				th.Label(12, version.String(), colors.RGBA(255, 255, 255, 0xC0), font.Normal).Layout)
		}),
	)
	layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	return layout.Dimensions{Size: image.Pt(w, h)}
}

func pageIcon(p Page) *widget.Icon {
	return [numPages]*widget.Icon{IconHome, IconNew, IconOpen, IconInfo, IconSave, IconSave, IconPrint, IconExport, IconClose}[p]
}

func (b *Backstage) content(gtx layout.Context, th *Theme, cfg BackstageConfig) layout.Dimensions {
	var rows []layout.Widget
	switch b.Page {
	case PageHome:
		rows = append(rows, th.Title(greeting(time.Now())), th.Heading("New"), b.tileRow(th, cfg.Templates),
			th.Heading("Recent"))
		rows = append(rows, b.recentRows(th, cfg.Recent, 12)...)
	case PageNew:
		rows = append(rows, th.Title("New"), b.tileRow(th, cfg.Templates))
	case PageOpen:
		rows = append(rows, th.Title("Open"),
			func(gtx layout.Context) layout.Dimensions {
				return th.Option(gtx, &b.browse, IconFolder, "Browse", "Choose a file on this PC", false)
			},
			th.Heading("Recent"))
		rows = append(rows, b.recentRows(th, cfg.Recent, len(cfg.Recent))...)
	default:
		rows = append(rows, th.Title(b.Page.String()))
		if cfg.Content != nil {
			rows = append(rows, cfg.Content(b.Page)...)
		}
	}
	b.list.Axis = layout.Vertical
	return layout.Inset{Top: 32, Left: 48, Right: 24}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(th.Theme, &b.list).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
			return layout.Inset{Right: 24, Bottom: 2}.Layout(gtx, rows[i])
		})
	})
}

func greeting(now time.Time) string {
	switch h := now.Hour(); {
	case h < 12:
		return "Good morning"
	case h < 18:
		return "Good afternoon"
	default:
		return "Good evening"
	}
}

// tileRow is the row of New tiles.
func (b *Backstage) tileRow(th *Theme, ts []Template) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, 2*len(ts))
		for i, t := range ts {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return b.tile(gtx, th, &b.tiles[i], t)
			}), layout.Rigid(layout.Spacer{Width: 20}.Layout))
		}
		return layout.Inset{Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx, children...)
		})
	}
}

func (b *Backstage) tile(gtx layout.Context, th *Theme, btn *widget.Clickable, t Template) layout.Dimensions {
	return Hoverable(gtx, btn, colors.Transparent, colors.Transparent, func(gtx layout.Context) layout.Dimensions {
		tw, tht := gtx.Dp(150), gtx.Dp(110)
		border := ChromeLine
		if btn.Hovered() {
			border = th.Accent
		}
		bw := gtx.Dp(1)
		if btn.Hovered() {
			bw = gtx.Dp(2)
		}
		FillRect(gtx.Ops, image.Rect(0, 0, tw, tht), border)
		g := gtx
		g.Constraints = layout.Exact(image.Pt(tw-2*bw, tht-2*bw))
		off := op.Offset(image.Pt(bw, bw)).Push(gtx.Ops)
		Thumbnail(g, t.Kind, th.Accent)
		off.Pop()
		off = op.Offset(image.Pt(0, tht+gtx.Dp(8))).Push(gtx.Ops)
		g.Constraints = layout.Constraints{Max: image.Pt(tw, gtx.Dp(40))}
		l := th.Label(13, t.Name, ChromeText, font.Normal)
		d := l.Layout(g)
		off.Pop()
		return layout.Dimensions{Size: image.Pt(tw, tht+gtx.Dp(8)+d.Size.Y)}
	})
}

// recentRows lists up to n recent files.
func (b *Backstage) recentRows(th *Theme, list []recent.Entry, n int) []layout.Widget {
	if len(list) == 0 {
		return []layout.Widget{th.Paragraph("Files you open will appear here.", MutedText)}
	}
	now := time.Now()
	rows := make([]layout.Widget, 0, min(n, len(list)))
	for i, e := range list[:min(n, len(list))] {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			return Hoverable(gtx, &b.recents[i], colors.Transparent, HoverBG, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Inset{Top: 7, Bottom: 7, Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							sz := image.Pt(gtx.Dp(26), gtx.Dp(32))
							gtx.Constraints = layout.Exact(sz)
							FillRect(gtx.Ops, image.Rectangle{Max: sz}, ChromeLine)
							off := op.Offset(image.Pt(1, 1)).Push(gtx.Ops)
							g := gtx
							g.Constraints = layout.Exact(sz.Sub(image.Pt(2, 2)))
							Thumbnail(g, e.Kind, kindAccent(e.Kind, th))
							off.Pop()
							return layout.Dimensions{Size: sz}
						}),
						layout.Rigid(layout.Spacer{Width: 14}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(th.Label(14, e.Name(), ChromeText, font.Normal).Layout),
								layout.Rigid(th.Label(12, e.Dir(), MutedText, font.Normal).Layout),
							)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Dp(170)
							l := th.Label(12, When(e.Opened, now), MutedText, font.Normal)
							l.Alignment = text.End
							return l.Layout(gtx)
						}),
					)
				})
			})
		})
	}
	return rows
}

// Accent colors of the apps, for recent-file icons.
var (
	ExcelAccent = colors.RGB(0x107C41)
	WordAccent  = colors.RGB(0x185ABD)
)

func kindAccent(k recent.Kind, th *Theme) color.NRGBA {
	switch k {
	case recent.Spreadsheet:
		return ExcelAccent
	case recent.Document:
		return WordAccent
	}
	return th.Accent
}

// Thumbnail draws a blank sheet (a grid) or a blank page (text lines)
// filling the constraints.
func Thumbnail(gtx layout.Context, kind recent.Kind, accent color.NRGBA) {
	sz := gtx.Constraints.Max
	FillRect(gtx.Ops, image.Rectangle{Max: sz}, colors.White)
	line := colors.Mix(ChromeLine, colors.White, 0.3)
	if kind == recent.Document {
		m := sz.X / 6
		step := max(sz.Y/10, 2)
		for y := m + step; y < sz.Y-m; y += step {
			x1 := sz.X - m
			if (y/step)%4 == 0 {
				x1 = sz.X - 2*m // short line ending a paragraph
			}
			HLine(gtx.Ops, m, x1, y, max(1, step/4), line)
		}
		HLine(gtx.Ops, m, sz.X/2, m, max(1, step/3), accent)
		return
	}
	rowH := max(sz.Y/8, 3)
	colW := max(sz.X/5, 4)
	for y := rowH; y < sz.Y; y += rowH {
		HLine(gtx.Ops, 0, sz.X, y, 1, line)
	}
	for x := colW; x < sz.X; x += colW {
		VLine(gtx.Ops, x, 0, sz.Y, 1, line)
	}
	// The selected first cell, as in Excel's blank workbook tile.
	r := image.Rect(colW, rowH, 2*colW+1, 2*rowH+1)
	t := max(1, gtx.Dp(1.5))
	FillRect(gtx.Ops, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+t), accent)
	FillRect(gtx.Ops, image.Rect(r.Min.X, r.Max.Y-t, r.Max.X, r.Max.Y), accent)
	FillRect(gtx.Ops, image.Rect(r.Min.X, r.Min.Y, r.Min.X+t, r.Max.Y), accent)
	FillRect(gtx.Ops, image.Rect(r.Max.X-t, r.Min.Y, r.Max.X, r.Max.Y), accent)
}

// When formats a file time for the recent list: "Today 3:04 PM",
// "Yesterday 9:15 AM", "Mon 9:15 AM" within a week, else "Jan 2, 2006".
func When(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	t = t.In(now.Location())
	day := func(x time.Time) time.Time { return time.Date(x.Year(), x.Month(), x.Day(), 0, 0, 0, 0, x.Location()) }
	days := int(day(now).Sub(day(t)).Hours()/24 + 0.5)
	switch {
	case days == 0:
		return "Today " + t.Format("3:04 PM")
	case days == 1:
		return "Yesterday " + t.Format("3:04 PM")
	case days > 1 && days < 7:
		return t.Format("Mon 3:04 PM")
	case t.Year() == now.Year():
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2, 2006")
	}
}
