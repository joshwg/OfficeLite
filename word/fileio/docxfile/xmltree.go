package docxfile

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"sort"
	"strings"
)

// Namespaces. Elements and attributes are named "prefix:local" with the
// prefixes below whatever prefix the file itself used, so code can switch on
// "w:p". Unknown namespaces are named "{url}local".
const (
	nsW   = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	nsR   = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	nsMC  = "http://schemas.openxmlformats.org/markup-compatibility/2006"
	nsRel = "http://schemas.openxmlformats.org/package/2006/relationships"
	nsCT  = "http://schemas.openxmlformats.org/package/2006/content-types"
	nsXML = "http://www.w3.org/XML/1998/namespace"
)

// stdNS are the namespace declarations WordLite writes on document.xml's
// root (Word's usual set), in order.
var stdNS = []struct{ prefix, url string }{
	{"wpc", "http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas"},
	{"mc", nsMC},
	{"o", "urn:schemas-microsoft-com:office:office"},
	{"r", nsR},
	{"m", "http://schemas.openxmlformats.org/officeDocument/2006/math"},
	{"v", "urn:schemas-microsoft-com:vml"},
	{"wp14", "http://schemas.microsoft.com/office/word/2010/wordprocessingDrawing"},
	{"wp", "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"},
	{"w10", "urn:schemas-microsoft-com:office:word"},
	{"w", nsW},
	{"w14", "http://schemas.microsoft.com/office/word/2010/wordml"},
	{"w15", "http://schemas.microsoft.com/office/word/2012/wordml"},
	{"wpg", "http://schemas.microsoft.com/office/word/2010/wordprocessingGroup"},
	{"wpi", "http://schemas.microsoft.com/office/word/2010/wordprocessingInk"},
	{"wne", "http://schemas.microsoft.com/office/word/2006/wordml"},
	{"wps", "http://schemas.microsoft.com/office/word/2010/wordprocessingShape"},
	{"a", "http://schemas.openxmlformats.org/drawingml/2006/main"},
	{"pic", "http://schemas.openxmlformats.org/drawingml/2006/picture"},
}

const stdIgnorable = "w14 w15 wp14"

var stdByPrefix = func() map[string]string {
	m := map[string]string{}
	for _, d := range stdNS {
		m[d.prefix] = d.url
	}
	return m
}()

// canonical prefixes by namespace URL (strict OOXML maps to transitional).
var prefixByURL = map[string]string{
	nsW: "w", "http://purl.oclc.org/ooxml/wordprocessingml/main": "w",
	nsR: "r", "http://purl.oclc.org/ooxml/officeDocument/relationships": "r",
	nsMC: "mc", nsRel: "rel", nsCT: "ct", nsXML: "xml",
	"http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing":  "wp",
	"http://schemas.openxmlformats.org/drawingml/2006/main":                   "a",
	"http://schemas.openxmlformats.org/drawingml/2006/picture":                "pic",
	"http://schemas.openxmlformats.org/drawingml/2006/chart":                  "c",
	"http://schemas.openxmlformats.org/officeDocument/2006/math":              "m",
	"urn:schemas-microsoft-com:vml":                                           "v",
	"urn:schemas-microsoft-com:office:office":                                 "o",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingShape":       "wps",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingGroup":       "wpg",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas":      "wpc",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingInk":         "wpi",
	"http://schemas.microsoft.com/office/word/2010/wordml":                    "w14",
	"http://schemas.microsoft.com/office/word/2012/wordml":                    "w15",
	"http://schemas.openxmlformats.org/package/2006/metadata/core-properties": "cp",
}

// node is a parsed XML element that remembers where it came from, so
// content WordLite doesn't model can be copied out byte for byte.
type node struct {
	name  string
	attrs []xml.Attr // Name.Space is the canonical prefix or the URL
	kids  []*node
	text  string // direct character data
	// byte offsets into src: start tag begins, start tag ends, end tag
	// begins, element ends (open == close == end for <x/>).
	start, open, close, end int
	ns                      map[string]string // prefix → URL in scope ("" = default)
	src                     []byte
}

func canonName(n xml.Name) string {
	if n.Space == "" {
		return n.Local
	}
	if p, ok := prefixByURL[n.Space]; ok {
		return p + ":" + n.Local
	}
	return "{" + n.Space + "}" + n.Local
}

