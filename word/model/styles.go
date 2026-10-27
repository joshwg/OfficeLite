package model

// DefaultStyles is the style sheet of a new blank Word document: Normal
// (Aptos/Calibri 11pt, 8pt after, 1.08 lines), headings, title, quote,
// list paragraph and the common character styles.
func DefaultStyles() StyleSheet {
	heading := func(n int, size float64, color string, before float64) *Style {
		id := "Heading" + string(rune('0'+n))
		return &Style{
			ID: id, Name: "heading " + string(rune('0'+n)), Type: ParagraphStyle, BasedOn: "Normal", Next: "Normal",
			Para: ParaProps{SpaceBefore: F(before), SpaceAfter: F(4), KeepNext: On, KeepLines: On, OutlineLevel: n},
			Run:  RunProps{Font: "Calibri Light", Size: size, Color: color},
		}
	}
	return StyleSheet{
		DefaultRun:  RunProps{Font: "Calibri", Size: 11},
		DefaultPara: ParaProps{SpaceAfter: F(8), Line: 1.08, LineRule: LineAuto, WidowControl: On},
		Styles: []*Style{
			{ID: "Normal", Name: "Normal", Type: ParagraphStyle, Default: true},
			heading(1, 16, "2F5496", 12),
			heading(2, 13, "2F5496", 2),
			heading(3, 12, "1F3763", 2),
			{ID: "Title", Name: "Title", Type: ParagraphStyle, BasedOn: "Normal", Next: "Normal",
				Para: ParaProps{SpaceAfter: F(0), Line: 1, LineRule: LineAuto},
				Run:  RunProps{Font: "Calibri Light", Size: 28}},
			{ID: "Subtitle", Name: "Subtitle", Type: ParagraphStyle, BasedOn: "Normal", Next: "Normal",
				Run: RunProps{Color: "5A5A5A", Size: 11}},
			{ID: "Quote", Name: "Quote", Type: ParagraphStyle, BasedOn: "Normal", Next: "Normal",
				Para: ParaProps{Align: AlignCenter, SpaceBefore: F(10)},
				Run:  RunProps{Italic: On, Color: "404040"}},
			{ID: "ListParagraph", Name: "List Paragraph", Type: ParagraphStyle, BasedOn: "Normal",
				Para: ParaProps{IndentLeft: F(36)}},
			{ID: "NoSpacing", Name: "No Spacing", Type: ParagraphStyle,
				Para: ParaProps{SpaceAfter: F(0), Line: 1, LineRule: LineAuto}},
			{ID: "DefaultParagraphFont", Name: "Default Paragraph Font", Type: CharacterStyle, Default: true, Hidden: true},
			{ID: "Strong", Name: "Strong", Type: CharacterStyle, BasedOn: "DefaultParagraphFont", Run: RunProps{Bold: On}},
			{ID: "Emphasis", Name: "Emphasis", Type: CharacterStyle, BasedOn: "DefaultParagraphFont", Run: RunProps{Italic: On}},
			{ID: "Hyperlink", Name: "Hyperlink", Type: CharacterStyle, BasedOn: "DefaultParagraphFont",
				Run: RunProps{Color: "0563C1", Underline: UnderlineSingle}},
			{ID: "TableNormal", Name: "Normal Table", Type: TableStyle, Default: true, Hidden: true},
			{ID: "TableGrid", Name: "Table Grid", Type: TableStyle, BasedOn: "TableNormal"},
		},
	}
}
