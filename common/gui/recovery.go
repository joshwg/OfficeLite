package gui

import (
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"officelite/common/autorecover"
	"officelite/common/colors"
	"officelite/common/recent"
)

// AutoRecoverRows are the Info page's AutoRecover setting: how often a
// copy of unsaved changes is kept for recovery after a crash.
func (b *Backstage) AutoRecoverRows(th *Theme) []layout.Widget {
	items := make([]DropItem, len(autorecover.Choices))
	for i, m := range autorecover.Choices {
		items[i].Label = minutesLabel(m)
	}
	return []layout.Widget{
		th.Heading("AutoRecover"),
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(th.Label(13, "Save AutoRecover information every", ChromeText, font.Normal).Layout),
				layout.Rigid(layout.Spacer{Width: 10}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if i := b.recover.Layout(gtx, th, 120, minutesLabel(autorecover.Minutes()), items); i >= 0 {
						go autorecover.SetMinutes(autorecover.Choices[i]) // a file write
					}
					return layout.Dimensions{Size: image.Pt(gtx.Dp(120), gtx.Dp(24))}
				}),
			)
		},
		th.Paragraph("Unsaved changes are copied to "+OrNone(autorecover.Dir(), "the user cache directory")+
			" and offered back if OfficeLite closes without saving them.", MutedText),
	}
}

func minutesLabel(m int) string {
	switch m {
	case 0:
		return "Off"
	case 1:
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", m)
}

// RecoveryConfig sets up the Document Recovery window.
type RecoveryConfig struct {
	Fonts   *FontLibrary
	Entries []autorecover.Entry
	// Open opens a recovered copy in its app's window, which takes it
	// over (autorecover.Saver.Adopt).
	Open func(e autorecover.Entry)
	// SaveAs writes a recovered copy to path in the format path's
	// extension names. It runs on a background goroutine.
	SaveAs func(e autorecover.Entry, path string) error
	// Close closes the window; Wake asks it for a frame.
	Close, Wake func()
	// Create is the Save dialog used when there is no native one
	// (explorer's CreateFile); Declined is its error for a cancelled
	// dialog. Nil: Save As needs the native one.
	Create   func(name string) (io.WriteCloser, error)
	Declined error
}

// Recovery is the Document Recovery window: the recovery copies a crashed
// OfficeLite left, each with Open, Save As… and Discard. Close keeps the
// rest for the next start, as Office does.
type Recovery struct {
	cfg   RecoveryConfig
	th    *Theme
	Files FileDialog // gets the window's events
	calls *CallQueue

	items    []recoveryItem
	closeBtn widget.Clickable
	list     widget.List
	ask      Dialog // Discard's confirmation
	asking   int    // the item it is about
	busy     bool   // a Save As is going on
	notice   string
	errMsg   string
}

type recoveryItem struct {
	e                     autorecover.Entry
	open, saveAs, discard widget.Clickable
}

// NewRecovery makes the window's state.
func NewRecovery(cfg RecoveryConfig) *Recovery {
	r := &Recovery{cfg: cfg, th: NewTheme(cfg.Fonts, ExcelAccent, colors.RGB(0x7A4300))}
	r.calls = NewCallQueue(cfg.Wake)
	for _, e := range cfg.Entries {
		r.items = append(r.items, recoveryItem{e: e})
	}
	r.list.Axis = layout.Vertical
	return r
}

// Entries are the copies still listed.
func (r *Recovery) Entries() []autorecover.Entry {
	out := make([]autorecover.Entry, len(r.items))
	for i, it := range r.items {
		out[i] = it.e
	}
	return out
}

// Release lets the copies still listed go (autorecover.Release), for the
// next start, when the window has closed.
func (r *Recovery) Release() {
	for _, it := range r.items {
		autorecover.Release(it.e)
	}
	r.items = nil
}

// Open opens item i in its app and drops it from the list.
func (r *Recovery) Open(i int) {
	e := r.items[i].e
	r.remove(i)
	r.cfg.Open(e)
	r.closeIfDone()
}

// Discard deletes item i's copy.
func (r *Recovery) Discard(i int) {
	autorecover.Discard(r.items[i].e)
	r.remove(i)
	r.closeIfDone()
}

func (r *Recovery) remove(i int) {
	r.items = slices.Delete(r.items, i, i+1)
	r.errMsg, r.notice = "", ""
}

// closeIfDone closes the window once every copy was dealt with.
func (r *Recovery) closeIfDone() {
	if len(r.items) == 0 {
		r.Close()
	}
}

// Close closes the window, keeping what is left for the next start.
func (r *Recovery) Close() {
	if r.cfg.Close != nil {
		r.cfg.Close()
	}
}

// SaveAs asks where to save item i and writes it there.
func (r *Recovery) SaveAs(i int) {
	if r.busy {
		return
	}
	r.busy = true
	e := r.items[i].e
	name, filters := RecoverySaveName(e)
	go func() {
		path, err := r.Files.Save(name, filters...)
		if errors.Is(err, ErrNoFileDialog) && r.cfg.Create != nil {
			// No native dialog: the fallback, which creates the file.
			wc, cerr := r.cfg.Create(filepath.Base(name))
			err = cerr
			if f, ok := wc.(*os.File); ok && err == nil {
				path = f.Name()
			}
			if wc != nil {
				wc.Close()
			}
			if errors.Is(err, r.cfg.Declined) {
				err = ErrCancelled
			}
		}
		if err == nil {
			err = r.cfg.SaveAs(e, path)
		}
		r.calls.Post(func() {
			r.busy = false
			switch {
			case errors.Is(err, ErrCancelled):
			case err != nil:
				r.errMsg = "Save failed: " + err.Error()
			default:
				autorecover.Discard(e)
				if j := slices.IndexFunc(r.items, func(it recoveryItem) bool { return it.e.Sidecar == e.Sidecar }); j >= 0 {
					r.remove(j)
				}
				r.notice = "Saved " + path
				r.closeIfDone()
			}
		})
	}()
}

// RecoverySaveName is the name Save As suggests for a recovered copy (its
// original folder and name, in its original format where OfficeLite
// writes it), and the dialog's file types, that format's first.
func RecoverySaveName(e autorecover.Entry) (string, []FileFilter) {
	var filters []FileFilter
	if e.App == autorecover.Word {
		filters = []FileFilter{
			{Name: "Word Document", Exts: []string{".docx"}},
			{Name: "Word 97-2003 Document", Exts: []string{".doc"}},
			{Name: "Plain Text", Exts: []string{".txt"}},
		}
	} else {
		filters = []FileFilter{
			{Name: "Excel Workbook", Exts: []string{".xlsx"}},
			{Name: "Excel 97-2003 Workbook", Exts: []string{".xls"}},
			{Name: "CSV (Comma delimited)", Exts: []string{".csv"}},
			{Name: "Text (Tab delimited)", Exts: []string{".tsv"}},
		}
	}
	ext := autorecover.Ext(e.App)
	format := strings.ToLower(e.Format)
	if j := slices.IndexFunc(filters, func(f FileFilter) bool { return f.Exts[0] == format }); j >= 0 {
		ext = format
		first := filters[j]
		filters = append([]FileFilter{first}, slices.Delete(filters, j, j+1)...)
	}
	name := strings.TrimSuffix(e.Name, filepath.Ext(e.Name)) + ext
	if e.Path != "" {
		name = filepath.Join(filepath.Dir(e.Path), name)
	}
	return name, append(filters, AllFilter)
}

// Layout draws the window.
func (r *Recovery) Layout(gtx layout.Context) layout.Dimensions {
	th := r.th
	for r.closeBtn.Clicked(gtx) {
		r.Close()
	}
	for i := 0; i < len(r.items); i++ {
		it := &r.items[i]
		switch {
		case it.open.Clicked(gtx):
			r.Open(i)
			i--
		case it.saveAs.Clicked(gtx):
			r.SaveAs(i)
		case it.discard.Clicked(gtx):
			r.asking = i
			r.ask.Show("Discard", "Delete the recovered copy of "+it.e.Name+"? This can't be undone.", "Discard", "Cancel")
		}
	}
	if r.ask.Clicked(gtx) == 0 && r.asking < len(r.items) {
		r.Discard(r.asking)
	}
	size := gtx.Constraints.Max
	FillRect(gtx.Ops, image.Rectangle{Max: size}, colors.White)
	now := time.Now()
	rows := []layout.Widget{
		th.Title("Document Recovery"),
		th.Paragraph("OfficeLite closed before these documents were saved. Open the ones you want "+
			"and save them; Close keeps the rest until the next start (for up to 30 days).", ChromeText),
		layout.Spacer{Height: 12}.Layout,
	}
	for i := range r.items {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions { return r.itemRow(gtx, &r.items[i], now) })
	}
	if len(r.items) == 0 {
		rows = append(rows, th.Paragraph("No documents left to recover.", MutedText))
	}
	if r.errMsg != "" {
		rows = append(rows, th.Paragraph(r.errMsg, ErrorText))
	} else if r.notice != "" {
		rows = append(rows, th.Paragraph(r.notice, MutedText))
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 20, Left: 28, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.List(th.Theme, &r.list).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
					return layout.Inset{Right: 8, Bottom: 2}.Layout(gtx, rows[i])
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8, Bottom: 16, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions { return th.Button(gtx, &r.closeBtn, "Close") }),
				)
			})
		}),
	)
	r.ask.Layout(gtx, th)
	return dims
}

