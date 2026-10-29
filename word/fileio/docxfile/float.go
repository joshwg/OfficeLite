package docxfile

import (
	"strconv"
	"strings"

	"officelite/word/model"
)

// Floating objects: a w:drawing holding a wp:anchor (usually inside
// mc:AlternateContent, whose mc:Fallback is the VML version), or a VML
// w:pict whose shape is positioned absolutely. The anchor's position,
// size and wrapping, the shape's geometry, fill and outline, and a text
// box's content become a model.Floating; the original XML is kept to be
// written back. Objects from other formats are written as DrawingML
// anchors (wps shapes, text boxes and pictures).

// floatOf reads the floating object of a w:drawing, mc:AlternateContent
// or w:pict element, or returns nil when it is inline.
func (r *reader) floatOf(k *node) *model.Floating {
	if a := k.find("wp:anchor"); a != nil {
		return r.anchorFloat(a)
	}
	if k.find("wp:inline") != nil {
		return nil
	}
	for _, n := range []string{"v:shape", "v:rect", "v:roundrect", "v:oval", "v:line"} {
		if s := k.find(n); s != nil && strings.Contains(strings.ReplaceAll(s.attr("style"), " ", ""), "position:absolute") {
			return r.vmlFloat(s)
		}
	}
	return nil
}

func emuPt(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v / 12700
}

var relFroms = map[string]model.FloatRel{
	"column": model.RelColumn, "paragraph": model.RelParagraph, "page": model.RelPage, "margin": model.RelMargin,
	"line": model.RelLine, "character": model.RelCharacter, "text": model.RelColumn,
	"leftMargin": model.RelPage, "rightMargin": model.RelPage, "insideMargin": model.RelPage, "outsideMargin": model.RelPage,
	"topMargin": model.RelPage, "bottomMargin": model.RelPage,
}

// anchorFloat reads a wp:anchor.
func (r *reader) anchorFloat(a *node) *model.Floating {
	f := &model.Floating{RelX: model.RelColumn, RelY: model.RelParagraph, Inset: model.DefaultInset}
	f.Distance = [4]float64{emuPt(a.attr("distL")), emuPt(a.attr("distT")), emuPt(a.attr("distR")), emuPt(a.attr("distB"))}
	for _, axis := range []struct {
		name  string
		rel   *model.FloatRel
		off   *float64
		align *string
	}{{"wp:positionH", &f.RelX, &f.X, &f.AlignX}, {"wp:positionV", &f.RelY, &f.Y, &f.AlignY}} {
		p := a.child(axis.name)
		if p == nil {
			continue
		}
		if rel, ok := relFroms[p.attr("relativeFrom")]; ok {
			*axis.rel = rel
		}
		if o := p.child("wp:posOffset"); o != nil {
			*axis.off = emuPt(strings.TrimSpace(o.text))
		}
		if al := p.child("wp:align"); al != nil {
			*axis.align = strings.TrimSpace(al.text)
		}
	}
	if ext := a.child("wp:extent"); ext != nil {
		f.W, f.H = emuPt(ext.attr("cx")), emuPt(ext.attr("cy"))
	}
	switch {
	case a.child("wp:wrapSquare") != nil:
		f.Wrap = model.WrapSquare
	case a.child("wp:wrapTight") != nil, a.child("wp:wrapThrough") != nil:
		f.Wrap = model.WrapTight
	case a.child("wp:wrapTopAndBottom") != nil:
		f.Wrap = model.WrapTopBottom
	case onOffAttr(a.attr("behindDoc")):
		f.Wrap = model.WrapBehind
	}
	if d := a.child("wp:docPr"); d != nil {
		f.Name = d.attr("name")
	}
	if sp := a.find("wps:spPr"); sp != nil {
		r.shapeProps(f, sp, a.find("wps:style"))
		if bp := a.find("wps:bodyPr"); bp != nil {
			for i, n := range []string{"lIns", "tIns", "rIns", "bIns"} {
				if v := bp.attr(n); v != "" {
					f.Inset[i] = emuPt(v)
				}
			}
		}
	}
	if pic := a.find("pic:pic"); pic != nil {
		if blip := pic.find("a:blip"); blip != nil {
			f.Picture = r.picture(blip.attr("r:embed"))
		}
		if sp := pic.child("pic:spPr"); sp != nil {
			r.shapeProps(f, sp, nil)
			f.Fill = ""
		}
		if f.Picture == nil {
			r.warn(warnPictures)
			f.Fill, f.Line = "F2F2F2", "BFBFBF"
		}
	}
	if tb := a.find("w:txbxContent"); tb != nil {
		f.Blocks = r.partBlocks("", tb)
	} else if f.Picture == nil && a.find("wps:wsp") == nil {
		// A chart, SmartArt or group: shown as a placeholder box.
		r.warn(warnPictures)
		f.Fill, f.Line = "F2F2F2", "BFBFBF"
	}
	return f
}

