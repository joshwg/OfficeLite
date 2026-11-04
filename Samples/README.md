# Samples

Samples are grouped by application and feature:

- `excel/` contains basic spreadsheets and general Excel-compatible examples.
- `excel-functions/` contains function examples and per-category function tests.
- `excel-data/` contains data tools, validation, filtering and outline examples.
- `excel-charts/` contains chart examples.
- `excel-pivots/` contains PivotTable and PivotChart examples.
- `excel-tests/` contains Numbers files used by spreadsheet import tests.
- `word/` contains Word-compatible documents and related reference images.
- `word-tests/` contains Pages files and documents used by Word import tests.

The Go test suites use these sample files for integration and file-format
coverage. Some tests skip the sample-specific checks if their input files are
not present.
