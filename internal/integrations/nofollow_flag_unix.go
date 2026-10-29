//go:build !windows

package integrations

import "syscall"

// oNoFollow refuses to open a symbolic link at the last step of a path.
const oNoFollow = syscall.O_NOFOLLOW
