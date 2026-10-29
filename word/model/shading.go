package model

import "slices"

// Home > Paragraph > Shading: one button that colors the background of
// the selected text, the selected paragraphs or the selected table cells,
// whichever the selection is.

// ShadeTarget is what SetShading colors.
type ShadeTarget int8

const (
	ShadeText      ShadeTarget = iota // character shading on the selected runs
	ShadeParagraph                    // paragraph shading (w:pPr/w:shd)
	ShadeCells                        // cell shading (w:tcPr/w:shd)
)

// ShadingTarget says what Shading applies to, as Word decides: the cells
// when the selection covers whole table cells (or crosses from one cell
// into another), the text when it is part of one paragraph, and otherwise
// the paragraphs (the caret's paragraph with nothing selected).
func (e *Editor) ShadingTarget() ShadeTarget {
	if e.Sel.Empty() {
		return ShadeParagraph
	}
	s, t := e.Sel.Ordered()
	cells := map[*TableCell]bool{}
	for _, i := range e.shadedParas() {
		cells[e.locs[i].Cell] = true
	}
	if len(cells) > 1 {
		return ShadeCells
	}
	if c := e.locs[s.Para].Cell; c != nil {
		if a, b, ok := e.CellBounds(c); ok && s == a && t == b {
			return ShadeCells
		}
	}
	if s.Para == t.Para && (s.Off > 0 || t.Off < e.locs[s.Para].P.Len()) {
		return ShadeText
	}
	return ShadeParagraph
}

// SetShading colors the background behind the selection (RRGGBB; "" is
// No Color) as one undo step "Shading"; see ShadingTarget. With cells
// selected, paragraphs selected outside the table get paragraph shading.
func (e *Editor) SetShading(hex string) {
	defer e.Op("Shading")()
	switch e.ShadingTarget() {
	case ShadeText:
		e.FormatRuns(func(r *Run) { r.Props.Shading = hex })
	case ShadeParagraph:
		e.FormatParas(func(p *Paragraph) { e.shadePara(p, hex) })
	case ShadeCells:
		from, to := e.span()
		e.edit("format", from, to, false, func() {
			for _, i := range e.shadedParas() {
				if c := e.locs[i].Cell; c != nil {
					c.Shading = hex
				} else {
					e.shadePara(e.locs[i].P, hex)
				}
			}
		})
	}
}

// shadedParas is SelectedParas, with an empty cell paragraph at the end
// of the selection (selecting up to an empty last cell selects the cell).
func (e *Editor) shadedParas() []int {
	ps := e.SelectedParas()
	if _, t := e.Sel.Ordered(); !slices.Contains(ps, t.Para) && e.locs[t.Para].Cell != nil && e.locs[t.Para].P.Len() == 0 {
		ps = append(ps, t.Para)
	}
	return ps
}

// shadePara sets a paragraph's shading; No Color over a shaded style is
// "auto", which hides the style's.
func (e *Editor) shadePara(p *Paragraph, hex string) {
	p.Props.Shading = hex
	if hex == "" {
		if s := e.Doc.ResolvePara(p).Shading; s != "" && s != "auto" {
			p.Props.Shading = "auto"
		}
	}
}
