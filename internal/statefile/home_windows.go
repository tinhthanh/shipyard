package statefile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// defaultHome is %LOCALAPPDATA%\berth. The state holds this computer's
// identity key and the app's token, which must not follow a roaming
// profile to a server or another machine, as %APPDATA% (where it was)
// would. What is still in %APPDATA%\berth moves over the first time it
// can; while something holds it open (an agent from before the move), the
// old folder goes on being the state.
func defaultHome() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return "", errors.New("%LOCALAPPDATA% is not set")
	}
	home := filepath.Join(local, "berth")
	roaming, err := os.UserConfigDir()
	if err != nil {
		return home, nil
	}
	return moveHome(filepath.Join(roaming, "berth"), home), nil
}

// moveHome moves what old holds into home, entry by entry, and answers the
// folder to use: home, or old while an entry of it can't move yet. An
// entry already in home is kept and the old one left alone.
func moveHome(old, home string) string {
	entries, err := os.ReadDir(old)
	if errors.Is(err, fs.ErrNotExist) {
		return home
	}
	if err != nil {
		return old
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return old
	}
	for _, e := range entries {
		to := filepath.Join(home, e.Name())
		if _, err := os.Lstat(to); err == nil {
			continue
		}
		if err := os.Rename(filepath.Join(old, e.Name()), to); err != nil {
			return old
		}
	}
	// Gone only if empty: whatever stayed behind stays.
	os.Remove(old)
	return home
}
