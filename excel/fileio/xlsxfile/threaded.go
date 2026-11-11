// threaded.go reads and writes Excel 365 threaded comments: the
// xl/threadedComments parts (one per sheet, every post with its author's
// person ID, time, and the post it replies to) and xl/persons/person.xml
// (the authors' names). Each thread also has a legacy comment whose
// author is "tc=" + the thread's ID, which older versions of Excel show.

package xlsxfile

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"officelite/excel"
	"officelite/excel/model"
)

const (
	relThreaded    = "http://schemas.microsoft.com/office/2017/10/relationships/threadedComment"
	relPerson      = "http://schemas.microsoft.com/office/2017/10/relationships/person"
	ctThreaded     = "application/vnd.ms-excel.threadedcomments+xml"
	ctPerson       = "application/vnd.ms-excel.person+xml"
	nsThreaded     = "http://schemas.microsoft.com/office/spreadsheetml/2018/threadedcomments"
	personPart     = "xl/persons/person.xml"
	threadedLayout = "2006-01-02T15:04:05.00"

	// threadFallback starts the legacy comment Excel writes for a thread
	// (the same words), so older versions show the discussion.
	threadFallback = "[Threaded comment]\n\nYour version of Excel allows you to read this threaded comment; however, any edits to it will get removed if the file is opened in a newer version of Excel. Learn more: https://go.microsoft.com/fwlink/?linkid=870924\n\n"
)

// xPersons is xl/persons/person.xml.
type xPersons struct {
	List []struct {
		Name string `xml:"displayName,attr"`
		ID   string `xml:"id,attr"`
	} `xml:"person"`
}

// xThreaded is a threadedComments part.
type xThreaded struct {
	List []struct {
		Ref      string `xml:"ref,attr"`
		Time     string `xml:"dT,attr"`
		PersonID string `xml:"personId,attr"`
		ID       string `xml:"id,attr"`
		ParentID string `xml:"parentId,attr"`
		Done     string `xml:"done,attr"`
		Text     string `xml:"text"`
	} `xml:"threadedComment"`
}

// readPersons reads the comment authors' names by person ID.
func (r *reader) readPersons(name string) {
	var p xPersons
	if err := r.unmarshal(name, &p); err != nil {
		r.wb.Warnings = append(r.wb.Warnings, "comment authors could not be read")
		return
	}
	r.persons = map[string]string{}
	for _, x := range p.List {
		r.persons[strings.ToUpper(x.ID)] = x.Name
	}
}

// readThreaded reads a sheet's threaded comments. They replace the
// legacy comments written for older versions on the same cells.
func (r *reader) readThreaded(name string, sh *model.Sheet) {
	var c xThreaded
	if err := r.unmarshal(name, &c); err != nil {
		r.wb.Warnings = append(r.wb.Warnings, sh.Name+": comments could not be read")
		return
	}
	type thread struct {
		row, col int
		note     model.Note
	}
	byID := map[string]*thread{}
	var order []*thread
	for _, x := range c.List {
		text := unescapeOOXML(strings.ReplaceAll(x.Text, "\r\n", "\n"))
		post := model.Post{Author: r.persons[strings.ToUpper(x.PersonID)], Text: text, Time: parseThreadTime(x.Time)}
		if x.ParentID != "" {
			if t := byID[strings.ToUpper(x.ParentID)]; t != nil && len(t.note.Replies) < excel.MaxCommentReplies {
				t.note = t.note.WithReply(post)
			}
			continue
		}
		row, col, ok := parseCellRef(x.Ref)
		if !ok || row >= excel.MaxRows || col >= excel.MaxCols {
			continue
		}
		t := &thread{row: row, col: col, note: model.Note{Threaded: true, Author: post.Author, Text: post.Text, Time: post.Time, Resolved: xmlBool(x.Done)}}
		byID[strings.ToUpper(x.ID)] = t
		order = append(order, t)
	}
	for _, t := range order {
		sh.SetNote(t.row, t.col, &t.note)
	}
}

