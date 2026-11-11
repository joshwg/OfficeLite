// comments.go reads and writes comments: word/comments.xml (each
// comment's author, date and text), word/commentsExtended.xml (which
// comment a reply answers, and whether a thread is resolved) and the
// anchors in the text (w:commentRangeStart, w:commentRangeEnd and the
// w:commentReference run). Replies are kept on the comment they answer;
// their anchors, which Word puts next to that comment's, are written
// back the same way.

package docxfile

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"officelite/common/opc"
	"officelite/word/model"
)

const (
	relCommentsExt         = "/commentsExtended"
	relCommentsIDs         = "/commentsIds"
	relCommentsExtensible  = "/commentsExtensible"
	relTypeCommentsExt     = "http://schemas.microsoft.com/office/2011/relationships/commentsExtended"
	ctComments             = "application/vnd.openxmlformats-officedocument.wordprocessingml.comments+xml"
	ctCommentsExt          = "application/vnd.openxmlformats-officedocument.wordprocessingml.commentsExtended+xml"
	nsW15                  = "http://schemas.microsoft.com/office/word/2012/wordml"
	commentDateLayout      = "2006-01-02T15:04:05Z"
	warnCommentsUnreadable = "Some comments could not be read; their marks are kept when saving as .docx."
)

// commentState is what the reader knows of the comments: each by id, and
// the replies (whose anchors are dropped).
type commentState struct {
	byID    map[string]*model.Comment
	replies map[*model.Comment]bool
}

// loadComments reads the comments part and commentsExtended.xml, linking
// replies to the comments they answer.
func (r *reader) loadComments() {
	r.cm = commentState{byID: map[string]*model.Comment{}, replies: map[*model.Comment]bool{}}
	var part string
	var root *node
	type ext struct {
		parent string
		done   bool
	}
	exts := map[string]ext{}
	for _, rl := range r.rels {
		p := opc.Resolve(r.main, rl.Target)
		switch {
		case rl.is(relComments):
			b, err := r.pkg.read(p)
			if err != nil {
				r.warn(warnCommentsUnreadable)
				continue
			}
			if root, err = parseXML(b); err != nil {
				r.warn(warnCommentsUnreadable)
				root = nil
				continue
			}
			part = p
		case rl.is(relCommentsExt):
			b, err := r.pkg.read(p)
			if err != nil {
				continue
			}
			er, err := parseXML(b)
			if err != nil {
				continue
			}
			for _, k := range er.kids {
				if k.name == "w15:commentEx" {
					exts[strings.ToUpper(k.attr("w15:paraId"))] = ext{strings.ToUpper(k.attr("w15:paraIdParent")), onOffAttr(k.attr("w15:done"))}
				}
			}
		}
	}
	if root == nil {
		return
	}
	byPara := map[string]*model.Comment{}
	var order []*model.Comment
	for _, k := range root.kids {
		if k.name != "w:comment" {
			continue
		}
		c := &model.Comment{Author: k.attr("w:author"), Initials: k.attr("w:initials"), DocxID: k.attr("w:id")}
		if t, err := time.Parse(time.RFC3339, k.attr("w:date")); err == nil {
			c.Date = t
		} else if t, err := time.Parse("2006-01-02T15:04:05", k.attr("w:date")); err == nil {
			c.Date = t
		}
		c.Blocks = r.partBlocks(part, k)
		if ps := k.children("w:p"); len(ps) > 0 {
			c.DocxParaID = strings.ToUpper(ps[len(ps)-1].attr("w14:paraId"))
		}
		r.cm.byID[k.attr("w:id")] = c
		if c.DocxParaID != "" {
			byPara[c.DocxParaID] = c
		}
		order = append(order, c)
	}
	for _, c := range order {
		e := exts[c.DocxParaID]
		c.Done = e.done
		if p := byPara[e.parent]; p != nil && p != c && !r.cm.replies[p] {
			p.Replies = append(p.Replies, c)
			r.cm.replies[c] = true
		}
	}
}

// commentMark makes the run of a comment anchor element (start or end of
// its range, or its reference); ok is false for an anchor whose comment
// isn't known, kept as it is.
func (r *reader) commentMark(k *node, run *model.Run) bool {
	c := r.cm.byID[k.attr("w:id")]
	if c == nil {
		return false
	}
	run.Text = ""
	run.Opaque.Comment = c
	switch k.name {
	case "w:commentRangeStart":
		run.Opaque.CommentMark = model.CommentStart
	case "w:commentRangeEnd":
		run.Opaque.CommentMark = model.CommentEnd
	default:
		run.Opaque.CommentMark = model.CommentRef
	}
	return true
}

