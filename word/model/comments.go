// comments.go holds comments: notes in the margin on a range of text,
// with replies and a resolved (done) state. A comment is anchored by
// three zero-width opaque runs that share it: where its range starts,
// where it ends, and its reference just after the end.

package model

import (
	"slices"
	"strings"
	"time"
)

// Comment is a comment on a range of text. Its replies are comments too,
// without anchors of their own (a file format writes their anchors next
// to the comment's).
//
// A Comment is never changed once it is in a document: UpdateComment
// puts a changed copy in place, which keeps undo working.
type Comment struct {
	Author   string
	Initials string
	Date     time.Time // zero if unknown
	Blocks   []Block   // the text: paragraphs (formatting kept from the file)
	Done     bool      // the thread is resolved (on the first comment)
	Replies  []*Comment
	// DocxID is the comment's id in the .docx it came from, and
	// DocxParaID the w14:paraId of its last paragraph there, which
	// commentsExtended.xml refers to. A save keeps them when it can.
	DocxID     string
	DocxParaID string
}

// CommentMark is what a comment anchor run marks.
type CommentMark int8

const (
	CommentNone  CommentMark = iota
	CommentStart             // where the commented text starts
	CommentEnd               // where it ends
	CommentRef               // the comment's reference, after the end
)

// NewComment returns a comment by author holding text (one paragraph per
// line), dated now.
func NewComment(author, text string) *Comment {
	return &Comment{Author: author, Initials: Initials(author), Date: time.Now(), Blocks: TextBlocks(text)}
}

// TextBlocks makes plain paragraphs of text, one per line.
func TextBlocks(text string) []Block {
	var out []Block
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		p := &Paragraph{}
		if line != "" {
			p.Runs = []Run{{Kind: RunText, Text: line}}
		}
		out = append(out, p)
	}
	return out
}

// Initials makes initials from a name ("Josh Smith" → "JS").
func Initials(name string) string {
	var b strings.Builder
	for _, w := range strings.Fields(name) {
		for _, r := range w {
			b.WriteRune(r)
			break
		}
	}
	return strings.ToUpper(b.String())
}

