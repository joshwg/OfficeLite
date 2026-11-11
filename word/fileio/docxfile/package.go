// The OPC package layer: opening the zip, reading parts within the size
// cap, relationships and [Content_Types].xml.

package docxfile

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"

	"officelite/common/opc"
	"officelite/word"
)

// Relationship types (matched by suffix so transitional and strict both work).
const (
	relOfficeDoc  = "/officeDocument"
	relStyles     = "/styles"
	relNumbering  = "/numbering"
	relSettings   = "/settings"
	relTheme      = "/theme"
	relHyperlink  = "/hyperlink"
	relVBA        = "/vbaProject"
	relVBAData    = "/wordVbaData"
	relToolbars   = "/attachedToolbars"
	relFootnotes  = "/footnotes"
	relEndnotes   = "/endnotes"
	relComments   = "/comments"
	relHeader     = "/header"
	relFooter     = "/footer"
	relTypePrefix = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
)

// pkg is an opened OPC (zip) package.
type pkg struct {
	zr    *zip.Reader
	files map[string]*zip.File // by part name without leading "/"
	// tooLarge is the first "part is too large" error any read hit; Read
	// reports it as an open failure even when the caller treated the part
	// as optional and ignored the error.
	tooLarge error
}

// openPkg opens the zip in data and indexes its entries by part name.
func openPkg(data []byte) (*pkg, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a valid .docx file: %w", err)
	}
	p := &pkg{zr: zr, files: map[string]*zip.File{}}
	for _, f := range zr.File {
		p.files[strings.TrimPrefix(f.Name, "/")] = f
	}
	return p, nil
}

// find returns the entry for part name, matched case-insensitively, or
// nil.
func (p *pkg) find(name string) *zip.File {
	if f, ok := p.files[name]; ok {
		return f
	}
	for n, f := range p.files { // part names are case-insensitive
		if strings.EqualFold(n, name) {
			return f
		}
	}
	return nil
}

// maxPartSize is the largest part read accepts: word.DocxMaxPartSize, a
// variable only so tests can lower it.
var maxPartSize int64 = word.DocxMaxPartSize

