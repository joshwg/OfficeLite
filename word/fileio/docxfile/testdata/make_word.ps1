# Regenerates testdata/word.docx with Microsoft Word (COM). Run on Windows:
#   powershell -NoProfile -File make_word.ps1   (put pic.png, any small PNG, in $dir first)
# then copy $dir\word.docx here.
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
  $sel.Style = $doc.Styles.Item(-63)
  $sel.TypeText('Word Fixture'); $sel.TypeParagraph()
  $sel.Style = $doc.Styles.Item(-1)
  $sel.TypeText('TOCHERE'); $sel.TypeParagraph()
  $sel.Style = $doc.Styles.Item(-2)
  $sel.TypeText('First Heading'); $sel.TypeParagraph()
  $sel.Style = $doc.Styles.Item(-1)
  $sel.TypeText('Some text with ')
  $sel.Font.Bold = 1; $sel.TypeText('bold'); $sel.Font.Bold = 0
  $sel.TypeText(' and a ')
  $doc.Hyperlinks.Add($sel.Range, 'https://example.com/', '', 'Example tip', 'hyperlink') | Out-Null
  $sel.EndKey(6) | Out-Null
  $sel.TypeText('. Footnote here')
  $doc.Footnotes.Add($sel.Range, [Type]::Missing, 'The footnote text.') | Out-Null
  $sel.EndKey(6) | Out-Null
  $sel.TypeText(' and page ')
  $doc.Fields.Add($sel.Range, 33, '', $false) | Out-Null
  $sel.EndKey(6) | Out-Null
  $sel.TypeParagraph()
  $sel.Range.ListFormat.ApplyBulletDefault()
  $sel.TypeText('Bullet A'); $sel.TypeParagraph()
  $sel.TypeText('Bullet B'); $sel.TypeParagraph()
  $sel.Range.ListFormat.RemoveNumbers()
  $sel.Range.ListFormat.ApplyNumberDefault()
  $sel.TypeText('Number 1'); $sel.TypeParagraph()
  $sel.TypeText('Number 2'); $sel.TypeParagraph()
  $sel.Range.ListFormat.RemoveNumbers()
  $sel.Style = $doc.Styles.Item(-3)
  $sel.TypeText('Table Section'); $sel.TypeParagraph()
  $sel.Style = $doc.Styles.Item(-1)
  $t = $doc.Tables.Add($sel.Range, 3, 3)
  $t.Borders.Enable = 1
  for ($r=1; $r -le 3; $r++) { for ($c=1; $c -le 3; $c++) { $t.Cell($r,$c).Range.Text = "R${r}C${c}" } }
  $t.Cell(2,1).Shading.BackgroundPatternColor = 0x00FFFF
  $t.Cell(1,1).Merge($t.Cell(1,2))
  $t.Cell(2,3).Merge($t.Cell(3,3))
  $sel.EndKey(6) | Out-Null
  $sel.TypeText('After table, picture: ')
  $sel.InlineShapes.AddPicture("$dir\pic.png") | Out-Null
  $sel.EndKey(6) | Out-Null
  $sel.TypeParagraph()
  $sel.InsertBreak(7)
  $sel.TypeText('Second page. ')
  $sel.Font.Italic = 1; $sel.Font.Color = 255; $sel.TypeText('italic red'); $sel.Font.Italic = 0; $sel.Font.Color = -16777216
  $doc.Bookmarks.Add('MyMark', $sel.Range) | Out-Null
  $sel.TypeParagraph()
  $sel.TypeText('CCHERE')
  $sel.TypeParagraph()
  $doc.Comments.Add($doc.Paragraphs.Item(4).Range, 'A comment') | Out-Null
  $sel.EndKey(6) | Out-Null
  $doc.TrackRevisions = $true
  $sel.TypeText('Tracked insertion.')
  $doc.TrackRevisions = $false
  $doc.Sections.Item(1).Headers.Item(1).Range.Text = 'Header text'
  $doc.Sections.Item(1).Footers.Item(1).Range.Text = 'Footer text'
  $f = $doc.Content.Find
  $rng = $doc.Content
  if ($rng.Find.Execute('CCHERE')) { $cc = $doc.ContentControls.Add(1, $rng); $cc.Range.Text = 'Inside a content control' }
  $rng = $doc.Content
  if ($rng.Find.Execute('TOCHERE')) { $rng.Text = ''; $doc.TablesOfContents.Add($rng, $true, 1, 3) | Out-Null }
  $doc.SaveAs2("$dir\word.docx", 16)
  $doc.Close(0)
  'saved'
} catch {
  "ERROR: $_"
} finally {
  $word.Quit()
  [System.Runtime.InteropServices.Marshal]::ReleaseComObject($word) | Out-Null
  Start-Sleep -Seconds 2
  foreach ($p in $mine) { if (Get-Process -Id $p -ErrorAction SilentlyContinue) { Stop-Process -Id $p -Force; "killed $p" } }
}