// shapeProps reads a shape's geometry, fill and outline (falling back to
// the theme colors its style refers to).
func (r *reader) shapeProps(f *model.Floating, sp, style *node) {
	if g := sp.child("a:prstGeom"); g != nil {
		f.Shape = g.attr("prst")
	}
	if x := sp.child("a:xfrm"); x != nil {
		f.FlipH, f.FlipV = onOffAttr(x.attr("flipH")), onOffAttr(x.attr("flipV"))
	}
	isLine := f.Shape == "line" || strings.Contains(f.Shape, "Connector")
	switch {
	case sp.child("a:noFill") != nil:
	case sp.child("a:solidFill") != nil:
		f.Fill = r.drawingColor(sp.child("a:solidFill"))
	case style != nil && !isLine:
		if ref := style.child("a:fillRef"); ref != nil && ref.attr("idx") != "0" {
			f.Fill = r.drawingColor(ref)
		}
	}
	ln := sp.child("a:ln")
	switch {
	case ln != nil && ln.child("a:noFill") != nil:
	case ln != nil && ln.child("a:solidFill") != nil:
		f.Line = r.drawingColor(ln.child("a:solidFill"))
	case style != nil:
		if ref := style.child("a:lnRef"); ref != nil && ref.attr("idx") != "0" {
			f.Line = r.drawingColor(ref)
		}
	}
	f.LineWidth = 0.75
	if ln != nil && ln.attr("w") != "" {
		f.LineWidth = emuPt(ln.attr("w"))
	}
}

// drawingColor reads the color inside a fill element: an RGB value, a
// preset or a theme color.
func (r *reader) drawingColor(n *node) string {
	if c := n.child("a:srgbClr"); c != nil {
		return strings.ToUpper(c.attr("val"))
	}
	if c := n.child("a:prstClr"); c != nil {
		switch c.attr("val") {
		case "white":
			return "FFFFFF"
		case "red":
			return "FF0000"
		case "green":
			return "00FF00"
		case "blue":
			return "0000FF"
		}
		return "000000"
	}
	if c := n.child("a:schemeClr"); c != nil {
		v := c.attr("val")
		alias := map[string]string{"tx1": "text1", "bg1": "background1", "tx2": "text2", "bg2": "background2",
			"dk1": "dark1", "lt1": "light1", "dk2": "dark2", "lt2": "light2"}
		if a, ok := alias[v]; ok {
			v = a
		}
		if r.th != nil {
			if col := r.th.colors[v]; col != "" {
				return col
			}
		}
		switch v {
		case "light1", "background1":
			return "FFFFFF"
		case "accent1":
			return "4472C4"
		}
		return "000000"
	}
	if c := n.child("a:sysClr"); c != nil {
		return strings.ToUpper(c.attr("lastClr"))
	}
	return ""
}

