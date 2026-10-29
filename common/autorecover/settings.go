package autorecover

import (
	"encoding/json"
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
	settings.Lock()
	settings.minutes = max(m, 0)
	settings.Unlock()
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
	s := readSettings()
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

func readSettings() map[string]any {
	path := SettingsPath()
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var s map[string]any
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return s
}
