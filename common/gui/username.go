// username.go names the person using the app, for the author of new
// notes and comments.

package gui

import (
	"os"
	"strings"
)

// UserName is the user's login name (USERNAME on Windows, USER
// elsewhere), or "" when neither is set (in the browser).
func UserName() string {
	for _, k := range []string{"USERNAME", "USER"} {
		if s := strings.TrimSpace(os.Getenv(k)); s != "" {
			return s
		}
	}
	return ""
}
