package statefile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
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
	home = moveHome(filepath.Join(roaming, "berth"), home)
	privateDir(home)
	return home, nil
}

// privateDir lets only the current user (and SYSTEM) into dir, and into
// what it holds, now and later: its files take its list unless they have
// their own (Write gives each its own). Files written before there was one
// (an identity key from an older berth) took their folder's, which can
// include every administrator, and change with it here. Once done, the
// folder's list is protected, and later calls leave it be.
func privateDir(dir string) error {
	if sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION); err == nil {
		if ctl, _, err := sd.Control(); err == nil && ctl&windows.SE_DACL_PROTECTED != 0 {
			return nil
		}
	}
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + u.User.Sid.String() + ")")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
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
