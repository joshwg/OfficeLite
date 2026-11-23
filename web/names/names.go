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
//
// Folder names: the file-name rules, except that a folder name may not
// end in one of StorableExts (so "Reports.docx" is never a folder, and a
// folder and a document can never share a name). Paths join folder names
// and a file name with '/' ("Reports/2026/Budget.xlsx"), at most
// web.FolderMaxDepth folders deep; "" is the user's top folder.
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
	name, err := common(name, "file name")
	if err != nil {
		return "", err
	}
	if !Storable(name) {
		return "", fmt.Errorf("file name must end in one of %s", strings.Join(StorableExts, " "))
	}
	return name, nil
}

// FolderName trims leading and trailing spaces from a folder name and
// returns it, or why it isn't acceptable.
func FolderName(name string) (string, error) {
	name, err := common(name, "folder name")
	if err != nil {
		return "", err
	}
	if Storable(name) {
		return "", fmt.Errorf("folder name may not end in a file extension (%s)", strings.Join(StorableExts, " "))
	}
	return name, nil
}

// common trims name and applies the rules file and folder names share;
// what names the kind of name in the messages.
func common(name, what string) (string, error) {
	name = strings.Trim(name, " ")
	if name == "" {
		return "", fmt.Errorf("%s is required", what)
	}
	if len(name) > web.FileNameMaxLength {
		return "", fmt.Errorf("%s must be at most %d characters", what, web.FileNameMaxLength)
	}
	for i := 0; i < len(name); i++ {
		switch c := name[i]; {
		case c < ' ' || c > '~':
			return "", fmt.Errorf("%s may contain only printable ASCII characters (no control, invisible or accented characters)", what)
		case c == '/' || c == '\\' || c == '*':
			return "", fmt.Errorf(`%s may not contain '/', '\' or '*'`, what)
		}
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "~$") {
		return "", fmt.Errorf("%s may not start with '.' or '~$'", what)
	}
	return name, nil
}

// FolderPath splits a folder's path ("Reports/2026"; "" for the top
// folder) into its folder names, each trimmed, or says why it isn't
// acceptable: a name breaks the rules, or it is over web.FolderMaxDepth
// deep.
func FolderPath(path string) ([]string, error) {
	if strings.Trim(path, " ") == "" {
		return nil, nil
	}
	parts := strings.Split(path, "/")
	if len(parts) > web.FolderMaxDepth {
		return nil, fmt.Errorf("folders may be at most %d deep", web.FolderMaxDepth)
	}
	for i, p := range parts {
		clean, err := FolderName(p)
		if err != nil {
			return nil, err
		}
		parts[i] = clean
	}
	return parts, nil
}

// FilePath splits a document's path ("Reports/2026/Budget.xlsx") into its
// folders and its file name, each trimmed, or says why it isn't
// acceptable.
func FilePath(path string) ([]string, string, error) {
	dir, name := "", path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		dir, name = path[:i], path[i+1:]
		if strings.Trim(dir, " ") == "" {
			return nil, "", errors.New("folder name is required")
		}
	}
	folders, err := FolderPath(dir)
	if err != nil {
		return nil, "", err
	}
	name, err = FileName(name)
	if err != nil {
		return nil, "", err
	}
	return folders, name, nil
}

// Storable reports whether name ends in one of StorableExts.
func Storable(name string) bool {
	return slices.Contains(StorableExts, strings.ToLower(filepath.Ext(name)))
}
