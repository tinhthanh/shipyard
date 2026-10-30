package box

import (
	"path/filepath"
	"reflect"
	"testing"
)

func runtimeSeparator() string { return string(filepath.ListSeparator) }

func TestASessionOnWindowsRunsItsCommandInPowerShell(t *testing.T) {
	for _, sh := range []string{`C:\Program Files\PowerShell\7\pwsh.exe`, "powershell.exe", "pwsh"} {
		if !isPowerShell(sh) {
			t.Errorf("%s is PowerShell", sh)
		}
	}
	for _, sh := range []string{"/bin/bash", "/usr/bin/fish", "/bin/sh"} {
		if isPowerShell(sh) {
			t.Errorf("%s is not PowerShell", sh)
		}
	}
	ps := "powershell.exe"
	file := `C:\Users\ana\berth\box\commands\it's.cmd`
	got := shellRun(ps, withPATH(ps, `C:\node;C:\tools`, sourceCommand(ps, file)))
	want := []string{ps, "-NoLogo", "-ExecutionPolicy", "RemoteSigned", "-Command", `$env:Path = 'C:\node;C:\tools;' + $env:Path; $c = [IO.File]::ReadAllText('C:\Users\ana\berth\box\commands\it''s.cmd'); if ($c -match '^\s*[''\x22]') { $c = '& ' + $c }; Invoke-Expression $c`}
	if runtimeSeparator() != ";" {
		want[5] = `$env:Path = 'C:\node;C:\tools:' + $env:Path; $c = [IO.File]::ReadAllText('C:\Users\ana\berth\box\commands\it''s.cmd'); if ($c -match '^\s*[''\x22]') { $c = '& ' + $c }; Invoke-Expression $c`
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("shellRun = %q\nwant      %q", got, want)
	}
	if got := shellLogin(ps); !reflect.DeepEqual(got, []string{ps, "-NoLogo", "-ExecutionPolicy", "RemoteSigned"}) {
		t.Errorf("shellLogin = %q", got)
	}
	// Other shells are as before.
	if got := shellRun("/bin/bash", sourceCommand("/bin/bash", "/tmp/c")); !reflect.DeepEqual(got, []string{"/bin/bash", "-lc", ". '/tmp/c'"}) {
		t.Errorf("bash: %q", got)
	}
	if got := shellLogin("/bin/zsh"); !reflect.DeepEqual(got, []string{"/bin/zsh", "-l"}) {
		t.Errorf("zsh: %q", got)
	}
}
