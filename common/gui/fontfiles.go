// FontFiles: the font files behind a family, for PDF embedding.

package gui

import (
	"path/filepath"
	"strings"
)

// FontFiles lists the font files of family and then of its
// metric-compatible substitutes, as Faces would load them (the first
// folder holding a family wins), so a PDF can embed the fonts the screen
// shows. It implements pdf.FontSource.
func (l *FontLibrary) FontFiles(family string) []string {
	if l == nil || len(l.dirs) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.scanned {
		l.scan()
		l.scanned = true
	}
	fams := []string{strings.ToLower(family)}
	for _, sub := range strings.Split(metricCompatible[fams[0]], ",") {
		if sub = strings.ToLower(strings.TrimSpace(sub)); sub != "" {
			fams = append(fams, sub)
		}
	}
	var out []string
	for _, fam := range fams {
		dir := ""
		for _, p := range l.index[fam] {
			if dir == "" {
				dir = filepath.Dir(p)
			}
			if filepath.Dir(p) == dir {
				out = append(out, p)
			}
		}
	}
	return out
}
