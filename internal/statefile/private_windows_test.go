package statefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// A file Write makes, or MakePrivate tightens, is readable by its owner and
// SYSTEM only, whatever its folder grants: mode bits mean nothing here.
func TestWrittenFilesAreOwnerOnlyOnWindows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui-token")
	if err := Write(path, []byte("secret\n")); err != nil {
		t.Fatal(err)
	}
	check := func(what string) {
		t.Helper()
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		sddl := sd.String()
		ctl, _, _ := sd.Control()
		if ctl&windows.SE_DACL_PROTECTED == 0 {
			t.Fatalf("%s: the access list still inherits from its folder: %s", what, sddl)
		}
		for _, broad := range []string{";;;BA)", ";;;BU)", ";;;WD)", ";;;AU)"} {
			if strings.Contains(sddl, broad) {
				t.Fatalf("%s: grants more than the owner and SYSTEM: %s", what, sddl)
			}
		}
	}
	check("Write")

	// A file an older build left with its folder's list.
	old := filepath.Join(t.TempDir(), "old-token")
	if err := os.WriteFile(old, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path = old
	if err := MakePrivate(old); err != nil {
		t.Fatal(err)
	}
	check("MakePrivate")
}
