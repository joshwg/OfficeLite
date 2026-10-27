package docfile

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf16"

	"officelite/word/model"
)

// Style kinds (stk).
const (
	stkPara  = 1
	stkChar  = 2
	stkTable = 3
	stkList  = 4
)

const (
	istdNil = 0x0FFF
	stiUser = 0x0FFE
)

// stiIDs maps built-in style identifiers to Word's .docx style IDs.
var stiIDs = map[int]string{
	0: "Normal", 28: "NormalIndent", 29: "FootnoteText", 30: "CommentText", 31: "Header", 32: "Footer",
	33: "IndexHeading", 34: "Caption", 35: "TableofFigures", 36: "EnvelopeAddress", 37: "EnvelopeReturn",
	38: "FootnoteReference", 39: "CommentReference", 40: "LineNumber", 41: "PageNumber",
	42: "EndnoteReference", 43: "EndnoteText", 44: "TableofAuthorities", 45: "MacroText", 46: "TOAHeading",
	47: "List", 48: "ListBullet", 49: "ListNumber", 62: "Title", 63: "Closing", 64: "Signature",
	65: "DefaultParagraphFont", 66: "BodyText", 67: "BodyTextIndent", 68: "ListContinue",
	73: "MessageHeader", 74: "Subtitle", 75: "Salutation", 76: "Date", 77: "BodyTextFirstIndent",
	78: "BodyTextFirstIndent2", 79: "NoteHeading", 80: "BodyText2", 81: "BodyText3",
	82: "BodyTextIndent2", 83: "BodyTextIndent3", 84: "BlockText", 85: "Hyperlink",
	86: "FollowedHyperlink", 87: "Strong", 88: "Emphasis", 89: "DocumentMap", 90: "PlainText",
	105: "TableNormal", 106: "CommentSubject", 107: "NoList", 108: "BalloonText", 154: "TableGrid",
}

func init() {
	for i := 1; i <= 9; i++ {
		stiIDs[i] = fmt.Sprintf("Heading%d", i)
		stiIDs[9+i] = fmt.Sprintf("Index%d", i)
		stiIDs[18+i] = fmt.Sprintf("TOC%d", i)
	}
	for i := 2; i <= 5; i++ {
		stiIDs[48+i] = fmt.Sprintf("List%d", i)
		stiIDs[52+i] = fmt.Sprintf("ListBullet%d", i)
		stiIDs[56+i] = fmt.Sprintf("ListNumber%d", i)
		stiIDs[67+i] = fmt.Sprintf("ListContinue%d", i)
	}
}

// stiFor returns the built-in identifier of a model style ID, or stiUser.
func stiFor(id string) int {
	for sti, sid := range stiIDs {
		if sid == id {
			return sti
		}
	}
	return stiUser
}

// styleIDFromName derives a .docx-like style ID: words capitalized,
// everything but letters and digits removed ("My Para" → "MyPara").
func styleIDFromName(name string) string {
	var b strings.Builder
	up := true
	for _, c := range name {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			if up {
				c = unicode.ToUpper(c)
			}
			b.WriteRune(c)
			up = false
		} else {
			up = true
		}
	}
	if b.Len() == 0 {
		return "Style"
	}
	return b.String()
}

// stdInfo is one parsed style definition.
type stdInfo struct {
	sti, stk          int
	istdBase, istdNxt int
	name              string
	hidden            bool
	papx, chpx        []byte
	istd              int
}

