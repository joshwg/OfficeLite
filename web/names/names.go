// Package names holds the web server's rules for usernames and for the
// names of documents stored on the server. Both become names on disk
// (data/users/<username>/<file name>), so both are kept to plain ASCII
// that is safe as a UNIX file name and shows the same everywhere.
//
// Usernames: 1 to web.UsernameMaxLength characters from a–z, A–Z, 0–9,
// '.', '_' and '-', starting with a letter or digit (so "." and ".."
// can't be one). They keep the case they were created with but compare
// without regard to case: "Josh" and "josh" are the same account, whose
// folder is the lowercase name.
//
// File names: leading and trailing spaces are trimmed; what is left must
// be 1 to web.FileNameMaxLength characters, all printable ASCII (codes
// 040 to 0176 octal: no control characters, no DEL, nothing above 0177,
// so no invisible or look-alike characters), without '/', '\' or '*';
// it may not start with '.' (hidden and temporary files) or "~$" (Office
// lock files), and it must end in one of StorableExts. Like usernames,
// file names keep their case but compare without regard to it: a user
// can't have both "Budget.xlsx" and "budget.xlsx".
package names

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"officelite/web"
)

// StorableExts are the extensions a stored document may have (compared
// without regard to case): what WordLite and ExcelLite open, plus PDF
// for their exports.
var StorableExts = []string{
	".docx", ".docm", ".dotx", ".dotm", ".doc", ".dot", ".pages",
	".xlsx", ".xlsm", ".xls", ".csv", ".tsv", ".numbers",
	".txt", ".pdf",
}

// Username returns why username isn't acceptable, or nil.
func Username(username string) error {
	if username == "" {
		return errors.New("username is required")
	}
	if len(username) > web.UsernameMaxLength {
		return fmt.Errorf("username must be at most %d characters", web.UsernameMaxLength)
	}
	for i := 0; i < len(username); i++ {
		c := username[i]
		switch {
		case isAlnum(c):
		case i > 0 && (c == '.' || c == '_' || c == '-'):
		case i == 0 && (c == '.' || c == '_' || c == '-'):
			return errors.New("username must start with a letter or digit")
		default:
			return errors.New("username may contain only letters a–z and A–Z, digits, '.', '_' and '-'")
		}
	}
	return nil
}

// isAlnum reports whether c is an ASCII letter or digit.
func isAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

// UserDir is the folder name of a valid username's documents: the name
// in lowercase, as usernames compare without regard to case.
func UserDir(username string) string { return strings.ToLower(username) }

// FileName trims leading and trailing spaces from a document name and
// returns it, or why it isn't acceptable.
func FileName(name string) (string, error) {
	name = strings.Trim(name, " ")
	if name == "" {
		return "", errors.New("file name is required")
	}
	if len(name) > web.FileNameMaxLength {
		return "", fmt.Errorf("file name must be at most %d characters", web.FileNameMaxLength)
	}
	for i := 0; i < len(name); i++ {
		switch c := name[i]; {
		case c < ' ' || c > '~':
			return "", errors.New("file name may contain only printable ASCII characters (no control, invisible or accented characters)")
		case c == '/' || c == '\\' || c == '*':
			return "", errors.New(`file name may not contain '/', '\' or '*'`)
		}
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "~$") {
		return "", errors.New("file name may not start with '.' or '~$'")
	}
	if !Storable(name) {
		return "", fmt.Errorf("file name must end in one of %s", strings.Join(StorableExts, " "))
	}
	return name, nil
}

// Storable reports whether name ends in one of StorableExts.
func Storable(name string) bool {
	return slices.Contains(StorableExts, strings.ToLower(filepath.Ext(name)))
}
