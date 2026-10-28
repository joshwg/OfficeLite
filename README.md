# OfficeLite

Lightweight, macro-free Office apps in Go. MIT licensed. Version 0.7
(`common/version`; `officelite --version`).

- **ExcelLite** (working): opens `.xlsx`/`.xlsm`, `.xls` (Excel 97-2003) and
  `.csv`/`.tsv` and shows them the way Excel does: cell formatting, number
  formats, merged cells, column widths/row heights, frozen panes, multiple
  sheets. Up to 256K rows × 2K columns per sheet. Cells can be edited as in
  Excel (see Editing below), formatted from a Home ribbon, and saved back as
  `.xlsx`, `.xls`, `.csv` or tab-delimited text, or exported to PDF.
  A calculation engine (255 functions) computes formulas that have no stored
  value on open, and recalculates everything with F9.
- **WordLite** (working): opens, edits and saves `.docx`, `.doc` (Word
  97-2003) and `.txt` in a page view with Word's styles: style gallery,
  Modify Style, New Style, Update Style to Match Selection, Delete Style
  and a Styles pane, fonts, sizes, bold/italic/underline/strike,
  sub/superscript, highlight, change case, color, alignment, spacing,
  indents, tabs, bullets and numbering, undo, clipboard (Paste Special /
  Keep Text Only), Format Painter, Find & Replace and Go To. Headers and
  footers (default, first page, odd/even) are shown and edited
  (double-click the top or bottom margin), with PAGE / NUMPAGES fields
  showing each page's number. Insert menu: tables (grid picker or Insert
  Table…, insert/delete rows and columns, Tab between cells), pictures
  (shown, selectable, resizable by their handles, saved in `.docx`),
  hyperlinks (Ctrl+K; Ctrl+click opens), symbols, date & time, page/line
  breaks. File > Export > PDF writes the pages as laid out; File > Print
  goes through a PDF (see below). No macros. Footnotes and comments are
  kept on a `.docx` re-save but not shown or edited; other fields show
  their last result; EMF/WMF pictures, shapes and charts show as
  placeholders.

Both are one program, `officelite`: each file opens in its own window, chosen
by the file's content (not its extension), and all windows share one process
and font cache.

## Running

```sh
./officelite.sh Samples/Book1.xlsx a.xls   # build + open (one window each)
./officelite.sh Samples/Word-Sample.docx   # a document
./officelite.sh                            # blank workbook
./officelite.sh --word                     # blank document
./officelite.sh --excel notes.txt          # force spreadsheet (or --word)
./officelite.sh --linux file.csv           # native Linux build (needs GUI headers)
./officelite.sh --build                    # build only
```

`Samples/` has workbooks and documents for trying the features (cell
formatting, notes and hyperlinks, tables/pictures/links, headers/footers
and styles, `.doc` layouts).

Under WSL the default is a native Windows executable (`bin/OfficeLite.exe`),
which needs nothing beyond Go. The Linux build uses CGO and the X11/Wayland
headers; `officelite.sh --linux` prints the `apt install` line if they are
missing.

### Menu bar, Home ribbon and File page

The menu bar under the title has **File**, **Home** (Undo, Redo, Cut, Copy,
Paste, Clear Contents, Select All, Insert/Delete Sheet Rows and Columns,
Insert/Delete/Rename Sheet, Fill Down/Right, Sort A to Z / Z to A, Custom
Sort…, Find…, Replace…, Go To…), **Insert** (Insert Cells… / Delete Cells…
with shift right/down/left/up or entire row/column, Insert Sheet
Rows/Columns, Insert Sheet, Function…, Symbol…, Hyperlink…, Remove
Hyperlinks, New/Edit Note, Delete Notes), **Formulas** (Calculate Now) and
**View** (zoom, gridlines).

