package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

// stopServing ends the berthd this box's service runs now, found by the pid
// it wrote (serve), and only when that process is this box's berthd: its
// program is program or berthd.exe beside it. It waits, briefly, for it to
// be gone.
func stopServing(b boxHome, program string) {
	raw, err := os.ReadFile(filepath.Join(b.dir, "berthd.pid"))
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid == os.Getpid() {
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) != nil {
		return
	}
	image := windows.UTF16ToString(buf[:n])
	ours := strings.EqualFold(image, program) || strings.EqualFold(image, filepath.Join(filepath.Dir(program), "berthd.exe"))
	if !ours {
		return
	}
	if windows.TerminateProcess(h, 0) == nil {
		windows.WaitForSingleObject(h, 10_000)
	}
}
