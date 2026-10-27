# OfficeLite

Lightweight, macro-free Office apps in Go. MIT licensed. Version 0.7
(`common/version`; `officelite --version`).

- **ExcelLite** (working): opens `.xlsx`/`.xlsm`, `.xls` (Excel 97-2003) and
  `.csv`/`.tsv` and shows them the way Excel does: cell formatting, number
  formats, merged cells, column widths/row heights, frozen panes, multiple
  sheets. Up to 256K rows × 2K columns per sheet. Cells can be edited as in
  Excel (see Editing below) and saved back as `.xlsx`, `.xls`, `.csv` or
  tab-delimited text.
  A calculation engine (255 functions) computes formulas that have no stored
  value on open, and recalculates everything with F9.
- **WordLite** (working): opens, edits and saves `.docx`, `.doc` (Word
  97-2003) and `.txt` in a page view with Word's styles: style gallery,
  Modify Style, fonts, sizes, bold/italic/underline/strike, color,
  alignment, spacing, indents, tabs, bullets and numbering, tables, undo,
  clipboard. No macros. Pictures, fields, headers/footers, footnotes and
  comments are kept on a `.docx` re-save but not shown or edited (pictures
  show as placeholders; fields show their last result).

Both are one program, `officelite`: each file opens in its own window, chosen
by the file's content (not its extension), and all windows share one process
and font cache.

## Running

```sh
./officelite.sh Samples/Book1.xlsx a.xls   # build + open (one window each)
./officelite.sh                            # blank workbook
./officelite.sh --word                     # blank document
./officelite.sh --excel notes.txt          # force spreadsheet (or --word)
./officelite.sh --linux file.csv           # native Linux build (needs GUI headers)
./officelite.sh --build                    # build only
```

Under WSL the default is a native Windows executable (`bin/OfficeLite.exe`),
which needs nothing beyond Go. The Linux build uses CGO and the X11/Wayland
headers; `officelite.sh --linux` prints the `apt install` line if they are
missing.

### Menu bar and File page

The menu bar under the title has **File**, **Home** (Undo, Redo, Cut, Copy,
Paste, Clear Contents, Select All, Insert/Delete Sheet Rows and Columns,
Insert/Delete/Rename Sheet),
**Formulas** (Calculate Now) and **View** (zoom, gridlines). File opens a
full-window page, as in Excel, with a side menu:

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
- **Print**: not available yet (placeholder page)
- **Export**: CSV or tab-delimited copy of the active sheet; PDF is listed as
  not available yet
- **Close**: closes the window

The Recent list (up to 50 files, both apps) is kept in the user's config
directory, `%AppData%\OfficeLite\recent.json` on Windows.

