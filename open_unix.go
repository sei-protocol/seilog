//go:build !windows

package seilog

import "syscall"

// noFollowFlag prevents following symlinks at the final path component
// when opening log files.
const noFollowFlag = syscall.O_NOFOLLOW
