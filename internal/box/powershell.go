package box

import (
	"path/filepath"
	"strings"
)

// Sessions on a Windows box run in PowerShell (platform.go's
// platformShell), so a session's command and PATH are written its way. The
// shell is known by its file name, so all of this runs, and is tested, on
// any system.

// isPowerShell is whether a session's shell is PowerShell.
func isPowerShell(shell string) bool {
	name := shell[strings.LastIndexAny(shell, `\/`)+1:]
	switch strings.ToLower(strings.TrimSuffix(name, ".exe")) {
	case "pwsh", "powershell":
		return true
	}
	return false
}

// psQuote is s as a PowerShell literal string.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// psSource runs a session's command file: read and run, rather than
// dot-sourced, as the file is no .ps1 and so the execution policy doesn't
// refuse it. A command that starts with a quoted program
// ('C:\…\berthd.exe' agent-host) is an expression to PowerShell unless
// called with &.
func psSource(file string) string {
	return "$c = [IO.File]::ReadAllText(" + psQuote(file) + `); if ($c -match '^\s*[''\x22]') { $c = '& ' + $c }; Invoke-Expression $c`
}

// psPATH puts dirs before PATH, then runs then.
func psPATH(dirs, then string) string {
	return "$env:Path = " + psQuote(dirs+string(filepath.ListSeparator)) + " + $env:Path; " + then
}

// shellRun is the argv that runs script in shell: a login shell's -lc, or
// PowerShell's -Command. PowerShell's own default refuses every script,
// npm's claude.ps1 included: a session's PowerShell runs local scripts
// (RemoteSigned, for that process only), as a developer's usually does.
func shellRun(shell, script string) []string {
	if isPowerShell(shell) {
		return []string{shell, "-NoLogo", "-ExecutionPolicy", "RemoteSigned", "-Command", script}
	}
	return []string{shell, "-lc", script}
}

// shellLogin is the argv of the shell on its own, for a session with no
// command.
func shellLogin(shell string) []string {
	if isPowerShell(shell) {
		return []string{shell, "-NoLogo", "-ExecutionPolicy", "RemoteSigned"}
	}
	return []string{shell, "-l"}
}
