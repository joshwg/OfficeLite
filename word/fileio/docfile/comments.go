// comments.go reads a .doc's comments (annotations): the reference
// characters in the main text (PlcfandRef, with each comment's author
// and initials), the comments' text in the annotation story (PlcfandTxt),
// the authors' names (GrpXstAtnOwners) and the commented ranges, kept as
// bookmarks (SttbfAtnBkmk, PlcfAtnBkf, PlcfAtnBkl) tied to a comment by
// its tag. A .doc can't save comments; they are kept when saving as .docx.

package docfile

import (
	"slices"
	"unicode/utf16"

	"officelite/word"
	"officelite/word/model"
)

// FibRgFcLcb97 entries of the comment structures.
const (
	fcPlcfandTxt      = 5
	fcGrpXstAtnOwners = 36
	fcSttbfAtnBkmk    = 37
	fcPlcfAtnBkf      = 42
	fcPlcfAtnBkl      = 43

	atrdSize = 30 // ATRDPre10: initials (2+18), ibst, 2 unused words, lTagBkmk
)

// WarnCommentsLost is the warning of a .doc save of a document with
// comments.
const WarnCommentsLost = "Comments are not kept when saving as .doc (save as .docx to keep them)."

// commentMarks are the comment anchors by CP: comments whose reference
// character is there, whose range starts there, and whose range ends
// there.
type commentMarks struct {
	ref         map[int]*model.Comment
	start, end  map[int][]*model.Comment
	startCP     map[*model.Comment]int
	hasComments bool
}

// xstList reads consecutive Xst strings (a count of UTF-16 units, then
// the units), at most max of them.
func xstList(b []byte, max int) []string {
	var out []string
	for len(b) >= 2 && len(out) < max {
		n := int(u16(b))
		if 2+2*n > len(b) {
			break
		}
		out = append(out, utf16String(b[2:2+2*n]))
		b = b[2+2*n:]
	}
	return out
}

// utf16String decodes little-endian UTF-16.
func utf16String(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = u16(b[2*i:])
	}
	return string(utf16.Decode(u))
}

// atnRanges reads the commented ranges by tag: [start, end) CPs.
func (r *reader) atnRanges() map[uint32][2]int {
	sttb := r.fib.blob(r.table, fcSttbfAtnBkmk)
	bkf := r.fib.blob(r.table, fcPlcfAtnBkf)
	bkl := r.fib.blob(r.table, fcPlcfAtnBkl)
	if len(sttb) < 6 || u16(sttb) != 0xFFFF || len(bkf) < 4 || len(bkl) < 4 {
		return nil
	}
	n, extra := int(u16(sttb[2:])), int(u16(sttb[4:]))
	var tags []uint32
	p := 6
	for i := 0; i < n && p+2 <= len(sttb); i++ {
		p += 2 + 2*int(u16(sttb[p:]))
		if extra < 6 || p+extra > len(sttb) {
			break
		}
		tags = append(tags, u32(sttb[p+2:]))
		p += extra
	}
	nf := (len(bkf) - 4) / 8
	nl := len(bkl)/4 - 1
	out := map[uint32][2]int{}
	for i := 0; i < nf && i < len(tags); i++ {
		start := int(u32(bkf[4*i:]))
		ibkl := int(u16(bkf[4*(nf+1)+4*i:]))
		if ibkl >= nl {
			continue
		}
		end := int(u32(bkl[4*ibkl:]))
		if end >= start {
			out[tags[i]] = [2]int{start, end}
		}
	}
	return out
}

// readComments reads the comments and where they are anchored.
func (r *reader) readComments() {
	m := &r.atn
	m.ref, m.start, m.end, m.startCP = map[int]*model.Comment{}, map[int][]*model.Comment{}, map[int][]*model.Comment{}, map[*model.Comment]int{}
	ref, txt := r.fib.blob(r.table, fcPlcfandRef), r.fib.blob(r.table, fcPlcfandTxt)
	if len(ref) < 4 || r.fib.lw[lwCcpAtn] <= 0 {
		return
	}
	n := (len(ref) - 4) / (4 + atrdSize)
	n = min(n, word.MaxComments)
	owners := xstList(r.fib.blob(r.table, fcGrpXstAtnOwners), 1<<16)
	ranges := r.atnRanges()
	base := r.storyStart(lwCcpAtn)
	for i := 0; i < n; i++ {
		cp := int(u32(ref[4*i:]))
		atrd := ref[4*(n+1)+atrdSize*i:]
		if cp < 0 || cp >= len(r.text) || len(atrd) < atrdSize {
			continue
		}
		c := &model.Comment{}
		if k := int(u16(atrd)); k <= 9 {
			c.Initials = utf16String(atrd[2 : 2+2*k])
		}
		if k := int(i16(atrd[20:])); k >= 0 && k < len(owners) {
			c.Author = owners[k]
		}
		if a, b, ok := r.story(txt, i, base, false); ok {
			r.inNote = true
			c.Blocks = r.blocks(a, b, false)
			r.inNote = false
		}
		if len(c.Blocks) == 0 {
			c.Blocks = []model.Block{&model.Paragraph{}}
		}
		m.ref[cp] = c
		m.hasComments = true
		if rg, ok := ranges[u32(atrd[26:])]; ok && rg[1] <= cp {
			m.start[rg[0]] = append(m.start[rg[0]], c)
			m.end[rg[1]] = append(m.end[rg[1]], c)
			m.startCP[c] = rg[0]
		}
	}
}

// commentMarksAt adds the comment anchors at cp before its character:
// the ends of ranges that end there, then the starts of those that start
// there (an empty range's end after its start).
func (r *reader) commentMarksAt(cp int, add func(model.Run)) {
	m := &r.atn
	if !m.hasComments {
		return
	}
	mark := func(c *model.Comment, k model.CommentMark) {
		add(model.Run{Kind: model.RunOpaque, Opaque: &model.Opaque{Comment: c, CommentMark: k}})
	}
	for _, c := range m.end[cp] {
		if m.startCP[c] < cp {
			mark(c, model.CommentEnd)
		}
	}
	for _, c := range m.start[cp] {
		mark(c, model.CommentStart)
		if m.startCP[c] == cp && slices.Contains(m.end[cp], c) {
			mark(c, model.CommentEnd)
		}
	}
}
