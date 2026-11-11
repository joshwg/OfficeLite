// A window's AutoRecover glue shared by the apps: where a recovery copy
// says the document came from, the Save As name and format of a
// recovered document.

package gui

import (
	"path/filepath"
	"slices"
	"strings"

	"officelite/common/autorecover"
)

// RecoveryOrigin is the file and format (its lowercase extension) a
// recovery copy of the window's document records: the window's file path,
// or for a document opened from a recovery copy rec, the original's.
func RecoveryOrigin(path string, rec *autorecover.Entry) (origin, format string) {
	if rec != nil {
		return rec.Path, rec.Format
	}
	return path, strings.ToLower(filepath.Ext(path))
}

// SaveAsName is the name Save As suggests: base (the document's name
// without its extension) with ext, in the original file's folder for a
// document opened from a recovery copy rec.
func SaveAsName(base, ext string, rec *autorecover.Entry) string {
	name := base + ext
	if rec != nil && rec.Path != "" {
		name = filepath.Join(filepath.Dir(rec.Path), name)
	}
	return name
}

// FormatIndex is the index of format f in formats, the Save As page's
// list, when ok (f was found for the file) and listed; 0, the first
// (native) format, otherwise.
func FormatIndex[F comparable](formats []F, f F, ok bool) int {
	if i := slices.Index(formats, f); ok && i >= 0 {
		return i
	}
	return 0
}
