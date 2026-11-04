//go:build !windows

// The Documents folder outside Windows: ~/Documents.

package templates

import (
	"os"
	"path/filepath"
)

// documentsDir is the user's Documents folder; "" without a home folder.
func documentsDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Documents")
}
