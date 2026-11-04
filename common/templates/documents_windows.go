// The Documents folder on Windows: the Known Folder, which follows a
// Documents folder moved elsewhere (to OneDrive, say).

package templates

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// documentsDir is the user's Documents folder, falling back to
// %USERPROFILE%\Documents; "" when neither is known.
func documentsDir() string {
	if dir, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0); err == nil && dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, "Documents")
}