// read returns the contents of part name. A part larger than maxPartSize is
// an error (remembered in p.tooLarge); at most maxPartSize+1 bytes are read
// to tell.
func (p *pkg) read(name string) (b []byte, err error) {
	limit := maxPartSize
	f := p.find(name)
	if f == nil {
		return nil, fmt.Errorf("missing part %s", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rc.Close(); err == nil {
			err = closeErr
		}
	}()
	b, err = io.ReadAll(io.LimitReader(rc, limit+1))
	if err == nil && int64(len(b)) > limit {
		err = fmt.Errorf("the file is damaged or too large: part %s is larger than %s", name, sizeText(limit))
		if p.tooLarge == nil {
			p.tooLarge = err
		}
		return nil, err
	}
	return b, err
}

// sizeText formats a byte count for an error message ("1 GB", "512 bytes").
func sizeText(n int64) string {
	switch {
	case n >= 1<<30 && n%(1<<30) == 0:
		return fmt.Sprintf("%d GB", n>>30)
	case n >= 1<<20 && n%(1<<20) == 0:
		return fmt.Sprintf("%d MB", n>>20)
	}
	return fmt.Sprintf("%d bytes", n)
}

// rel is one <Relationship>; Mode is "External" for targets outside the
// package.
type rel struct {
	ID, Type, Target, Mode string
}

// is reports whether the relationship's type ends in suffix (one of the
// rel* constants).
func (r rel) is(suffix string) bool { return strings.HasSuffix(r.Type, suffix) }

// rels returns part's relationships ("" for the package's own), or nil
// when it has no readable .rels part.
func (p *pkg) rels(part string) []rel {
	data, err := p.read(opc.RelsPath(part))
	if err != nil {
		return nil
	}
	root, err := parseXML(data)
	if err != nil {
		return nil
	}
	var out []rel
	for _, k := range root.kids {
		if strings.HasSuffix(k.name, "Relationship") {
			out = append(out, rel{k.attr("Id"), k.attr("Type"), k.attr("Target"), k.attr("TargetMode")})
		}
	}
	return out
}

// mainPart is the main document part's name.
func (p *pkg) mainPart() string {
	for _, r := range p.rels("") {
		if r.is(relOfficeDoc) {
			return opc.Resolve("", r.Target)
		}
	}
	return "word/document.xml"
}

// relsXML encodes a .rels part.
func relsXML(rels []rel) []byte {
	var b strings.Builder
	b.WriteString(xmlHeader + `<Relationships xmlns="` + nsRel + `">`)
	for _, r := range rels {
		b.WriteString(`<Relationship Id="` + escAttr(r.ID) + `" Type="` + escAttr(r.Type) + `" Target="` + escAttr(r.Target) + `"`)
		if r.Mode != "" {
			b.WriteString(` TargetMode="` + escAttr(r.Mode) + `"`)
		}
		b.WriteString("/>")
	}
	b.WriteString("</Relationships>")
	return []byte(b.String())
}

// xmlHeader starts every part the writer generates.
const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"

// contentTypes is [Content_Types].xml.
type contentTypes struct {
	defaults  [][2]string // extension, type
	overrides [][2]string // part name ("/word/document.xml"), type
}

// parseContentTypes reads [Content_Types].xml; unparsable data gives an
// empty table.
func parseContentTypes(data []byte) *contentTypes {
	ct := &contentTypes{}
	root, err := parseXML(data)
	if err != nil {
		return ct
	}
	for _, k := range root.kids {
		switch {
		case strings.HasSuffix(k.name, "Default"):
			ct.defaults = append(ct.defaults, [2]string{k.attr("Extension"), k.attr("ContentType")})
		case strings.HasSuffix(k.name, "Override"):
			ct.overrides = append(ct.overrides, [2]string{k.attr("PartName"), k.attr("ContentType")})
		}
	}
	return ct
}

// xml encodes the table as [Content_Types].xml.
func (ct *contentTypes) xml() []byte {
	var b strings.Builder
	b.WriteString(xmlHeader + `<Types xmlns="` + nsCT + `">`)
	for _, d := range ct.defaults {
		b.WriteString(`<Default Extension="` + escAttr(d[0]) + `" ContentType="` + escAttr(d[1]) + `"/>`)
	}
	for _, o := range ct.overrides {
		b.WriteString(`<Override PartName="` + escAttr(o[0]) + `" ContentType="` + escAttr(o[1]) + `"/>`)
	}
	b.WriteString("</Types>")
	return []byte(b.String())
}

// setOverride gives part the content type typ, replacing its Override or
// adding one.
func (ct *contentTypes) setOverride(part, typ string) {
	name := "/" + part
	for i, o := range ct.overrides {
		if strings.EqualFold(o[0], name) {
			ct.overrides[i][1] = typ
			return
		}
	}
	ct.overrides = append(ct.overrides, [2]string{name, typ})
}

// typeOf is part's content type: its Override's, else the Default for
// its extension, else "".
func (ct *contentTypes) typeOf(part string) string {
	name := "/" + part
	for _, o := range ct.overrides {
		if strings.EqualFold(o[0], name) {
			return o[1]
		}
	}
	ext := path.Ext(part)
	for _, d := range ct.defaults {
		if ext != "" && strings.EqualFold(d[0], ext[1:]) {
			return d[1]
		}
	}
	return ""
}

// removeOverride drops part's Override, if any.
func (ct *contentTypes) removeOverride(part string) {
	name := "/" + part
	out := ct.overrides[:0]
	for _, o := range ct.overrides {
		if !strings.EqualFold(o[0], name) {
			out = append(out, o)
		}
	}
	ct.overrides = out
}

// Content types of the parts the writer creates.
const (
	ctMain      = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	ctTemplate  = "application/vnd.openxmlformats-officedocument.wordprocessingml.template.main+xml"
	ctStyles    = "application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"
	ctNumbering = "application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"
	ctSettings  = "application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"
	ctCore      = "application/vnd.openxmlformats-package.core-properties+xml"
	ctApp       = "application/vnd.openxmlformats-officedocument.extended-properties+xml"
	ctRels      = "application/vnd.openxmlformats-package.relationships+xml"
)
