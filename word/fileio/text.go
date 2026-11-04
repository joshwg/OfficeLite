package fileio

import (
	"bufio"
	"io"
	"strings"

	"officelite/common/textenc"
	"officelite/word/model"
)

// readText turns plain text into a document, one paragraph per line.
func readText(data []byte) *model.Document {
	doc := model.New("")
	doc.Body = nil
	decoded, err := textenc.Decode(data)
	if err != nil {
		decoded = string(data)
	}
	text := strings.ReplaceAll(decoded, "\r\n", "\n")
	for _, line := range strings.Split(text, "\n") {
		p := &model.Paragraph{}
		for i, part := range strings.Split(line, "\t") {
			if i > 0 {
				p.Runs = append(p.Runs, model.Run{Kind: model.RunTab})
			}
			if part != "" {
				p.Runs = append(p.Runs, model.Run{Text: part})
			}
		}
		doc.Body = append(doc.Body, p)
	}
	doc.SourceFormat = "txt"
	return doc
}

// writeText writes each paragraph as a CRLF-terminated line; table cells
// are separated by tabs.
func writeText(doc *model.Document, w io.Writer) error {
	bw := bufio.NewWriter(w)
	var writeErr error
	write := func(s string) {
		if writeErr == nil {
			_, writeErr = bw.WriteString(s)
		}
	}
	blocks := func(bs []model.Block) {
		for _, b := range bs {
			switch b := b.(type) {
			case *model.Paragraph:
				write(strings.NewReplacer("\n", "\r\n", "\f", "\r\n").Replace(plainText(b)))
				write("\r\n")
			case *model.Table:
				for _, row := range b.Rows {
					for i, c := range row.Cells {
						if i > 0 {
							write("\t")
						}
						var parts []string
						model.WalkBlocks(c.Blocks, func(p *model.Paragraph) bool {
							parts = append(parts, plainText(p))
							return true
						})
						write(strings.Join(parts, " "))
					}
					write("\r\n")
				}
			}
		}
	}
	blocks(doc.Body)
	if writeErr != nil {
		return writeErr
	}
	return bw.Flush()
}

// plainText is a paragraph's text for a .txt file: fields and hyperlinks
// read from a .doc are opaque runs whose Text is what they show, while
// pictures, shapes and the like have only a "[picture]" placeholder,
// which is left out.
func plainText(p *model.Paragraph) string {
	var b strings.Builder
	for _, r := range p.Runs {
		switch r.Kind {
		case model.RunText:
			b.WriteString(r.Text)
		case model.RunTab:
			b.WriteByte('\t')
		case model.RunBreak:
			b.WriteByte('\n')
		case model.RunPageBreak:
			b.WriteByte('\f')
		case model.RunOpaque:
			placeholder := strings.HasPrefix(r.Text, "[") && strings.HasSuffix(r.Text, "]") &&
				(r.Opaque == nil || r.Opaque.Field == "")
			if !placeholder && (r.Opaque == nil || r.Opaque.Picture == nil) {
				b.WriteString(r.Text)
			}
		}
	}
	return b.String()
}
