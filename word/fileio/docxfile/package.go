package docxfile

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"strings"

	"officelite/common/opc"
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
}

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

func (p *pkg) read(name string) ([]byte, error) {
	f := p.find(name)
	if f == nil {
		return nil, fmt.Errorf("missing part %s", name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, 1<<30))
}

type rel struct {
	ID, Type, Target, Mode string
}

func (r rel) is(suffix string) bool { return strings.HasSuffix(r.Type, suffix) }

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

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n"

// contentTypes is [Content_Types].xml.
type contentTypes struct {
	defaults  [][2]string // extension, type
	overrides [][2]string // part name ("/word/document.xml"), type
}

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

const (
	ctMain      = "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"
	ctStyles    = "application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"
	ctNumbering = "application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"
	ctSettings  = "application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"
	ctCore      = "application/vnd.openxmlformats-package.core-properties+xml"
	ctApp       = "application/vnd.openxmlformats-officedocument.extended-properties+xml"
	ctRels      = "application/vnd.openxmlformats-package.relationships+xml"
)
