// Package docfile reads and writes Word 97-2003 binary documents (.doc),
// following [MS-DOC]: the FIB, piece table, FKPs of character and
// paragraph properties, the style sheet, lists, tables, the last section's
// page setup, fields and inline pictures. Macros are never run or written.
//
// Fields and pictures read from a .doc become RunOpaque runs with Format
// "doc"; their Data is this package's own encoding of the original
// characters and formatting, which Write puts back. Opaque runs from other
// formats are written as their placeholder text; Warnings lists such
// losses before saving.
package docfile
