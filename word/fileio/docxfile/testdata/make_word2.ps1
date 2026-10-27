# Regenerates testdata/word2.docx with Microsoft Word (COM); see make_word.ps1.
$ErrorActionPreference = 'Stop'
$dir = 'C:\Users\josh\AppData\Local\Temp\officelite-docx'
$before = @(Get-Process WINWORD -ErrorAction SilentlyContinue | ForEach-Object Id)
$word = New-Object -ComObject Word.Application
$mine = @(Get-Process WINWORD -ErrorAction SilentlyContinue | Where-Object { $before -notcontains $_.Id } | ForEach-Object Id)
"started pids: $mine"
try {
  $word.Visible = $false
  $word.DisplayAlerts = 0
  $doc = $word.Documents.Add()
  $sel = $word.Selection
  $sel.TypeText('Delete THIS word. Symbol: ')
  $sel.InsertSymbol(-4000, 'Wingdings', $true)
  $sel.TypeText(' Equation: ')
  $r = $sel.Range
  $r.Text = 'a^2+b^2=c^2'
  $m = $doc.OMaths.Add($r)
  $m.OMaths.Item(1).BuildUp()
  $sel.EndKey(6) | Out-Null
  $sel.TypeText(' Endnote')
  $doc.Endnotes.Add($sel.Range, [Type]::Missing, 'Endnote text') | Out-Null
  $sel.EndKey(6) | Out-Null
  $sel.TypeText(' Inline control: ')
  $sel.TypeText('CC end.')
  $rng = $doc.Range($sel.Start - 7, $sel.Start - 5)
  $cc = $doc.ContentControls.Add(0, $rng)
  $cc.Range.Text = 'inline value'
  $sel.EndKey(6) | Out-Null
  $sel.TypeParagraph()
  $sel.TypeText('Paragraph with a text box anchored.')
  $tb = $doc.Shapes.AddTextbox(1, 100, 200, 150, 60, $sel.Range)
  $tb.TextFrame.TextRange.Text = 'Text box content'
  $sel.EndKey(6) | Out-Null
  $sel.TypeParagraph()
  $sel.ParagraphFormat.Alignment = 3
  $sel.ParagraphFormat.SpaceAfter = 18
  $sel.ParagraphFormat.LineSpacingRule = 1
  $sel.Font.Name = 'Courier New'; $sel.Font.Size = 9; $sel.Font.Underline = 3; $sel.Font.SmallCaps = 1; $sel.Font.StrikeThrough = 1
  $sel.Font.Superscript = 0
  $sel.TypeText('Justified, double spaced, courier 9pt double underline small caps strike.')
  $sel.Font.Underline = 0; $sel.Font.SmallCaps = 0; $sel.Font.StrikeThrough = 0; $sel.Font.Name = 'Aptos'; $sel.Font.Size = 12
  $sel.Range.HighlightColorIndex = 7
  $sel.TypeParagraph()
  $sel.ParagraphFormat.Alignment = 0
  $sel.ParagraphFormat.LineSpacingRule = 0
  $sel.ParagraphFormat.TabStops.Add(144, 2, 1) | Out-Null
  $sel.TypeText("Tab`tright with dots")
  $sel.TypeParagraph()
  $sel.InsertBreak(2)
  $doc.Sections.Item(2).PageSetup.Orientation = 1
  $doc.Sections.Item(2).PageSetup.TextColumns.SetCount(2)
  $sel.TypeText('Landscape two-column section. ' * 20)
  $doc.TrackRevisions = $true
  $rng = $doc.Content
  if ($rng.Find.Execute('THIS ')) { $rng.Delete() | Out-Null }
  $doc.TrackRevisions = $false
  $doc.SaveAs2("$dir\word2.docx", 16)
  $doc.Close(0)
  'saved'
} catch {
  "ERROR: $_ $($_.InvocationInfo.ScriptLineNumber)"
} finally {
  $word.Quit()
  [System.Runtime.InteropServices.Marshal]::ReleaseComObject($word) | Out-Null
  Start-Sleep -Seconds 5
  foreach ($p in $mine) { if (Get-Process -Id $p -ErrorAction SilentlyContinue) { Stop-Process -Id $p -Force; "killed $p" } }
}