// dropReplyMarks removes the anchors of replies, which are written next
// to the anchors of the comments they answer.
func (r *reader) dropReplyMarks() {
	if len(r.cm.replies) == 0 {
		return
	}
	model.WalkBlocks(r.doc.Body, func(p *model.Paragraph) bool {
		p.Runs = slices.DeleteFunc(p.Runs, func(run model.Run) bool {
			return run.Opaque != nil && r.cm.replies[run.Opaque.Comment]
		})
		return true
	})
}

// ---- writing ----

// commentWriter is the writer's comment state: the id and paraId of each
// comment written, and which of its anchors have been written (a copied
// anchor is written once).
type commentWriter struct {
	ids     map[*model.Comment]string
	paraIDs map[*model.Comment]string
	written map[*model.Comment]map[model.CommentMark]bool
	// drop are parts left out of the package: comment id lists that
	// would name comments that changed.
	drop []string
}

// commentsIn lists the comments referenced in blocks, in order.
func commentsIn(blocks []model.Block) []*model.Comment {
	var out []*model.Comment
	model.WalkBlocks(blocks, func(p *model.Paragraph) bool {
		for _, r := range p.Runs {
			if o := r.Opaque; o != nil && o.Comment != nil && o.CommentMark == model.CommentRef && !slices.Contains(out, o.Comment) {
				out = append(out, o.Comment)
			}
		}
		return true
	})
	return out
}

// newParaID makes a paraId (8 hex digits below 0x80000000) not in used.
func newParaID(used map[string]bool) string {
	for {
		var b [4]byte
		_, _ = rand.Read(b[:])
		id := fmt.Sprintf("%08X", binary.BigEndian.Uint32(b[:])&0x7FFFFFFF)
		if !used[id] && id != "00000000" {
			used[id] = true
			return id
		}
	}
}

// prepareComments writes comments.xml and commentsExtended.xml when the
// document has comments (or had the parts), before document.xml so the
// anchors know their ids. Comments as they were read keep their parts
// byte for byte; otherwise the id lists Word adds (commentsIds.xml,
// commentsExtensible.xml) are dropped, as they would name comments that
// changed.
func (w *writer) prepareComments() {
	cw := &w.cm
	cw.ids, cw.paraIDs = map[*model.Comment]string{}, map[*model.Comment]string{}
	cw.written = map[*model.Comment]map[model.CommentMark]bool{}
	top := commentsIn(w.doc.Body)
	all := withReplies(top)
	part, extPart := "", ""
	var orig *node
	for _, rl := range w.rels {
		p := opc.Resolve(w.main, rl.Target)
		switch {
		case rl.is(relComments):
			part = p
			if w.src != nil {
				if b, err := w.src.read(p); err == nil {
					orig, _ = parseXML(b)
				}
			}
		case rl.is(relCommentsExt):
			extPart = p
		}
	}
	if len(all) == 0 && part == "" {
		return
	}
	w.assignCommentIDs(all)
	if orig != nil && w.unchangedComments(top, part) {
		for _, name := range []string{part, extPart} {
			if b, err := w.src.read(name); err == nil && name != "" {
				w.noteParts = append(w.noteParts, hfPart{name: name, data: b})
			}
		}
		return
	}
	var keep []rel
	for _, rl := range w.rels {
		if rl.is(relCommentsIDs) || rl.is(relCommentsExtensible) {
			p := opc.Resolve(w.main, rl.Target)
			cw.drop = append(cw.drop, p, opc.RelsPath(p))
			w.relsDirty = true
			continue
		}
		keep = append(keep, rl)
	}
	w.rels = keep

	add := func(name, typ, ctype string) hfPart {
		dir := path0(w.main)
		np := hfPart{name: dir + name, ctype: ctype, isNew: true, relID: w.newRelID()}
		w.rels = append(w.rels, rel{ID: np.relID, Type: typ, Target: name})
		w.relsDirty = true
		return np
	}
	cp := hfPart{name: part, ctype: ctComments}
	if part == "" {
		cp = add("comments.xml", relTypePrefix+relComments, ctComments)
	}
	cp.data, cp.rels = w.commentsXML(all, orig, cp.name)
	w.noteParts = append(w.noteParts, cp)

	ep := hfPart{name: extPart, ctype: ctCommentsExt}
	if extPart == "" {
		ep = add("commentsExtended.xml", relTypeCommentsExt, ctCommentsExt)
	}
	ep.data = []byte(w.commentsExXML(top))
	w.noteParts = append(w.noteParts, ep)
}

