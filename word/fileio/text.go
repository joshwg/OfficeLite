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
	var blocks func([]model.Block)
	blocks = func(bs []model.Block) {
		for _, b := range bs {
			switch b := b.(type) {
			case *model.Paragraph:
				bw.WriteString(strings.NewReplacer("\n", "\r\n", "\f", "\r\n", "￼", "").Replace(b.Text()))
				bw.WriteString("\r\n")
			case *model.Table:
				for _, row := range b.Rows {
					for i, c := range row.Cells {
						if i > 0 {
							bw.WriteString("\t")
						}
						var parts []string
						model.WalkBlocks(c.Blocks, func(p *model.Paragraph) bool {
							parts = append(parts, p.Text())
							return true
						})
						bw.WriteString(strings.Join(parts, " "))
					}
					bw.WriteString("\r\n")
				}
			}
		}
	}
	blocks(doc.Body)
	return bw.Flush()
}
