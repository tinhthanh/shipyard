package box

import "testing"

// ioreg -n Root -d1, as macOS prints it (trimmed): one console session for
// uid 501, and another user's.
const ioregUnlocked = `+-o Root  <class IORegistryEntry, id 0x100000100, retain 31>
    {
      "IOConsoleUsers" = ({"kCGSSessionOnConsoleKey"=Yes,"kCGSSessionUserNameKey"="ana","kCGSSessionUserIDKey"=501,"kCGSessionLoginDoneKey"=Yes},{"kCGSSessionUserNameKey"="bo","kCGSSessionUserIDKey"=502,"CGSSessionScreenIsLocked"=Yes})
      "IOKitBuildVersion" = "Darwin Kernel Version 25.6.0"
    }`

const ioregLocked = `+-o Root  <class IORegistryEntry, id 0x100000100, retain 31>
    {
      "IOConsoleUsers" = ({"kCGSSessionOnConsoleKey"=Yes,"kCGSSessionUserNameKey"="ana","kCGSSessionUserIDKey"=501,"CGSSessionScreenIsLocked"=Yes,"kCGSessionLoginDoneKey"=Yes})
    }`

func TestALockedScreenIsTheUsersOwn(t *testing.T) {
	if consoleLocked(ioregUnlocked, 501) {
		t.Fatal("ana's screen is unlocked: another user's lock isn't hers")
	}
	if !consoleLocked(ioregUnlocked, 502) {
		t.Fatal("bo's session is locked")
	}
	if !consoleLocked(ioregLocked, 501) {
		t.Fatal("ana's screen is locked")
	}
	if consoleLocked(ioregLocked, 50) || consoleLocked("no console users", 501) {
		t.Fatal("no session of the user: not locked")
	}
}