Under the menu bar the **Home ribbon** has Excel's Home tab groups:
Clipboard (Paste ▾ with Paste Values / Formulas / Formatting, Cut, Copy,
Format Painter — double-click keeps it on until Esc), Font (font, size,
bold, italic, underline, strikethrough, borders, fill and font color),
Alignment (vertical and horizontal alignment, wrap text, indent, Merge &
Center ▾ with Merge Across / Merge Cells / Unmerge), Number (General,
Number, Currency, Accounting, dates, time, percentage, fraction,
scientific, text; $ % , and more/fewer decimals), Styles (built-in cell
styles: Good, Bad, Neutral, Title, Headings, Total, Note, Input, Output,
Calculation, Check Cell, Comma, Currency, Percent), Cells (Insert ▾,
Delete ▾, Format ▾: row height, column width, AutoFit, default width,
hide/unhide rows, columns and sheets, rename, Format Cells) and Editing
(AutoSum ▾, Fill ▾, Clear ▾, Sort & Filter ▾, Find & Select ▾). The
controls show the active cell's format; on a narrow window the ribbon
scrolls sideways (wheel or the ‹ › buttons). WordLite's ribbon has Undo,
Clipboard, Font, Paragraph, Styles and Editing groups.

File opens a full-window page, as in Excel, with a side menu:

- **Home**: the New tiles (blank workbook, blank document) and the Recent list
- **New**: blank workbook or document, each in a new window
- **Open**: Browse… plus the full Recent list; a file opens in this window if
  it is still an untouched new workbook, otherwise in a new one (documents go
  to a WordLite window)
- **Info**: file path, type, size, modified time, sheets, used range, cell,
  formula and defined-name counts, date system, items not shown, version
- **Save**: writes the workbook back to its file in that file's format
  (a new workbook goes to Save As)
- **Save As**: Excel Workbook (`.xlsx`) or Excel 97-2003 Workbook (`.xls`),
  all sheets, or CSV or tab-delimited text (values of the active sheet); the
  saved file becomes the window's document. Before saving in a format that
  can't hold everything, a prompt lists what would be lost (once per file)
- **Print**: Print via PDF — the sheet (or workbook, or document) is written
  to a temporary PDF and opened in the default PDF viewer to print from
  there; direct printing isn't available yet