Keys: arrows / Shift+arrows / Ctrl+arrows, PageUp/PageDown, Home, Ctrl+Home,
Ctrl+End, Tab/Enter, Ctrl+A, Ctrl+C (copies the selection as tab-separated
text), Ctrl+PageUp/PageDown (sheets), Ctrl+N (new window), Ctrl+O (Open page),
Ctrl+F12 (Open dialog), Ctrl+S (save), F12 (Save As), Ctrl+P (Print),
Ctrl+W (close), Esc (leave the File page / menu), Ctrl+wheel or
Ctrl+Alt +/− and Ctrl+0 (zoom; also the status bar's − 100% +), Shift+wheel
(scroll sideways), F9 (recalculate all formulas).

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
- Ctrl+Shift+= / Ctrl+- insert / delete the selected rows (columns when whole
  columns are selected). References everywhere in the workbook, defined
  names, merged cells, sizes, frozen panes and spills follow; references to
  deleted cells become `#REF!`.
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
cmd/officelite/    the app: routes each file to an Excel or Word window
common/            shared by Excel and Word
  colors/            color parsing and mixing
  doctype/           spreadsheet vs. document detection
  gui/               Gio helpers: font library, painting, scrollbar, theme,
                     menu bar, File backstage (Home/New/Open pages)
  recent/            recent-files list shared by all windows
  version/           the software version
  ole/               OLE2 compound files (.xls, .doc containers)
  textenc/           text decoding (BOMs, UTF-16, Windows-1252)
excel/
  model/             workbook/sheet/cell/style model, size limits, units
  fileio/            Open/Read: picks the reader from the file content;
                     Write/SaveFile: .xlsx, .csv, tab-delimited text
    xlsxfile/          .xlsx (streamed sheet XML; excelize for styles)
    xlsfile/           .xls BIFF8 parser
    csvfile/           .csv/.tsv (delimiter sniffing, Excel-style typing)
  numfmt/            Excel number formats (General, dates, currency, ...)
  formula/           formula engine: parser, values, evaluator, criteria, dates
    functions/         worksheet functions, one package per category
  calc/              workbook recalculation, spilling, circular references,
                     incremental recalculation after edits
  edit/              editing: typed input, clipboard, insert/delete rows,
                     columns and sheets, undo/redo
  render/            draws the grid: panes, fills, text, borders, selection
  ui/                window: title/menu/formula bars, tabs, status bar, input,
                     File pages (Info, Save As, Print, Export)
  cmd/excelshot/     headless PNG renderer (dev tool)
  cmd/calccheck/     recalculate and compare with Excel's saved values
word/              WordLite
  model/             document model: blocks, runs, styles, lists; editing
                     operations and undo (edit.go)
  fileio/            Open/Read/Write/SaveFile, picks the reader by content
    docxfile/          .docx/.docm/.dotx (keeps unmodeled parts on re-save)
    docfile/           .doc Word 97-2003 binary (FIB, piece table, STSH)
  layout/            line breaking, tabs, lists, tables, pagination, drawing
  ui/                window: toolbar, page view, caret/selection, File pages
  cmd/wordshot/      headless PNG renderer (dev tool)
Fonts/             local fonts, not committed (see Fonts/README.md)
Samples/           sample documents
```

## Fonts

Fonts are indexed from `./Fonts`, `OFFICELITE_FONTS`, the OS font folders and
Office's cloud-font cache (where Aptos lives); only the families a workbook
uses are loaded. Embedded bitmap strikes (Calibri has them) are ignored so
text always renders from outlines.

## Not yet supported

No macros (VBA is never run; `.xlsm`/`.docm` macros are dropped on save, with
a warning) and no functions beyond the 255 implemented.

Both apps: printing, PDF export, password-protected or encrypted files,
Excel 95 / Word 95 and older binary formats.

ExcelLite:
- Not shown and lost on save: charts, images, pivot tables, tables (ListObjects),
  conditional formatting, data validation, comments, row/column default
  styles, rich text runs within a cell. Rotated text is kept but drawn flat.
- Calculation: 3-D references (`Sheet1:Sheet3!A1`) and union references,
  hidden-row awareness in SUBTOTAL/AGGREGATE, PCRE-only regex features (Go's
  RE2 is used); STOCKHISTORY/TRANSLATE/DETECTLANGUAGE return `#CONNECT!`.
- Editing: no formatting commands, AutoFill, Paste Special, Find/Replace or
  sorting; references can't be picked with the mouse while typing a formula;
  Insert/Delete work on whole rows and columns; pasting doesn't copy merges.
- `.xls` save: BIFF8 limits (65,536 × 256, 4,000 styles); dynamic arrays
  that spill become ordinary formulas (Excel shows `=@...`); LET/LAMBDA
  formulas and references to other workbooks are saved as values only.
  `fileio.Fits` lists what won't fit before saving.

WordLite:
- Not shown or edited (kept on a `.docx` re-save): headers/footers, footnotes
  and endnotes, comments, tracked changes (shown accepted), text boxes and
  floating shapes (inline placeholders), multiple sections/columns.
- Fields show their last result and are not updated.
- No find/replace, format painter, right-to-left text or IME composition;
  table rows don't split across pages and header rows don't repeat.
- `.doc` save: pictures and fields from `.docx` become placeholder text,
  nested tables are flattened, table styles and cell borders beyond on/off
  are dropped (`docfile.Warnings` lists these before saving).