// parseXML parses data into a tree. A UTF-8 byte-order mark is skipped.
func parseXML(data []byte) (*node, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	d := xml.NewDecoder(bytes.NewReader(data))
	d.CharsetReader = func(label string, r io.Reader) (io.Reader, error) { return r, nil }
	var root *node
	var stack []*node
	scope := map[string]string{"xml": nsXML}
	for {
		off := int(d.InputOffset())
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: canonName(t.Name), start: off, open: int(d.InputOffset()), src: data}
			ns := scope
			if len(stack) > 0 {
				ns = stack[len(stack)-1].ns
			}
			copied := false
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" || (a.Name.Space == "" && a.Name.Local == "xmlns") {
					if !copied {
						copied = true
						cp := make(map[string]string, len(ns)+1)
						for k, v := range ns {
							cp[k] = v
						}
						ns = cp
					}
					if a.Name.Space == "xmlns" {
						ns[a.Name.Local] = a.Value
					} else {
						ns[""] = a.Value
					}
					continue
				}
				a.Name.Space = canonSpace(a.Name.Space)
				n.attrs = append(n.attrs, a)
			}
			n.ns = ns
			if len(stack) > 0 {
				p := stack[len(stack)-1]
				p.kids = append(p.kids, n)
			} else if root == nil {
				root = n
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				continue
			}
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			n.end = int(d.InputOffset())
			if n.end == n.open {
				n.close = n.open // <x/>
			} else {
				n.close = off
			}
		case xml.CharData:
			// Only leaf text matters (w:t, w:instrText, m:t); skipping
			// mixed content keeps whitespace between elements from piling up.
			if n := len(stack); n > 0 && len(stack[n-1].kids) == 0 {
				stack[n-1].text += string(t)
			}
		}
	}
	if root == nil {
		return nil, io.ErrUnexpectedEOF
	}
	return root, nil
}

func canonSpace(s string) string {
	if s == "" {
		return ""
	}
	if p, ok := prefixByURL[s]; ok {
		return p
	}
	return s
}

// attr returns the attribute "prefix:local" (or "local"); a missing w:
// attribute also matches an unprefixed one, as some writers produce.
func (n *node) attr(name string) string {
	v, _ := n.attrOK(name)
	return v
}

func (n *node) attrOK(name string) (string, bool) {
	if n == nil {
		return "", false
	}
	space, local := "", name
	if i := strings.IndexByte(name, ':'); i >= 0 {
		space, local = name[:i], name[i+1:]
	}
	for _, a := range n.attrs {
		if a.Name.Local == local && a.Name.Space == space {
			return a.Value, true
		}
	}
	if space == "w" {
		for _, a := range n.attrs {
			if a.Name.Local == local && a.Name.Space == "" {
				return a.Value, true
			}
		}
	}
	return "", false
}

// child returns the first child named name, or nil.
func (n *node) child(name string) *node {
	if n == nil {
		return nil
	}
	for _, k := range n.kids {
		if k.name == name {
			return k
		}
	}
	return nil
}

// children returns all children named name.
func (n *node) children(name string) []*node {
	if n == nil {
		return nil
	}
	var out []*node
	for _, k := range n.kids {
		if k.name == name {
			out = append(out, k)
		}
	}
	return out
}

// find returns the first descendant named name (depth first), or nil.
func (n *node) find(name string) *node {
	if n == nil {
		return nil
	}
	for _, k := range n.kids {
		if k.name == name {
			return k
		}
		if f := k.find(name); f != nil {
			return f
		}
	}
	return nil
}

// allText concatenates the text of every descendant named name.
func (n *node) allText(name string) string {
	var b strings.Builder
	var walk func(*node)
	walk = func(x *node) {
		for _, k := range x.kids {
			if k.name == name {
				b.WriteString(k.text)
			}
			walk(k)
		}
	}
	walk(n)
	return b.String()
}

// raw returns the element's original bytes made self-contained: namespace
// declarations it relies on from its ancestors are added to its first tag
// unless WordLite's document root declares the same binding.
func (n *node) raw() []byte {
	return withDecls(n.src[n.start:n.end], n.ns)
}

// rawRange returns src[from:to] (starting at an element's start tag) with
// declarations added as raw does.
func (n *node) rawRange(from, to int) []byte {
	return withDecls(n.src[from:to], n.ns)
}

