# OfficeLite

Lightweight, macro-free office applications written in Go. OfficeLite is one
program with two editors: ExcelLite for spreadsheets and WordLite for
word-processing documents. It routes files by their content, not just their
extension, and opens each document in its own window. Version 0.7.0.1
(`common/version`; `officelite --version`).

Run the build and sample commands below from the repository root.

## Architecture

The application shares its launcher, file detection, UI components, PDF
export, recent-file handling and font support. ExcelLite has a workbook model
and formula/calculation engine; WordLite has a document model and page-layout
engine. See [ARCHITECTURE.md](./ARCHITECTURE.md) for the package map and
shared file handling, recovery, AutoSave and rendering behavior.

## Limitations

OfficeLite is not a full replacement for Microsoft Excel or Word. It does not
run VBA macros, open encrypted/password-protected documents, save to Apple's
file formats, or read legacy Excel 95/Word 95 and older binary formats.
Keynote files are unsupported. Printing is done by exporting to PDF and
opening the PDF in the system viewer; PDF output currently uses Letter paper.
See [Macros](#macros), the
[ExcelLite limitations](./EXCEL.md#limitations-compared-with-excel) and
[WordLite limitations](./WORD.md#limitations-compared-with-word) for details.

## Macros

OfficeLite never runs macros: VBA projects in `.xlsm`, `.xls`, `.docm`,
`.doc`, `.dotm` and other files, and Excel 4 macro sheets. When a file
with macros opens, a message says that they won't run, so buttons, forms
and anything else that depends on them won't work. The file's other
content opens as usual.

OfficeLite can't save macros either. Saving a workbook or document that
has them (Save, or Save As to any format) first asks **Save without
macros?**: Save writes the file without them, and Cancel leaves the file
as it is. To keep the original with its macros, use Save As to save a
copy under a new name. Exports (PDF, CSV, text) leave the file alone and
don't ask. AutoSave stays off until the file has been saved once without
its macros. A new document made from a template with macros gets the
notice but no save warning, since the template keeps its macros.

## Running and building

```sh
./officelite.sh Samples/excel/Book1.xlsx a.xls   # build and open workbooks
./officelite.sh Samples/word/Word-Sample.docx   # open a document
./officelite.sh                            # open a blank workbook
./officelite.sh --word                     # open a blank document
./officelite.sh --excel notes.txt          # force a spreadsheet
./officelite.sh --linux file.csv           # native Linux build (needs GUI headers)
./officelite.sh --apple                    # build for Apple Silicon on macOS
./officelite.sh --build                    # build only
```

Under WSL, the default is `bin/OfficeLite.exe`; the Linux build needs CGO and
X11/Wayland development headers. On macOS, `--apple` builds
`bin/OfficeLite-Apple-Silicon`, separate from the Windows and Linux binaries.
When launching the native Linux build, the script defaults Mesa to software
rendering to avoid DRI3 warnings on displays without a DRI3 device. Set
`LIBGL_ALWAYS_SOFTWARE=0` to enable hardware rendering on a supported display:
`LIBGL_ALWAYS_SOFTWARE=0 ./officelite.sh --linux`.

For macOS builds, install Apple's command-line developer tools if needed and
enable CGO:

```sh
xcode-select --install
export CGO_ENABLED=1
```

Build directly for either Mac architecture:

```sh
GOOS=darwin GOARCH=arm64 go build -o bin/OfficeLite-Apple-Silicon ./cmd/officelite  # Apple Silicon
GOOS=darwin GOARCH=amd64 go build -o bin/OfficeLite-Intel ./cmd/officelite        # Intel Mac
```

See [EXCEL.md](./EXCEL.md), [WORD.md](./WORD.md), and
[EXCEL-FUNCTIONS.md](./EXCEL-FUNCTIONS.md) for feature details, and
[KEYBOARD.md](./KEYBOARD.md) for every key binding (Windows and Linux, macOS). The
[`Samples/`](../Samples/) directory groups workbooks and documents by topic.
