// Package winsys is what Shipyard asks of Windows, in one place: starting
// a program with no window, the console build of a program that has none,
// stopping our own process by its pid, an administrator's prompt, and an
// access list for the owner alone. Code shared by every platform calls it
// as it is; elsewhere each function does nothing, or answers as if there
// were nothing to do, so no caller asks which system it runs on.
package winsys

import "strings"

// consoleNames are Shipyard's programs built without a console, which
// Windows starts with no window (at sign-in, from the Run key), and the
// console build of each, beside it: what a console program they start, or a
// program run in a console (a session's), must be.
var consoleNames = map[string]string{
	"berth-agent.exe": "berth-cli.exe",
	"berthd-bg.exe":   "berthd.exe",
}

// twinName is the console build's file name for a program's, or "".
func twinName(base string) string { return consoleNames[strings.ToLower(base)] }

// windowlessName is the file name of the build without a console for a
// console program's, or "".
func windowlessName(base string) string {
	for gui, console := range consoleNames {
		if strings.EqualFold(base, console) {
			return gui
		}
	}
	return ""
}