// withReplies lists comments each followed by its replies.
func withReplies(top []*model.Comment) []*model.Comment {
	var all []*model.Comment
	for _, c := range top {
		all = append(all, c)
		all = append(all, c.Replies...)
	}
	return all
}

// assignCommentIDs gives every comment its id and paraId: those it was
// read with when they are still unique, new ones otherwise.
func (w *writer) assignCommentIDs(all []*model.Comment) {
	cw := &w.cm
	usedID, usedPara := map[string]bool{}, map[string]bool{}
	next := 0
	for _, c := range all {
		if id := c.DocxID; id != "" && !usedID[id] {
			usedID[id] = true
			cw.ids[c] = id
			next = max(next, atoi(id)+1)
		}
		if id := c.DocxParaID; len(id) == 8 && !usedPara[id] {
			usedPara[id] = true
			cw.paraIDs[c] = id
		}
	}
	for _, c := range all {
		if cw.ids[c] == "" {
			for usedID[strconv.Itoa(next)] {
				next++
			}
			cw.ids[c] = strconv.Itoa(next)
			usedID[cw.ids[c]] = true
		}
		if cw.paraIDs[c] == "" {
			cw.paraIDs[c] = newParaID(usedPara)
		}
	}
}

// commentsExXML writes commentsExtended.xml: each comment's paraId, the
// paraId of the comment a reply answers, and whether the thread is done.
func (w *writer) commentsExXML(top []*model.Comment) string {
	cw := &w.cm
	var b strings.Builder
	b.WriteString(xmlHeader + `<w15:commentsEx xmlns:mc="` + nsMC + `" xmlns:w15="` + nsW15 + `" mc:Ignorable="w15">`)
	for _, c := range top {
		fmt.Fprintf(&b, `<w15:commentEx w15:paraId="%s" w15:done="%s"/>`, cw.paraIDs[c], boolDigit(c.Done))
		for _, rc := range c.Replies {
			fmt.Fprintf(&b, `<w15:commentEx w15:paraId="%s" w15:paraIdParent="%s" w15:done="%s"/>`, cw.paraIDs[rc], cw.paraIDs[c], boolDigit(c.Done))
		}
	}
	b.WriteString(`</w15:commentsEx>`)
	return b.String()
}

// unchangedComments reports whether the comments are those of the
// source package as it was read: the same ids, each with the same text,
// author, date, replies and state.
func (w *writer) unchangedComments(top []*model.Comment, part string) bool {
	r := &reader{doc: &model.Document{Styles: w.doc.Styles, Lists: map[int]*model.List{}}, th: w.th,
		warned: map[string]bool{}, pkg: w.src, main: w.main, rels: map[string]rel{}}
	for _, rl := range w.src.rels(w.main) {
		r.rels[rl.ID] = rl
	}
	r.loadComments()
	var srcTop []*model.Comment
	for _, c := range r.cm.byID {
		if !r.cm.replies[c] {
			srcTop = append(srcTop, c)
		}
	}
	if len(srcTop) != len(top) {
		return false
	}
	mainRels, mainDirty, mainPart, mainPics := w.rels, w.relsDirty, w.main, w.picRels
	defer func() { w.rels, w.relsDirty, w.main, w.picRels = mainRels, mainDirty, mainPart, mainPics }()
	w.rels, w.relsDirty, w.main, w.picRels = w.src.rels(part), false, part, nil
	sig := func(top []*model.Comment, ids map[*model.Comment]string) map[string]string {
		out := map[string]string{}
		for _, c := range top {
			for i, x := range withReplies([]*model.Comment{c}) {
				parent := ""
				if i > 0 {
					parent = c.DocxID
				}
				out[x.DocxID] = w.commentXML(x, ids[x], x.DocxParaID) + "|" + parent + "|" + boolDigit(c.Done)
			}
		}
		return out
	}
	srcIDs := map[*model.Comment]string{}
	for id, c := range r.cm.byID {
		srcIDs[c] = id
	}
	for _, c := range withReplies(top) {
		if w.cm.ids[c] != c.DocxID || w.cm.paraIDs[c] != c.DocxParaID {
			return false
		}
	}
	return maps.Equal(sig(top, w.cm.ids), sig(srcTop, srcIDs)) && !w.relsDirty
}