// Run runs the work posted to the window's goroutine; call it every frame.
func (r *Recovery) Run() { r.calls.Run() }

func (r *Recovery) itemRow(gtx layout.Context, it *recoveryItem, now time.Time) layout.Dimensions {
	th := r.th
	kind, app := recent.Spreadsheet, "ExcelLite"
	if it.e.App == autorecover.Word {
		kind, app = recent.Document, "WordLite"
	}
	where := "Never saved"
	if it.e.Path != "" {
		where = it.e.Path
	}
	return layout.Inset{Top: 8, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				sz := image.Pt(gtx.Dp(26), gtx.Dp(32))
				gtx.Constraints = layout.Exact(sz)
				FillRect(gtx.Ops, image.Rectangle{Max: sz}, ChromeLine)
				off := op.Offset(image.Pt(1, 1)).Push(gtx.Ops)
				g := gtx
				g.Constraints = layout.Exact(sz.Sub(image.Pt(2, 2)))
				Thumbnail(g, kind, kindAccent(kind, th))
				off.Pop()
				return layout.Dimensions{Size: sz}
			}),
			layout.Rigid(layout.Spacer{Width: 14}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(th.Label(14, it.e.Name, ChromeText, font.SemiBold).Layout),
					layout.Rigid(th.Label(12, app+" · recovered "+When(it.e.Saved, now), MutedText, font.Normal).Layout),
					layout.Rigid(th.Label(12, where, MutedText, font.Normal).Layout),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return th.Button(gtx, &it.open, "Open")
			}),
			layout.Rigid(layout.Spacer{Width: 6}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if r.busy {
					gtx = gtx.Disabled()
				}
				return th.SecondaryButton(gtx, &it.saveAs, "Save As…")
			}),
			layout.Rigid(layout.Spacer{Width: 6}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return th.SecondaryButton(gtx, &it.discard, "Discard")
			}),
		)
	})
}