// Text is the comment's plain text: its paragraphs' text, one per line,
// without the content it can't show as text.
func (c *Comment) Text() string {
	var lines []string
	WalkBlocks(c.Blocks, func(p *Paragraph) bool {
		var b strings.Builder
		for _, r := range p.Runs {
			switch r.Kind {
			case RunText:
				b.WriteString(r.Text)
			case RunTab:
				b.WriteByte('\t')
			case RunBreak:
				b.WriteByte('\n')
			}
		}
		lines = append(lines, b.String())
		return true
	})
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// Clone returns a copy of c that can be changed (its replies are shared;
// replace them to change one).
func (c *Comment) Clone() *Comment {
	d := *c
	d.Replies = slices.Clone(c.Replies)
	return &d
}

// commentRun is an anchor run of c.
func commentRun(c *Comment, m CommentMark) Run {
	return Run{Kind: RunOpaque, Opaque: &Opaque{Comment: c, CommentMark: m}}
}

// CommentAnchor is where a comment is in the text: the positions of its
// start and end marks (Start == End for a comment without a range, at
// its reference).
type CommentAnchor struct {
	Comment    *Comment
	Start, End Pos
}

// Comments lists the editor's comments in the order of their references,
// each with where its text is. A comment without a reference isn't one
// (its file had it removed).
func (e *Editor) Comments() []CommentAnchor {
	var out []CommentAnchor
	starts := map[*Comment]Pos{}
	ends := map[*Comment]Pos{}
	for i, loc := range e.locs {
		off := 0
		for _, r := range loc.P.Runs {
			if o := r.Opaque; o != nil && o.Comment != nil {
				p := Pos{i, off}
				switch o.CommentMark {
				case CommentStart:
					if _, ok := starts[o.Comment]; !ok {
						starts[o.Comment] = p
					}
				case CommentEnd:
					if _, ok := ends[o.Comment]; !ok {
						ends[o.Comment] = p
					}
				case CommentRef:
					if !slices.ContainsFunc(out, func(a CommentAnchor) bool { return a.Comment == o.Comment }) {
						out = append(out, CommentAnchor{Comment: o.Comment, End: p})
					}
				}
			}
			off += RunLen(&r)
		}
	}
	for i := range out {
		a := &out[i]
		if p, ok := ends[a.Comment]; ok {
			a.End = p
		}
		a.Start = a.End
		if p, ok := starts[a.Comment]; ok && !a.End.Less(p) {
			a.Start = p
		}
	}
	return out
}

// CommentAt returns the comment whose text holds the caret (the
// innermost one: the last to start), if any.
func (e *Editor) CommentAt(p Pos) (*Comment, bool) {
	var found *Comment
	var at Pos
	for _, a := range e.Comments() {
		if !p.Less(a.Start) && !a.End.Less(p) && (found == nil || !a.Start.Less(at)) {
			found, at = a.Comment, a.Start
		}
	}
	return found, found != nil
}

// AddComment comments on the selection (or, with nothing selected, on
// the caret's position) with c, leaving the selection as it was.
func (e *Editor) AddComment(c *Comment) {
	defer e.Op("New Comment")()
	from, to := e.span()
	e.edit("insert", from, to, false, func() {
		a, b := e.Sel.Ordered()
		forward := e.Sel.Anchor == a
		insertRuns(e.locs[b.Para].P, b.Off, commentRun(c, CommentEnd), commentRun(c, CommentRef))
		insertRuns(e.locs[a.Para].P, a.Off, commentRun(c, CommentStart))
		if a == b {
			p := Pos{a.Para, a.Off + 3} // after the three marks
			e.Sel = Selection{p, p}
			return
		}
		// The text between the marks stays selected.
		a.Off++
		if b.Para == a.Para {
			b.Off++
		}
		e.Sel = Selection{b, a}
		if forward {
			e.Sel = Selection{a, b}
		}
	})
}

// commentSpan returns the body blocks holding c's anchors; ok is false
// when there are none.
func (e *Editor) commentSpan(c *Comment) (from, to int, ok bool) {
	from, to = -1, -1
	for _, loc := range e.locs {
		for _, r := range loc.P.Runs {
			if r.Opaque != nil && r.Opaque.Comment == c {
				if from < 0 {
					from = loc.Top
				}
				to = loc.Top
				break
			}
		}
	}
	return from, to, from >= 0
}

// UpdateComment puts next in the place of old (a reply added, the text
// changed, resolved…). Undoable.
func (e *Editor) UpdateComment(old, next *Comment, label string) {
	from, to, ok := e.commentSpan(old)
	if !ok {
		return
	}
	defer e.Op(label)()
	e.edit("insert", from, to, false, func() {
		WalkBlocks(e.Doc.Body[from:to+1], func(p *Paragraph) bool {
			for i := range p.Runs {
				if o := p.Runs[i].Opaque; o != nil && o.Comment == old {
					c := *o
					c.Comment = next
					p.Runs[i].Opaque = &c
				}
			}
			return true
		})
	})
}

// DeleteComment removes c (with its replies), keeping its text. Undoable.
func (e *Editor) DeleteComment(c *Comment) {
	from, to, ok := e.commentSpan(c)
	if !ok {
		return
	}
	defer e.Op("Delete Comment")()
	sel := e.Sel
	e.edit("delete", from, to, false, func() {
		for i, loc := range e.locs {
			p := loc.P
			var gone []int // caret offsets of the removed marks
			off := 0
			out := p.Runs[:0]
			for _, r := range p.Runs {
				if r.Opaque != nil && r.Opaque.Comment == c {
					gone = append(gone, off)
				} else {
					out = append(out, r)
				}
				off += RunLen(&r)
			}
			clear(p.Runs[len(out):])
			p.Runs = out
			if len(gone) > 0 {
				normalize(p)
			}
			sel.Anchor = shiftBack(sel.Anchor, i, gone)
			sel.Caret = shiftBack(sel.Caret, i, gone)
		}
		e.Sel = sel
	})
}

// shiftBack moves p back over the caret steps removed from paragraph
// para at the offsets gone.
func shiftBack(p Pos, para int, gone []int) Pos {
	if p.Para != para {
		return p
	}
	n := 0
	for _, o := range gone {
		if o < p.Off {
			n++
		}
	}
	p.Off -= n
	return p
}

// IsCommentMark reports whether r is a comment anchor, which takes no
// room and isn't shown.
func IsCommentMark(r *Run) bool {
	return r.Kind == RunOpaque && r.Opaque != nil && r.Opaque.Comment != nil
}
