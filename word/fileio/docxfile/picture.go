package docxfile

import (
	"regexp"
	"strconv"
	"strings"

	"officelite/word/model"
)

// Pictures inserted in WordLite (or whose original XML can't be kept) are
// written as inline DrawingML pictures with their image in a new media
// part.

const relImage = "/image"

// media is an image part added to the package.
type media struct {
	part  string // e.g. "word/media/image3.png"
	data  []byte
	ext   string
	ctype string
}

var reDocPrID = regexp.MustCompile(`<wp:docPr\b[^>]*\bid="(\d+)"`)

// pictureRel returns the relationship id of the media part holding pic,
// adding the part on first use.
func (w *writer) pictureRel(pic *model.Picture) string {
	if id, ok := w.picRels[pic]; ok {
		return id
	}
	if w.picRels == nil {
		w.picRels = map[*model.Picture]string{}
	}
	ext := pic.Ext
	if ext == "" {
		ext = "png"
	}
	dir := "word"
	if w.src != nil {
		dir = dirOf(w.main)
	}
	var name, part string
	for n := len(w.media) + 1; ; n++ {
		name = "media/image" + strconv.Itoa(n) + "." + ext
		part = name
		if dir != "" {
			part = dir + "/" + name
		}
		if (w.src == nil || w.src.find(part) == nil) && !w.hasMedia(part) {
			break
		}
	}
	w.media = append(w.media, media{part: part, data: pic.Data, ext: ext, ctype: pic.ContentType()})
	id := w.newRelID()
	w.rels = append(w.rels, rel{ID: id, Type: relTypePrefix + relImage, Target: name})
	w.relsDirty = true
	w.picRels[pic] = id
	return id
}

func (w *writer) hasMedia(part string) bool {
	for _, m := range w.media {
		if m.part == part {
			return true
		}
	}
	return false
}

func dirOf(part string) string {
	if i := strings.LastIndexByte(part, '/'); i >= 0 {
		return part[:i]
	}
	return ""
}

// nextDocPrID returns a drawing id not used elsewhere in the document.
func (w *writer) nextDocPrID() int {
	if w.docPrID == 0 {
		w.docPrID = 1000
		if w.src != nil {
			if data, err := w.src.read(w.main); err == nil {
				for _, m := range reDocPrID.FindAllSubmatch(data, -1) {
					if v, err := strconv.Atoi(string(m[1])); err == nil && v >= w.docPrID {
						w.docPrID = v + 1
					}
				}
			}
		}
	}
	w.docPrID++
	return w.docPrID
}

// pictureRun writes a picture run as <w:drawing><wp:inline>.
func (w *writer) pictureRun(b *strings.Builder, r *model.Run) {
	o := r.Opaque
	rid := w.pictureRel(o.Picture)
	id := strconv.Itoa(w.nextDocPrID())
	cx := strconv.FormatInt(int64(o.Width*12700+0.5), 10)
	cy := strconv.FormatInt(int64(o.Height*12700+0.5), 10)
	name := o.Picture.Name
	if name == "" {
		name = "Picture " + id
	}
	b.WriteString("<w:r>")
	if rpr := rPrXML(r.Style, r.Props, w.th); rpr != "" {
		b.WriteString("<w:rPr>" + rpr + "</w:rPr>")
	}
	b.WriteString(`<w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0">` +
		`<wp:extent cx="` + cx + `" cy="` + cy + `"/><wp:effectExtent l="0" t="0" r="0" b="0"/>` +
		`<wp:docPr id="` + id + `" name="Picture ` + id + `" descr="` + escAttr(name) + `"/>` +
		`<wp:cNvGraphicFramePr><a:graphicFrameLocks xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" noChangeAspect="1"/></wp:cNvGraphicFramePr>` +
		`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">` +
		`<a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
		`<pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
		`<pic:nvPicPr><pic:cNvPr id="0" name="` + escAttr(name) + `"/><pic:cNvPicPr/></pic:nvPicPr>` +
		`<pic:blipFill><a:blip r:embed="` + rid + `"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>` +
		`<pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="` + cx + `" cy="` + cy + `"/></a:xfrm>` +
		`<a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r>`)
}

// addMedia adds the new media parts to a package being written: their
// data to parts and a default content type for each new extension.
func (w *writer) addMedia(ct *contentTypes, parts map[string][]byte) bool {
	changed := false
	for _, m := range w.media {
		parts[m.part] = m.data
		found := false
		for _, d := range ct.defaults {
			if strings.EqualFold(d[0], m.ext) {
				found = true
			}
		}
		if !found {
			ct.defaults = append(ct.defaults, [2]string{m.ext, m.ctype})
			changed = true
		}
	}
	return changed
}
