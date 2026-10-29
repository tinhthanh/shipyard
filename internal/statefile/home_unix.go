//go:build !windows

package statefile

import (
	"os"
	"path/filepath"
)

func defaultHome() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "berth"), nil
}