- **Export**: CSV or tab-delimited copy of the active sheet, or **PDF**: the
  active sheet or the entire workbook as Excel prints it (used range, Letter
  pages down then across, Excel's normal margins, column widths, row heights,
  formatting, borders, merged cells; gridlines and "Fit all columns on one
  page" optional), with the fonts embedded (subsetted) and the text
  searchable; the PDF opens in the default viewer afterwards (optional).
  WordLite exports its pages as laid out (tables, shading, pictures,
  headers/footers)
- **Close**: closes the window

The Recent list (up to 50 files, both apps) is kept in the user's config
directory, `%AppData%\OfficeLite\recent.json` on Windows.

Keys: arrows / Shift+arrows / Ctrl+arrows, PageUp/PageDown, Home, Ctrl+Home,
Ctrl+End, Tab/Enter, Ctrl+A, Ctrl+C (copies the selection as tab-separated
text), Ctrl+PageUp/PageDown (sheets), Ctrl+N (new window), Ctrl+O (Open page),
Ctrl+F12 (Open dialog), Ctrl+S (save), F12 (Save As), Ctrl+P (Print),
Ctrl+W (close), Esc (leave the File page / menu), Ctrl+wheel or
Ctrl+Alt +/− and Ctrl+0 (zoom; also the status bar's − 100% +), Shift+wheel
(scroll sideways), F9 (recalculate all formulas), Ctrl+F (Find), Ctrl+H
(Replace), Ctrl+G or F5 (Go To), Ctrl+K (hyperlink), Ctrl+D / Ctrl+R (fill
down / right), Shift+F2 (new or edit note), Shift+F3 (Insert Function).

WordLite keys: Ctrl+F find, Ctrl+H replace, Ctrl+G go to page, F3 /
Shift+F3 next / previous match (Enter / Shift+Enter in the find box),
Ctrl+K link, Ctrl+Shift+C / Ctrl+Shift+V copy / paste formatting,
Ctrl+Alt+V Paste Special, Ctrl+Shift+> / < grow / shrink font,
Ctrl+Alt+Shift+S Styles pane, Tab / Shift+Tab between table cells, Esc
leaves header/footer editing, the find bar or the Format Painter.

### Editing

- Typing replaces the active cell's content (Enter mode: the arrow keys
  commit and move); F2, a double-click or a click in the formula bar edit
  the existing content (Edit mode; F2 switches between the modes). The entry
  shows in the cell, growing to the right, and in the formula bar.
- Enter / Shift+Enter commit and move down / up, Tab / Shift+Tab right /
  left, Ctrl+Enter stays; Alt+Enter adds a line break; Esc cancels.
- Entries are typed as Excel types them: numbers (`1,234.5`, `(12)`, `1E3`),
  percentages and currency (which give a General cell a `0%` or `$` format),
  dates and times (`1/15/2026`, `15-Jan`, `3:45 PM`, ...), TRUE/FALSE, error
  values, a leading `'` for text. A leading `=` (or `+`/`-` before a
  reference or function) makes a formula; function names and references are
  upper-cased and missing closing parentheses added. A formula that doesn't
  parse is reported in the status bar and stays in the editor. Typed
  formulas are dynamic arrays (results spill) as in Excel 365.
- Delete clears contents (formats stay); Backspace starts an empty entry.
- Ctrl+X / Ctrl+C / Ctrl+V: cells copied in the window paste with their
  formulas (relative references shifted), values and styles, tiled over a
  selection that is a multiple of the copied size; a cut moves the cells and
  formulas pointing at them follow. Text from other programs (tab-separated
  lines) is pasted as typed entries.
- Ctrl+Z / Ctrl+Y undo and redo every change (100 levels).
- Formatting applies to the whole selection as one undoable step; empty
  cells keep their format. Ctrl+B / Ctrl+I / Ctrl+U / Ctrl+5 toggle bold,
  italic, underline, strikethrough; Ctrl+Shift+~ $ % # @ ! set General,
  currency, percent, date, time and number formats; Ctrl+1 opens Format
  Cells (Number, Alignment, Font, Border, Fill). Rows without their own
  height grow to fit a larger font or wrapped text.
- Drag a column or row header divider (the cursor changes over it) to resize;
  the status bar shows the size as Excel's tooltip does. Dragging inside a
  selection of whole columns or rows resizes all of them; dragging to zero
  hides. Double-clicking a divider fits the column to its widest text (the
  row to its tallest), as Excel's AutoFit does. Undoable.
- Ctrl+Shift+= / Ctrl+- insert / delete the selected rows (columns when whole
  columns are selected). References everywhere in the workbook, defined
  names, merged cells, sizes, frozen panes and spills follow; references to
  deleted cells become `#REF!`.
- Insert Cells / Delete Cells shift the cells right, down, left or up;
  references into the moved cells follow them (a reference partly outside
  the moved strip is left alone), and references to deleted cells become
  `#REF!`.
- Fill: Ctrl+D / Ctrl+R copy the top row / left column (formulas shifted).
  Dragging the fill handle continues a series: number trends (1, 3 → 5, 7),
  dates by day, day and month names, text ending in a number (Item1 →
  Item2, Q01 → Q02); anything else is copied with formulas shifted.
  Dragging back into the selection clears cells.
- Sort A to Z / Z to A sorts the selection (or, for a single cell, the data
  around it) by the active cell's column, guessing a header row. Custom
  Sort takes up to three keys, a header option and case sensitivity. Whole
  rows of the range move with their formats, notes and links, and relative
  references in moved formulas shift. Blanks sort last.
- Find & Replace: within the sheet (or the selection) or the whole
  workbook, looking in formulas or values, with match case, match entire
  cell and `*` `?` `~` wildcards; Find Next, Find All, Replace, and Replace
  All as one undo step. Go To accepts B12, Sheet2!A1:C5, C:D, 3:7 or a
  defined name.
- Notes: New Note (Shift+F2) adds a note, marked by a red triangle in the
  cell's corner and shown on hover or when the cell is selected. Hyperlinks
  (Ctrl+K): text to display, address (URL, mailto:, file, or #Sheet2!A1)
  and ScreenTip; the cell takes the blue underlined Hyperlink style, and
  Ctrl+click follows the link. Notes and links are kept in `.xlsx` and
  `.xls`.
- Insert Function lists the implemented functions by category with a
  search box and inserts `=NAME(`; Insert Symbol types a symbol into the
  cell entry.
- Shift+F11 inserts a sheet; double-click a tab to rename it. Sheet renames
  and deletions are applied to formulas.
- Formulas are recalculated after every change, incrementally: only those
  that depend on the changed cells (an edit takes a few milliseconds in a
  workbook of 100,000 formulas; inserting a row there takes ~0.3 s).
- The title shows `•` while there are unsaved changes; closing the window
  then asks to save.

## Calculation

`excel/formula` parses and evaluates formulas with Excel's semantics: type
coercion, dynamic arrays (element-wise "lifting", spilling, `A1#`), LET and
LAMBDA, defined names, criteria (`">=5"`, wildcards), 1900/1904 dates.
`excel/calc` recalculates a workbook in dependency order, detects circular
references and reports `#SPILL!`. Functions, by category
(`excel/formula/functions/*`): date & time, information, logical (incl.
MAP/REDUCE/SCAN/BYROW/BYCOL/MAKEARRAY), math & trig, statistics used by
SUBTOTAL/AGGREGATE (COUNT, COUNTA, COUNTIFS, AVERAGE, ...), text (incl.
REGEX*, TEXTSPLIT), lookup (XLOOKUP, FILTER, UNIQUE, SORT, VLOOKUP, INDEX,
MATCH, ...) and FV/NPV/PMT. STOCKHISTORY, TRANSLATE and DETECTLANGUAGE need
Microsoft's online services and return `#CONNECT!`.

`calccheck` recalculates workbooks and diffs the results against the values
Excel saved — any real workbook becomes a test:

```sh
go run ./excel/cmd/calccheck Samples/Book1.xlsx
```

`excelshot` renders a sheet to a PNG without a window, for checking rendering:

```sh
GOOS=windows go build -o bin/excelshot.exe ./excel/cmd/excelshot
bin/excelshot.exe -o out.png -w 1280 -h 800 Samples/Book1.xlsx
```

## Layout

```
cmd/officelite/    the app: routes each file to an Excel or Word window;
                   its icon (officelite.png/.ico) and the Windows resource
                   linked into the .exe (rsrc_windows_amd64.syso)
cmd/mkicon/        draws the icon (go generate ./cmd/officelite)
common/            shared by Excel and Word
  colors/            color parsing and mixing
  doctype/           spreadsheet vs. document detection
  gui/               Gio helpers: font library, painting, scrollbar, theme,
                     menu bar, ribbon (shared by both apps), File backstage
                     (Home/New/Open pages), native Open/Save dialogs
  pdf/               PDF 1.7 writer: embedded subsetted fonts, images (PDF
                     export)
  recent/            recent-files list shared by all windows
  version/           the software version
  ole/               OLE2 compound files (.xls, .doc containers)
  textenc/           text decoding (BOMs, UTF-16, Windows-1252)
excel/
  model/             workbook/sheet/cell/style model, size limits, units
  fileio/            Open/Read: picks the reader from the file content;
                     Write/SaveFile: .xlsx, .csv, tab-delimited text
    xlsxfile/          .xlsx (streamed sheet XML; excelize for styles,
                       notes and hyperlinks)
    xlsfile/           .xls BIFF8 reader and writer (incl. notes, links)
    csvfile/           .csv/.tsv (delimiter sniffing, Excel-style typing)
  numfmt/            Excel number formats (General, dates, currency, ...)
  formula/           formula engine: parser, values, evaluator, criteria, dates
    functions/         worksheet functions, one package per category
  calc/              workbook recalculation, spilling, circular references,
                     incremental recalculation after edits
  edit/              editing: typed input, clipboard and paste modes,
                     formatting, insert/delete rows, columns, cells and
                     sheets, fill, sort, find/replace, notes and links,
                     undo/redo
  render/            draws the grid: panes, fills, text, borders, selection;
                     AutoFit measuring; PDF export (pdf.go)
  ui/                window: title/menu/formula bars, Home ribbon, dialogs
                     (Format Cells, Insert, Find, Sort, ...), tabs, status
                     bar, input, File pages (Info, Save As, Print, Export)
  cmd/excelshot/     headless PNG renderer (dev tool)
  cmd/calccheck/     recalculate and compare with Excel's saved values
word/              WordLite
  model/             document model: blocks, runs, styles, lists; editing
                     operations and undo (edit.go); table editing,
                     pictures and links, find, Format Painter,
                     headers/footers, style management
  fileio/            Open/Read/Write/SaveFile, picks the reader by content
    docxfile/          .docx/.docm/.dotx (keeps unmodeled parts on re-save)
    docfile/           .doc Word 97-2003 binary (FIB, piece table, STSH)
  layout/            line breaking, tabs, lists, tables, pagination,
                     headers/footers and page fields, pictures, drawing;
                     PDF export (pdf.go)
  ui/                window: Home ribbon, Insert menu and dialogs, page
                     view, caret/selection, find bar, header/footer
                     editing, Styles pane, File pages
  cmd/wordshot/      headless PNG renderer (dev tool)
Fonts/             local fonts, not committed (see Fonts/README.md)
Samples/           sample workbooks and documents for trying features by
                   hand (Go test fixtures live in each package's testdata/)
```

## Fonts

Fonts are indexed from `./Fonts`, `OFFICELITE_FONTS`, the OS font folders and
Office's cloud-font cache (where Aptos lives); only the families a workbook
uses are loaded. Embedded bitmap strikes (Calibri has them) are ignored so
text always renders from outlines.

## Not yet supported

No macros (VBA is never run; `.xlsm`/`.docm` macros are dropped on save, with
a warning) and no functions beyond the 255 implemented.

Both apps: direct printing (Print goes through a PDF opened in the default
viewer), PDF paper is Letter only (no page setup is read), password-protected
or encrypted files, Excel 95 / Word 95 and older binary formats.

ExcelLite:
- Not shown and lost on save: charts, images, pivot tables, tables (ListObjects),
  conditional formatting, data validation, threaded comments (read as their
  note text), row/column default styles, rich text runs within a cell.
  Rotated text is kept but drawn flat.
- Calculation: 3-D references (`Sheet1:Sheet3!A1`) and union references,
  hidden-row awareness in SUBTOTAL/AGGREGATE, PCRE-only regex features (Go's
  RE2 is used); STOCKHISTORY/TRANSLATE/DETECTLANGUAGE return `#CONNECT!`.
- Editing: references can't be picked with the mouse while typing a
  formula; pasting doesn't copy merges; notes and links don't move with
  copy/paste or fill; notes are plain text (rich text flattened, box size
  and position not kept); Ctrl+Shift+= / Ctrl+- insert/delete whole rows
  (the Insert Cells dialog is on the Insert menu and ribbon). Sort is top
  to bottom only (no sort by color or custom lists, no merged cells); Find
  searches by rows and not by format; AutoFill copies a single number (no
  Ctrl+drag series) and has no Auto Fill Options button.
- Formatting: a whole-column or whole-row selection formats only the used
  range (no row/column default styles); cell styles are applied as direct
  formats, not saved as named styles; no conditional formatting, Format as
  Table, Fill Up/Left/Series or filters; theme colors and custom colors
  aren't offered; rows grow but don't shrink automatically and merged
  cells don't auto-fit; Unhide restores the default size.
- PDF export: no print titles, page headers/footers, custom print areas or
  charts/images.
- `.xls` save: BIFF8 limits (65,536 × 256, 4,000 styles); dynamic arrays
  that spill become ordinary formulas (Excel shows `=@...`); LET/LAMBDA
  formulas and references to other workbooks are saved as values only.
  `fileio.Fits` lists what won't fit before saving.

WordLite:
- Not shown or edited (kept on a `.docx` re-save): footnotes and endnotes,
  comments, tracked changes (shown accepted), text boxes and floating shapes
  (inline placeholders; floating pictures are drawn inline), multiple
  sections/columns (the last section's headers/footers are used).
- Fields other than PAGE / NUMPAGES / SECTIONPAGES show their last result.
- Headers/footers: creating or removing one and the first-page / odd-even
  switches aren't undoable. Find doesn't match across paragraph marks.
  Modify… in the Styles pane edits paragraph styles only.
- Resizing a picture from a `.docx` rewrites it (its crop and effects are
  lost).
- No right-to-left text or IME composition; table rows don't split across
  pages and header rows don't repeat.
- `.doc` save: pictures and fields from `.docx` become placeholder text,
  headers/footers are dropped,
  nested tables are flattened, table styles and cell borders beyond on/off
  are dropped (`docfile.Warnings` lists these before saving).
