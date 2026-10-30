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
See the
[ExcelLite limitations](./EXCEL.md#limitations-compared-with-excel) and
[WordLite limitations](./WORD.md#limitations-compared-with-word) for details.

## Running and building

```sh
./officelite.sh Samples/Book1.xlsx a.xls   # build and open workbooks
./officelite.sh Samples/Word-Sample.docx   # open a document
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
[EXCEL-FUNCTIONS.md](./EXCEL-FUNCTIONS.md) for feature details. `Samples/`
contains example workbooks and documents.
