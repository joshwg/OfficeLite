// settings.go holds the AutoRecover interval setting and the per-user
// settings.json file it is kept in.

package autorecover

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"officelite/common/atomicfile"
)

// DefaultMinutes is the AutoRecover interval until the user changes it.
const DefaultMinutes = 5

// Choices are the intervals offered, in minutes; 0 is off.
var Choices = []int{0, 1, 2, 5, 10, 15, 30}

// SettingsPath is the per-user settings file, next to the recent-files
// list: %APPDATA%\OfficeLite\settings.json on Windows. Tests point it
// elsewhere.
var SettingsPath = func() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "OfficeLite", "settings.json")
}

var settings struct {
	sync.Mutex
	loaded  bool
	minutes int
}

// Minutes is how often unsaved documents are copied, in minutes; 0 is
// off. It is read from the settings file once per process.
func Minutes() int {
	settings.Lock()
	defer settings.Unlock()
	if !settings.loaded {
		settings.loaded = true
		settings.minutes = DefaultMinutes
		if m, ok := readSettings()["autoRecoverMinutes"].(float64); ok && m >= 0 {
			settings.minutes = int(m)
		}
	}
	return settings.minutes
}

// SetMinutes changes the interval for every window of this process and
// saves it for later ones. Other keys of the settings file are kept.
func SetMinutes(m int) error {
	Minutes() // loaded first, so it isn't read over the new value
	// Held through the save, so that of two calls at once the one whose
	// value stays in memory is also the one left in the file.
	settings.Lock()
	defer settings.Unlock()
	settings.minutes = max(m, 0)
	return SaveSetting("autoRecoverMinutes", max(m, 0))
}

// Setting is the value of key in the settings file (a JSON number is a
// float64); nil when it isn't there.
func Setting(key string) any { return readSettings()[key] }

// fileMu keeps two windows' changes to the settings file from losing
// one another.
var fileMu sync.Mutex

// SaveSetting writes key's value to the settings file, keeping the other
// keys.
func SaveSetting(key string, v any) error {
	path := SettingsPath()
	if path == "" {
		return nil
	}
	fileMu.Lock()
	defer fileMu.Unlock()
	s, err := loadSettings(path)
	if err != nil {
		// A file there but unreadable for now (on Windows, another
		// process may hold it): writing would drop all its other keys.
		return err
	}
	if s == nil {
		s = map[string]any{}
	}
	s[key] = v
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(path, b, 0o644)
}

// UserInterval is the user's setting as a duration (0 when off).
func UserInterval() time.Duration { return time.Duration(Minutes()) * time.Minute }

// readSettings reads the settings file's keys; nil when it is missing or
// isn't a JSON object.
func readSettings() map[string]any {
	path := SettingsPath()
	if path == "" {
		return nil
	}
	s, _ := loadSettings(path)
	return s
}

// loadSettings reads the settings file at path. A missing file, or one
// that isn't a JSON object, is nil with no error; the error is for a file
// that is there but can't be read.
func loadSettings(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s map[string]any
	if json.Unmarshal(b, &s) != nil {
		return nil, nil
	}
	return s, nil
}