// boolDigit is "1" for true, "0" for false.
func boolDigit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// commentsXML writes comments.xml with its comments' text, each comment's
// last paragraph carrying the paraId commentsExtended.xml refers to.
func (w *writer) commentsXML(all []*model.Comment, orig *node, part string) ([]byte, []byte) {
	var b strings.Builder
	b.WriteString(xmlHeader)
	rootTag(&b, "w:comments", orig)
	mainRels, mainDirty, mainPart, mainPics := w.rels, w.relsDirty, w.main, w.picRels
	var partRels []rel
	if w.src != nil && orig != nil {
		partRels = w.src.rels(part)
	}
	w.rels, w.relsDirty, w.main, w.picRels = partRels, false, part, nil
	for _, c := range all {
		b.WriteString(w.commentXML(c, w.cm.ids[c], w.cm.paraIDs[c]))
	}
	b.WriteString("</w:comments>")
	var relsData []byte
	if w.relsDirty {
		relsData = relsXML(w.rels)
	}
	w.rels, w.relsDirty, w.main, w.picRels = mainRels, mainDirty, mainPart, mainPics
	return []byte(b.String()), relsData
}

// commentXML writes one w:comment, its last paragraph carrying paraID.
func (w *writer) commentXML(c *model.Comment, id, paraID string) string {
	var b strings.Builder
	b.WriteString(`<w:comment w:id="` + id + `" w:author="` + escAttr(c.Author) + `"`)
	if !c.Date.IsZero() {
		b.WriteString(` w:date="` + c.Date.UTC().Format(commentDateLayout) + `"`)
	}
	if c.Initials != "" {
		b.WriteString(` w:initials="` + escAttr(c.Initials) + `"`)
	}
	b.WriteString(">")
	blocks := c.Blocks
	if len(blocks) == 0 {
		blocks = []model.Block{&model.Paragraph{}}
	} else if _, ok := blocks[len(blocks)-1].(*model.Paragraph); !ok {
		blocks = append(blocks[:len(blocks):len(blocks)], &model.Paragraph{})
	}
	var cb strings.Builder
	w.blocks(&cb, blocks)
	s := cb.String()
	if i := strings.LastIndex(s, "<w:p>"); i >= 0 && paraID != "" {
		s = s[:i] + `<w:p w14:paraId="` + paraID + `" w14:textId="77777777">` + s[i+len("<w:p>"):]
	}
	return b.String() + s + "</w:comment>"
}

// reCommentID finds the id of an anchor element's original XML.
var reCommentID = regexp.MustCompile(`w:id="([^"]*)"`)

// commentRun writes a comment anchor and the same anchor of each reply;
// nothing for a comment without a reference, or an anchor already
// written (a copy).
func (w *writer) commentRun(b *strings.Builder, r *model.Run) {
	c, m := r.Opaque.Comment, r.Opaque.CommentMark
	if _, ok := w.cm.ids[c]; !ok || w.cm.written[c][m] {
		return
	}
	if w.cm.written[c] == nil {
		w.cm.written[c] = map[model.CommentMark]bool{}
	}
	w.cm.written[c][m] = true
	refStyle := ""
	if w.doc.Styles.Find("CommentReference") != nil {
		refStyle = `<w:rPr><w:rStyle w:val="CommentReference"/></w:rPr>`
	}
	for i, x := range append([]*model.Comment{c}, c.Replies...) {
		id := w.cm.ids[x]
		if o := r.Opaque; i == 0 && w.opaqueOK(o) {
			if m := reCommentID.FindSubmatch(o.Data); m != nil && string(m[1]) == id {
				b.Write(o.Data) // the anchor as it was read
				continue
			}
		}
		switch m {
		case model.CommentStart:
			b.WriteString(`<w:commentRangeStart w:id="` + id + `"/>`)
		case model.CommentEnd:
			b.WriteString(`<w:commentRangeEnd w:id="` + id + `"/>`)
		case model.CommentRef:
			b.WriteString(`<w:r>` + refStyle + `<w:commentReference w:id="` + id + `"/></w:r>`)
		}
	}
}