// parseThreadTime parses a post's dT attribute; zero if it can't.
func parseThreadTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", time.RFC3339Nano} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// guid makes a new random GUID in Excel's {XXXXXXXX-…} form.
func guid() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0F | 0x40
	b[8] = b[8]&0x3F | 0x80
	return fmt.Sprintf("{%X-%X-%X-%X-%X}", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// pendingThreads is a sheet's threaded comments to write.
type pendingThreads struct {
	part string // the worksheet part
	xml  strings.Builder
}

// threadLegacy adds the thread n on cell k to the sheet's threaded
// comments part and returns the legacy comment's author and text.
func (x *writer) threadLegacy(part string, k [2]int, n model.Note) (author, text string) {
	if x.threads == nil || x.threads[len(x.threads)-1].part != part {
		x.threads = append(x.threads, &pendingThreads{part: part})
	}
	pt := x.threads[len(x.threads)-1]
	ref := model.CellName(k[0], k[1])
	top := guid()
	var fb strings.Builder
	fb.WriteString(threadFallback)
	for i, p := range n.Posts() {
		attrs := fmt.Sprintf(` ref="%s" dT="%s" personId="%s" id="%s"`, ref, threadTime(p.Time), x.personID(p.Author), top)
		label := "Comment:"
		if i > 0 {
			attrs = fmt.Sprintf(` ref="%s" dT="%s" personId="%s" id="%s" parentId="%s"`, ref, threadTime(p.Time), x.personID(p.Author), guid(), top)
			label = "Reply:"
		} else if n.Resolved {
			attrs += ` done="1"`
		}
		fmt.Fprintf(&pt.xml, `<threadedComment%s><text>%s</text></threadedComment>`, attrs, xmlEscape(escapeOOXML(p.Text)))
		fmt.Fprintf(&fb, "%s\n    %s\n", label, strings.ReplaceAll(p.Text, "\n", "\n    "))
	}
	return "tc=" + top, strings.TrimSuffix(fb.String(), "\n")
}

// threadTime writes a post's time; now when it isn't known.
func threadTime(t time.Time) string {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UTC().Format(threadedLayout)
}

// personID returns the person ID of an author, adding the author to the
// workbook's persons.
func (x *writer) personID(name string) string {
	for _, p := range x.persons {
		if p[0] == name {
			return p[1]
		}
	}
	id := guid()
	x.persons = append(x.persons, [2]string{name, id})
	return id
}

// threadParts makes the threaded comments and persons parts and adds the
// edits of the parts that list them.
func (x *writer) threadParts(names map[string]bool, edits map[string][]func([]byte) ([]byte, error)) []part {
	if len(x.threads) == 0 {
		return nil
	}
	add := func(name string, fn func([]byte) ([]byte, error)) { edits[name] = append(edits[name], fn) }
	used := lowerSet(names)
	var parts []part
	var overrides strings.Builder
	n := 0
	for _, pt := range x.threads {
		var name string
		for {
			n++
			name = fmt.Sprintf("xl/threadedComments/threadedComment%d.xml", n)
			if !used[strings.ToLower(name)] {
				break
			}
		}
		parts = append(parts, part{name, []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
			`<ThreadedComments xmlns="` + nsThreaded + `" xmlns:x="` + nsMain + `">` + pt.xml.String() + `</ThreadedComments>`)})
		fmt.Fprintf(&overrides, `<Override PartName="/%s" ContentType="%s"/>`, name, ctThreaded)
		rel := fmt.Sprintf(`<Relationship Id="rIdThreaded1" Type="%s" Target="%s"/>`, relThreaded, relTarget(pt.part, name))
		add(relsName(pt.part), func(b []byte) ([]byte, error) {
			if b == nil {
				return []byte(relsXML(rel)), nil
			}
			return insertBefore(b, "</Relationships>", rel)
		})
	}
	var people strings.Builder
	for _, p := range x.persons {
		fmt.Fprintf(&people, `<person displayName="%s" id="%s" userId="%s" providerId="None"/>`, xmlEscape(p[0]), p[1], xmlEscape(p[0]))
	}
	parts = append(parts, part{personPart, []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<personList xmlns="` + nsThreaded + `" xmlns:x="` + nsMain + `">` + people.String() + `</personList>`)})
	fmt.Fprintf(&overrides, `<Override PartName="/%s" ContentType="%s"/>`, personPart, ctPerson)
	add("[Content_Types].xml", func(b []byte) ([]byte, error) { return insertBefore(b, "</Types>", overrides.String()) })
	add("xl/_rels/workbook.xml.rels", func(b []byte) ([]byte, error) {
		return insertBefore(b, "</Relationships>", `<Relationship Id="rIdPersons1" Type="`+relPerson+`" Target="persons/person.xml"/>`)
	})
	return parts
}
