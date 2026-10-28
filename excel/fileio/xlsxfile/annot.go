package xlsxfile

import (
	"encoding/xml"
	"path"
	"slices"
	"strings"

	"github.com/xuri/excelize/v2"

	"officelite/excel/model"
)

// Notes (legacy comments, xl/commentsN.xml) and hyperlinks (<hyperlink>
// in the worksheet, external targets in its relationships).

// xLink is a <hyperlink> element.
type xLink struct {
	ref, rid, location, tooltip string
}

func xLinkOf(t xml.StartElement) xLink {
	return xLink{ref: attr(t, "ref"), rid: attr(t, "id"), location: attr(t, "location"), tooltip: attr(t, "tooltip")}
}

type xComments struct {
	Authors []string `xml:"authors>author"`
	List    []struct {
		Ref      string `xml:"ref,attr"`
		AuthorID int    `xml:"authorId,attr"`
		Text     struct {
			T string `xml:"t"`
			R []struct {
				T string `xml:"t"`
			} `xml:"r"`
		} `xml:"text"`
	} `xml:"commentList>comment"`
}

// maxLinkCells bounds how many cells one <hyperlink ref="A1:Z9999"> fills.
const maxLinkCells = 10000

// readAnnotations reads the notes and resolves the hyperlinks of the
// worksheet part (read just before, its links in r.links).
func (r *reader) readAnnotations(part string, sh *model.Sheet) {
	rels, _ := r.rels(relsPath(part), path.Dir(part))
	byID := map[string]xRel{}
	for _, rel := range rels {
		byID[rel.ID] = rel
		if strings.HasSuffix(rel.Type, "/comments") {
			r.readComments(rel.Target, sh)
		}
	}
	for _, l := range r.links {
		ref := l.ref
		if !strings.Contains(ref, ":") {
			ref += ":" + ref
		}
		rng, ok := parseRange(ref)
		if !ok {
			continue
		}
		h := model.Hyperlink{Location: l.location, Tooltip: l.tooltip}
		if l.rid != "" {
			if rel, ok := byID[l.rid]; ok {
				h.Target = rel.Target
			}
		}
		if h.Target == "" && h.Location == "" {
			continue
		}
		n := 0
		for row := rng.R1; row <= min(rng.R2, model.MaxRows-1); row++ {
			for col := rng.C1; col <= min(rng.C2, model.MaxCols-1) && n < maxLinkCells; col++ {
				sh.SetLink(row, col, &h)
				n++
			}
		}
	}
}

func (r *reader) readComments(name string, sh *model.Sheet) {
	var c xComments
	if err := r.unmarshal(name, &c); err != nil {
		r.wb.Warnings = append(r.wb.Warnings, sh.Name+": notes could not be read")
		return
	}
	for _, cm := range c.List {
		row, col, ok := parseCellRef(cm.Ref)
		if !ok || row >= model.MaxRows || col >= model.MaxCols {
			continue
		}
		var b strings.Builder
		b.WriteString(cm.Text.T)
		for _, run := range cm.Text.R {
			b.WriteString(run.T)
		}
		n := model.Note{Text: strings.ReplaceAll(b.String(), "\r\n", "\n")}
		if cm.AuthorID >= 0 && cm.AuthorID < len(c.Authors) {
			n.Author = c.Authors[cm.AuthorID]
		}
		sh.SetNote(row, col, &n)
	}
}

// writeAnnotations adds the sheet's notes and hyperlinks. It runs before
// the sheet's StreamWriter is created, which keeps them.
func (x *writer) writeAnnotations(sh *model.Sheet, name string) error {
	for _, k := range sortedKeys(sh.Notes) {
		n := sh.Notes[k]
		author := n.Author
		if author == "" {
			author = "Author"
		}
		if err := x.f.AddComment(name, excelize.Comment{Author: author, Cell: model.CellName(k[0], k[1]), Text: n.Text}); err != nil {
			return err
		}
	}
	for _, k := range sortedKeys(sh.Links) {
		h := sh.Links[k]
		var opts excelize.HyperlinkOpts
		if h.Tooltip != "" {
			opts.Tooltip = &h.Tooltip
		}
		link, typ := h.Target, "External"
		switch {
		case h.Target == "":
			link, typ = h.Location, "Location"
		case h.Location != "":
			link += "#" + h.Location
		}
		if err := x.f.SetCellHyperLink(name, model.CellName(k[0], k[1]), link, typ, opts); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys[V any](m map[[2]int]V) [][2]int {
	keys := make([][2]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b [2]int) int {
		if a[0] != b[0] {
			return a[0] - b[0]
		}
		return a[1] - b[1]
	})
	return keys
}
