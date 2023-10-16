//go:build linux || darwin
// +build linux darwin

// Package fd provides filesystem descriptor count for different architectures.
package fd

import (
	"golang.org/x/sys/unix"
)

func GetNumFDs() int {
	var l unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &l); err != nil {
		return 0
	}
	return int(l.Cur)
}
