package box

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// screenLocked says whether this Mac's screen is locked for the user berthd
// runs as: a plugin's setup that needs the person (macOS permissions, a
// browser extension) can't be finished there until someone unlocks it. It
// is false wherever it can't tell (Linux, ioreg missing).
func screenLocked(ctx context.Context) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ioreg", "-n", "Root", "-d1").Output()
	if err != nil {
		return false
	}
	return consoleLocked(string(out), os.Getuid())
}

// consoleLocked reads ioreg's IOConsoleUsers: the user's console session
// carries "CGSSessionScreenIsLocked"=Yes while its screen is locked.
func consoleLocked(ioreg string, uid int) bool {
	i := strings.Index(ioreg, `"IOConsoleUsers"`)
	if i < 0 {
		return false
	}
	users := ioreg[i:]
	if j := strings.IndexByte(users, '\n'); j >= 0 {
		users = users[:j]
	}
	me := `"kCGSSessionUserIDKey"=` + strconv.Itoa(uid)
	for _, session := range strings.Split(users, "},{") {
		if strings.Contains(session, me+",") || strings.Contains(session, me+"}") {
			return strings.Contains(session, `"CGSSessionScreenIsLocked"=Yes`)
		}
	}
	return false
}