// vmlFloat reads an absolutely positioned VML shape (a text box from an
// older Word).
func (r *reader) vmlFloat(s *node) *model.Floating {
	f := &model.Floating{RelX: model.RelColumn, RelY: model.RelParagraph, Inset: model.DefaultInset, Fill: "FFFFFF", Line: "000000", LineWidth: 0.75}
	switch s.name {
	case "v:oval":
		f.Shape = "ellipse"
	case "v:roundrect":
		f.Shape = "roundRect"
	case "v:line":
		f.Shape = "line"
		f.Fill = ""
	}
	style := map[string]string{}
	for _, d := range strings.Split(s.attr("style"), ";") {
		if k, v, ok := strings.Cut(d, ":"); ok {
			style[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	f.X, f.Y = cssPt(style["margin-left"]), cssPt(style["margin-top"])
	f.W, f.H = cssPt(style["width"]), cssPt(style["height"])
	vrel := map[string]model.FloatRel{"text": model.RelColumn, "page": model.RelPage, "margin": model.RelMargin, "char": model.RelCharacter, "line": model.RelLine}
	if v, ok := vrel[style["mso-position-horizontal-relative"]]; ok {
		f.RelX = v
	}
	if v, ok := vrel[style["mso-position-vertical-relative"]]; ok {
		f.RelY = v
		if v == model.RelColumn {
			f.RelY = model.RelParagraph
		}
	}
	if s.attr("filled") == "f" || s.attr("filled") == "false" {
		f.Fill = ""
	} else if c := vmlColor(s.attr("fillcolor")); c != "" {
		f.Fill = c
	}
	if s.attr("stroked") == "f" || s.attr("stroked") == "false" {
		f.Line = ""
	} else if c := vmlColor(s.attr("strokecolor")); c != "" {
		f.Line = c
	}
	if w := s.attr("strokeweight"); w != "" {
		f.LineWidth = cssPt(w)
	}
	if strings.Contains(style["z-index"], "-") {
		f.Wrap = model.WrapBehind
	}
	if tb := s.find("w:txbxContent"); tb != nil {
		f.Blocks = r.partBlocks("", tb)
	}
	if im := s.find("v:imagedata"); im != nil {
		f.Picture = r.picture(im.attr("r:id"))
	}
	return f
}

// cssPt converts a CSS length ("12pt", "1.5in", "20px") to points.
func cssPt(s string) float64 {
	m := reStylePt.FindStringSubmatch("width:" + s)
	if m == nil {
		return 0
	}
	v, _ := strconv.ParseFloat(m[2], 64)
	switch strings.ToLower(m[3]) {
	case "in":
		v *= 72
	case "px":
		v *= 0.75
	case "cm":
		v *= 72 / 2.54
	case "mm":
		v *= 72 / 25.4
	case "":
		v /= 12700 // EMU
	}
	return v
}

// vmlColor converts a VML color ("#RRGGBB", "black", "white [3212]").
func vmlColor(s string) string {
	s = strings.TrimSpace(strings.Split(s, " ")[0])
	if strings.HasPrefix(s, "#") && len(s) == 7 {
		return strings.ToUpper(s[1:])
	}
	switch s {
	case "white":
		return "FFFFFF"
	case "black":
		return "000000"
	}
	return ""
}

// ---- writing ----

var relNames = map[model.FloatRel]string{}

func init() {
	for i, n := range model.FloatRelNames {
		relNames[model.FloatRel(i)] = n
	}
}

func ptEMU(v float64) string { return strconv.FormatInt(int64(v*12700+0.5*sign(v)), 10) }

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// floatRun writes a floating object from another format as a DrawingML
// anchor: a wps shape (with its text box content) or a picture.
func (w *writer) floatRun(b *strings.Builder, r *model.Run) {
	f := r.Opaque.Float
	id := w.nextDocPrID()
	b.WriteString("<w:r>")
	if rpr := rPrXML(r.Style, r.Props, w.th); rpr != "" {
		b.WriteString("<w:rPr>" + rpr + "</w:rPr>")
	}
	d := f.Distance
	behind := "0"
	if f.Wrap == model.WrapBehind {
		behind = "1"
	}
	b.WriteString(`<w:drawing><wp:anchor distT="` + ptEMU(d[1]) + `" distB="` + ptEMU(d[3]) + `" distL="` + ptEMU(d[0]) + `" distR="` + ptEMU(d[2]) +
		`" simplePos="0" relativeHeight="` + strconv.Itoa(251659264+id) + `" behindDoc="` + behind + `" locked="0" layoutInCell="1" allowOverlap="1"><wp:simplePos x="0" y="0"/>`)
	pos := func(tag string, rel model.FloatRel, off float64, align string) {
		b.WriteString("<wp:" + tag + ` relativeFrom="` + relNames[rel] + `">`)
		if align != "" {
			b.WriteString("<wp:align>" + escText(align) + "</wp:align>")
		} else {
			b.WriteString("<wp:posOffset>" + ptEMU(off) + "</wp:posOffset>")
		}
		b.WriteString("</wp:" + tag + ">")
	}
	relY := f.RelY
	if relY == model.RelColumn {
		relY = model.RelParagraph
	}
	relX := f.RelX
	if relX == model.RelParagraph || relX == model.RelLine {
		relX = model.RelColumn
	}
	pos("positionH", relX, f.X, f.AlignX)
	pos("positionV", relY, f.Y, f.AlignY)
	b.WriteString(`<wp:extent cx="` + ptEMU(f.W) + `" cy="` + ptEMU(f.H) + `"/><wp:effectExtent l="0" t="0" r="0" b="0"/>`)
	switch f.Wrap {
	case model.WrapSquare:
		b.WriteString(`<wp:wrapSquare wrapText="bothSides"/>`)
	case model.WrapTight:
		b.WriteString(`<wp:wrapTight wrapText="bothSides"><wp:wrapPolygon edited="0"><wp:start x="0" y="0"/><wp:lineTo x="0" y="21600"/><wp:lineTo x="21600" y="21600"/><wp:lineTo x="21600" y="0"/><wp:lineTo x="0" y="0"/></wp:wrapPolygon></wp:wrapTight>`)
	case model.WrapTopBottom:
		b.WriteString(`<wp:wrapTopAndBottom/>`)
	default:
		b.WriteString(`<wp:wrapNone/>`)
	}
	name := f.Name
	if name == "" {
		name = "Shape " + strconv.Itoa(id)
	}
	b.WriteString(`<wp:docPr id="` + strconv.Itoa(id) + `" name="` + escAttr(name) + `"/><wp:cNvGraphicFramePr/>`)
	b.WriteString(`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">`)
	spPr := func(fill string) string {
		geom := f.Shape
		if geom == "" {
			geom = "rect"
		}
		s := `<a:xfrm`
		if f.FlipH {
			s += ` flipH="1"`
		}
		if f.FlipV {
			s += ` flipV="1"`
		}
		s += `><a:off x="0" y="0"/><a:ext cx="` + ptEMU(f.W) + `" cy="` + ptEMU(f.H) + `"/></a:xfrm><a:prstGeom prst="` + escAttr(geom) + `"><a:avLst/></a:prstGeom>`
		if fill != "" {
			s += `<a:solidFill><a:srgbClr val="` + escAttr(fill) + `"/></a:solidFill>`
		} else {
			s += `<a:noFill/>`
		}
		if f.Line != "" {
			s += `<a:ln w="` + ptEMU(max(f.LineWidth, 0.25)) + `"><a:solidFill><a:srgbClr val="` + escAttr(f.Line) + `"/></a:solidFill></a:ln>`
		} else {
			s += `<a:ln><a:noFill/></a:ln>`
		}
		return s
	}
	if f.Picture != nil {
		rid := w.pictureRel(f.Picture)
		b.WriteString(`<a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
			`<pic:nvPicPr><pic:cNvPr id="` + strconv.Itoa(id) + `" name="` + escAttr(name) + `"/><pic:cNvPicPr/></pic:nvPicPr>` +
			`<pic:blipFill><a:blip r:embed="` + escAttr(rid) + `"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr>`)
		b.WriteString(spPr("") + `</pic:spPr></pic:pic></a:graphicData>`)
	} else {
		b.WriteString(`<a:graphicData uri="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"><wps:wsp>`)
		if f.IsTextBox() {
			b.WriteString(`<wps:cNvSpPr txBox="1"/>`)
		} else {
			b.WriteString(`<wps:cNvSpPr/>`)
		}
		b.WriteString(`<wps:spPr>` + spPr(f.Fill) + `</wps:spPr>`)
		if f.IsTextBox() {
			b.WriteString(`<wps:txbx><w:txbxContent>`)
			blocks := f.Blocks
			if len(blocks) == 0 {
				blocks = []model.Block{&model.Paragraph{}}
			} else if _, ok := blocks[len(blocks)-1].(*model.Paragraph); !ok {
				blocks = append(blocks[:len(blocks):len(blocks)], &model.Paragraph{})
			}
			w.blocks(b, blocks)
			b.WriteString(`</w:txbxContent></wps:txbx>`)
		}
		in := f.Inset
		b.WriteString(`<wps:bodyPr rot="0" vert="horz" wrap="square" lIns="` + ptEMU(in[0]) + `" tIns="` + ptEMU(in[1]) + `" rIns="` + ptEMU(in[2]) + `" bIns="` + ptEMU(in[3]) +
			`" anchor="t" anchorCtr="0"><a:noAutofit/></wps:bodyPr></wps:wsp></a:graphicData>`)
	}
	b.WriteString(`</a:graphic></wp:anchor></w:drawing></w:r>`)
}

// fieldChar writes a field character kept from another format (a .doc
// field whose result spans paragraphs) as complex field runs.
func (w *writer) fieldChar(b *strings.Builder, r *model.Run) {
	o := r.Opaque
	rpr := ""
	if x := rPrXML(r.Style, r.Props, w.th); x != "" {
		rpr = "<w:rPr>" + x + "</w:rPr>"
	}
	char := func(t string) { b.WriteString("<w:r>" + rpr + `<w:fldChar w:fldCharType="` + t + `"/></w:r>`) }
	instr := func() {
		b.WriteString("<w:r>" + rpr + `<w:instrText xml:space="preserve"> ` + escText(o.Field) + ` </w:instrText></w:r>`)
	}
	switch o.FieldChar {
	case model.FieldBegin:
		char("begin")
	case model.FieldInstr:
		instr()
	case model.FieldSep:
		char("separate")
	case model.FieldEnd:
		char("end")
	case model.FieldStart:
		char("begin")
		instr()
		char("separate")
	}
}