// readStyles parses the STSH into doc.Styles.
func (r *reader) readStyles() {
	st := &r.doc.Styles
	st.DefaultRun = model.RunProps{Font: "Times New Roman", Size: 10}
	st.DefaultPara = model.ParaProps{SpaceBefore: model.F(0), SpaceAfter: model.F(0), Line: 1, LineRule: model.LineAuto}
	b := r.fib.blob(r.table, fcStshf)
	if len(b) < 2 {
		r.ensureNormal()
		return
	}
	cbStshi := int(u16(b))
	if 2+cbStshi > len(b) || cbStshi < 14 {
		r.ensureNormal()
		return
	}
	stshi := b[2 : 2+cbStshi]
	cstd := int(u16(stshi))
	cbBase := int(u16(stshi[2:]))
	if f := r.font(u16(stshi[12:])); f != "" {
		st.DefaultRun.Font = f
	}
	p := 2 + cbStshi
	for istd := 0; istd < cstd && p+2 <= len(b); istd++ {
		cb := int(u16(b[p:]))
		p += 2
		if p+cb > len(b) {
			break
		}
		std := b[p : p+cb]
		p += cb
		var si *stdInfo
		if cb >= 10 {
			si = parseStd(std, cbBase)
		}
		if si != nil {
			si.istd = istd
		}
		r.styles = append(r.styles, si)
	}

	// IDs.
	r.styleIDs = make([]string, len(r.styles))
	used := map[string]bool{}
	for i, si := range r.styles {
		if si == nil {
			continue
		}
		id, ok := stiIDs[si.sti]
		if !ok || si.sti == stiUser {
			id = styleIDFromName(firstAlias(si.name))
		}
		base := id
		for n := 2; used[id]; n++ {
			id = fmt.Sprintf("%s%d", base, n)
		}
		used[id] = true
		r.styleIDs[i] = id
	}

	// Styles in two passes: props without toggles, then relative toggles.
	type pending struct {
		s   *model.Style
		chp *chpInfo
		si  *stdInfo
	}
	var pend []pending
	for i, si := range r.styles {
		if si == nil {
			continue
		}
		s := &model.Style{ID: r.styleIDs[i], Name: firstAlias(si.name), Hidden: si.hidden}
		switch si.stk {
		case stkPara:
			s.Type = model.ParagraphStyle
		case stkChar:
			s.Type = model.CharacterStyle
		case stkTable:
			s.Type = model.TableStyle
		case stkList:
			s.Type = model.NumberingStyle
		default:
			continue
		}
		if s.Name == "" {
			s.Name = s.ID
		}
		s.BasedOn = r.styleID(si.istdBase)
		s.Next = r.styleID(si.istdNxt)
		if s.Next == s.ID {
			s.Next = ""
		}
		switch si.sti {
		case 0, 65, 105, 107:
			s.Default = true
		}
		if si.papx != nil && s.Type != model.CharacterStyle {
			var pi papInfo
			r.decodePap(&pi, si.papx)
			s.Para = pi.props
			s.Para.List = pi.listRef()
		}
		c := &chpInfo{istd: -1}
		if si.chpx != nil {
			r.decodeChp(c, si.chpx)
		}
		s.Run = c.props
		st.Styles = append(st.Styles, s)
		pend = append(pend, pending{s, c, si})
	}
	r.ensureNormal()
	for _, pd := range pend {
		var base model.RunProps
		if pd.s.BasedOn != "" {
			if pd.s.Type == model.CharacterStyle {
				base = r.doc.ResolveRun(&model.Paragraph{}, &model.Run{Style: pd.s.BasedOn})
			} else {
				base = r.doc.ResolveRun(&model.Paragraph{Style: pd.s.BasedOn}, &model.Run{})
			}
		}
		pd.s.Run = resolveToggles(pd.chp, base)
	}
}

func firstAlias(name string) string {
	if i := strings.IndexByte(name, ','); i > 0 {
		return name[:i]
	}
	return name
}

func (r *reader) styleID(istd int) string {
	if istd >= 0 && istd < len(r.styleIDs) {
		return r.styleIDs[istd]
	}
	return ""
}

// ensureNormal makes sure there is a default paragraph style.
func (r *reader) ensureNormal() {
	if r.doc.Styles.Default(model.ParagraphStyle) == nil {
		r.doc.Styles.Styles = append([]*model.Style{{ID: "Normal", Name: "Normal", Type: model.ParagraphStyle, Default: true}}, r.doc.Styles.Styles...)
	}
}

// parseStd decodes an STD.
func parseStd(std []byte, cbBase int) *stdInfo {
	if cbBase < 10 {
		cbBase = 10
	}
	si := &stdInfo{
		sti:      int(u16(std) & 0x0FFF),
		stk:      int(u16(std[2:]) & 0xF),
		istdBase: int(u16(std[2:]) >> 4),
		istdNxt:  int(u16(std[4:]) >> 4),
	}
	cupx := int(u16(std[4:]) & 0xF)
	grf := u16(std[8:])
	si.hidden = grf&0x0002 != 0 || grf&0x0100 != 0
	q := cbBase
	if q+2 > len(std) {
		return si
	}
	cch := int(u16(std[q:]))
	if q+2+2*cch > len(std) {
		return si
	}
	u := make([]uint16, cch)
	for i := range u {
		u[i] = u16(std[q+2+2*i:])
	}
	si.name = string(utf16.Decode(u))
	q += 2 + 2*cch + 2
	var upx [][]byte
	for i := 0; i < cupx && q+2 <= len(std); i++ {
		cb := int(u16(std[q:]))
		q += 2
		if q+cb > len(std) {
			break
		}
		upx = append(upx, std[q:q+cb])
		q += cb
		if cb%2 == 1 {
			q++
		}
	}
	get := func(i int) []byte {
		if i < len(upx) {
			return upx[i]
		}
		return nil
	}
	switch si.stk {
	case stkPara:
		if p := get(0); len(p) >= 2 {
			si.papx = p[2:]
		}
		si.chpx = get(1)
	case stkChar:
		si.chpx = get(0)
	case stkTable:
		if p := get(1); len(p) >= 2 {
			si.papx = p[2:]
		}
		si.chpx = get(2)
	case stkList:
		if p := get(0); len(p) >= 2 {
			si.papx = p[2:]
		}
	}
	return si
}