var (
	rePrefixElem = regexp.MustCompile(`</?([A-Za-z_][\w.\-]*):[A-Za-z_]`)
	rePrefixAttr = regexp.MustCompile(`\s([A-Za-z_][\w.\-]*):[\w.\-]+\s*=\s*["']`)
	reNSList     = regexp.MustCompile(`(?:Ignorable|Requires|ProcessContent|PreserveElements|PreserveAttributes|MustUnderstand)\s*=\s*"([^"]*)"`)
	reUnprefixed = regexp.MustCompile(`<([A-Za-z_][\w.\-]*)[\s/>]`)
	reDecl       = regexp.MustCompile(`xmlns(?::([\w.\-]+))?\s*=`)
)

func withDecls(frag []byte, ns map[string]string) []byte {
	if len(frag) == 0 || frag[0] != '<' {
		return append([]byte(nil), frag...)
	}
	end := tagEnd(frag)
	own := map[string]bool{}
	for _, m := range reDecl.FindAllSubmatch(frag[:end], -1) {
		own[string(m[1])] = true
	}
	used := map[string]bool{}
	for _, m := range rePrefixElem.FindAllSubmatch(frag, -1) {
		used[string(m[1])] = true
	}
	for _, m := range rePrefixAttr.FindAllSubmatch(frag, -1) {
		used[string(m[1])] = true
	}
	for _, m := range reNSList.FindAllSubmatch(frag, -1) {
		for _, p := range strings.Fields(string(m[1])) {
			if i := strings.IndexByte(p, ':'); i >= 0 {
				p = p[:i]
			}
			used[p] = true
		}
	}
	if reUnprefixed.Match(frag) {
		used[""] = true
	}
	var add []string
	for p := range used {
		if p == "xml" || p == "xmlns" || own[p] {
			continue
		}
		url, ok := ns[p]
		if !ok || url == "" {
			continue
		}
		if p != "" && stdByPrefix[p] == url {
			continue
		}
		if p == "" {
			add = append(add, ` xmlns="`+escAttr(url)+`"`)
		} else {
			add = append(add, ` xmlns:`+p+`="`+escAttr(url)+`"`)
		}
	}
	if len(add) == 0 {
		return append([]byte(nil), frag...)
	}
	sort.Strings(add)
	// insert after the element name
	i := 1
	for i < end && !isSpaceOrEnd(frag[i]) {
		i++
	}
	out := make([]byte, 0, len(frag)+64)
	out = append(out, frag[:i]...)
	out = append(out, strings.Join(add, "")...)
	out = append(out, frag[i:]...)
	return out
}

func isSpaceOrEnd(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '/' || c == '>'
}

// tagEnd returns the index just past the first tag in b.
func tagEnd(b []byte) int {
	var q byte
	for i := 1; i < len(b); i++ {
		c := b[i]
		switch {
		case q != 0:
			if c == q {
				q = 0
			}
		case c == '"' || c == '\'':
			q = c
		case c == '>':
			return i + 1
		}
	}
	return len(b)
}

// rawKids is the concatenated self-contained XML of n's children, skipping
// those named in skip.
func rawKids(n *node, skip ...string) string {
	if n == nil {
		return ""
	}
	var b []byte
outer:
	for _, k := range n.kids {
		for _, s := range skip {
			if k.name == s {
				continue outer
			}
		}
		b = append(b, k.raw()...)
	}
	return string(b)
}

// parseFragment parses a sequence of sibling elements (as stored in a
// DocxXML field) and returns them; the wrapper supplies the w:, r: and
// other standard namespaces.
func parseFragment(frag string) []*node {
	if frag == "" {
		return nil
	}
	var b strings.Builder
	b.WriteString("<frag")
	for _, d := range stdNS {
		b.WriteString(` xmlns:` + d.prefix + `="` + d.url + `"`)
	}
	b.WriteString(">")
	b.WriteString(frag)
	b.WriteString("</frag>")
	root, err := parseXML([]byte(b.String()))
	if err != nil {
		return nil
	}
	return root.kids
}

// ---- writing helpers ----

var textEsc = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
var attrEsc = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "\n", "&#10;", "\r", "&#13;", "\t", "&#9;")

func escText(s string) string { return textEsc.Replace(validXML(s)) }
func escAttr(s string) string { return attrEsc.Replace(validXML(s)) }

// validXML removes characters XML 1.0 can't contain.
func validXML(s string) string {
	ok := true
	for _, r := range s {
		if !isXMLChar(r) {
			ok = false
			break
		}
	}
	if ok {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if isXMLChar(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isXMLChar(r rune) bool {
	return r == 0x9 || r == 0xA || r == 0xD || (r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) || (r >= 0x10000 && r <= 0x10FFFF)
}
