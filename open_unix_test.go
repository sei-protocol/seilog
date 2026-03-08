//go:build !windows

package seilog

import (
	"syscall"
	"testing"
)

func TestNoFollowFlag(t *testing.T) {
	if noFollowFlag != syscall.O_NOFOLLOW {
		t.Fatalf("noFollowFlag = %d, want syscall.O_NOFOLLOW (%d)", noFollowFlag, syscall.O_NOFOLLOW)
	}
}
