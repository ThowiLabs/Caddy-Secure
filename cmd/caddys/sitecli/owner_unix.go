//go:build !windows

package sitecli

import (
	"os"
	"syscall"
)

func ownerIDs(info os.FileInfo) (int, int) {
	if s, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(s.Uid), int(s.Gid)
	}
	return -1, -1
}
