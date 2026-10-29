package statefile

import (
	"io/fs"
	"syscall"
)

// Windows has no inode in a FileInfo: the creation time stands in for it,
// so a file replaced by a rename (a new file) is told from one rewritten.
func statKey(fi fs.FileInfo, k *fileKey) {
	if a, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		k.ctim = a.CreationTime.Nanoseconds()
	}
}
